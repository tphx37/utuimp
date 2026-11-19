//go:build linux

// Package gst provides a GStreamer-backed audio player for URI-based streams.
// It wraps a playbin3 (or playbin fallback) pipeline via CGO, handling HLS
// manifests, plain HTTPS audio files, and eventual authenticated streams.
package gst

/*
#cgo pkg-config: gstreamer-1.0
#cgo LDFLAGS: -lm
#include <gst/gst.h>
#include <arpa/inet.h>
#include <math.h>
#include <netinet/in.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#define UTUIMP_EQ_BANDS 10

static const double utuimp_eq_freqs[UTUIMP_EQ_BANDS] = {
    29.0, 59.0, 119.0, 237.0, 474.0, 947.0, 1889.0, 3770.0, 7523.0, 15011.0
};

typedef struct {
    double b0, b1, b2, a1, a2;
    double z1[2], z2[2];
    gboolean active;
} UtuimpBiquad;

static void utuimp_biquad_design(UtuimpBiquad* bq, double fs, double f0, double gain_db) {
    if (fabs(gain_db) < 0.01 || fs <= 0.0 || f0 >= fs * 0.45) {
        bq->active = FALSE;
        return;
    }
    double A = pow(10.0, gain_db / 40.0);
    double w0 = 2.0 * 3.14159265358979323846 * f0 / fs;
    double alpha = sin(w0) / (2.0 * 1.4);
    double cw = cos(w0);
    double a0 = 1.0 + alpha / A;
    bq->b0 = (1.0 + alpha * A) / a0;
    bq->b1 = (-2.0 * cw) / a0;
    bq->b2 = (1.0 - alpha * A) / a0;
    bq->a1 = (-2.0 * cw) / a0;
    bq->a2 = (1.0 - alpha / A) / a0;
    bq->active = TRUE;
}

static inline double utuimp_biquad_run(UtuimpBiquad* bq, double x, int ch) {
    double y = bq->b0 * x + bq->z1[ch];
    bq->z1[ch] = bq->b1 * x - bq->a1 * y + bq->z2[ch];
    bq->z2[ch] = bq->b2 * x - bq->a2 * y;
    return y;
}

typedef struct {
    GMutex mu;
    gchar* next_uri;
    gboolean trim_next; // the next stream to start is a whole track
    gboolean trim_cur;  // the current stream's first segment still needs trimming
    guint64 trimmed;    // time cut from earlier streams since the last flush
    gdouble eq_gains[UTUIMP_EQ_BANDS];
    guint32 eq_ver;
    guint32 eq_seen;
    gint eq_rate;
    UtuimpBiquad eq_bank[UTUIMP_EQ_BANDS];
} UtuimpGstState;

// ── Embedded GstElement: utuimp_aac_parse ────────────────────────────────────
// Parses ADTS AAC frames into raw AAC frames with codec_data (AudioSpecificConfig)
// and TIME segments so decodebin3/playbin3 works without gst-plugins-good's aacparse.

typedef struct {
    GstElement parent;
    GstPad* sinkpad;
    GstPad* srcpad;
    GByteArray* adapter;
    GstEvent* pending_segment;
    gint rate;
    gint channels;
    gboolean caps_set;
    gboolean synced;
    guint8 hdr_proto[4];
    guint64 frames_pushed;
} UtuimpAacParse;

typedef struct {
    GstElementClass parent_class;
} UtuimpAacParseClass;

static GType utuimp_aac_parse_get_type(void);
G_DEFINE_TYPE(UtuimpAacParse, utuimp_aac_parse, GST_TYPE_ELEMENT)

// ADTS only. qtdemux hands decodebin3 raw framed AAC for local .m4a files;
// ranked above aacparse, this parser would otherwise be plugged after it,
// find no syncwords, and drop every buffer.
static GstStaticPadTemplate utuimp_aac_sink_tmpl = GST_STATIC_PAD_TEMPLATE(
    "sink",
    GST_PAD_SINK,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS("audio/mpeg, mpegversion=(int){ 2, 4 }, stream-format=(string)adts")
);

static GstStaticPadTemplate utuimp_aac_src_tmpl = GST_STATIC_PAD_TEMPLATE(
    "src",
    GST_PAD_SRC,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS("audio/mpeg, framed=(boolean)true, mpegversion=(int){ 2, 4 }, stream-format=(string){ raw, adts }")
);

static const int utuimp_adts_rates[16] = {
    96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
    16000, 12000, 11025, 8000, 7350, 0, 0, 0
};

static GstFlowReturn utuimp_aac_parse_chain(GstPad* pad, GstObject* parent, GstBuffer* buf) {
    UtuimpAacParse* self = (UtuimpAacParse*)parent;
    GstMapInfo map;
    if (gst_buffer_map(buf, &map, GST_MAP_READ)) {
        g_byte_array_append(self->adapter, map.data, map.size);
        gst_buffer_unmap(buf, &map);
    }
    gst_buffer_unref(buf);

    GstFlowReturn ret = GST_FLOW_OK;
    guint offset = 0;
    guint len = self->adapter->len;
    const guint8* data = self->adapter->data;

    while (offset + 7 <= len) {
        if (data[offset] != 0xFF || (data[offset + 1] & 0xF6) != 0xF0) {
            self->synced = FALSE;
            offset++;
            continue;
        }
        guint hdr_len = (data[offset + 1] & 0x01) ? 7 : 9;
        guint frame_len = ((guint)(data[offset + 3] & 0x03) << 11) |
                          ((guint)data[offset + 4] << 3) |
                          ((guint)(data[offset + 5] >> 5) & 0x07);
        int freq_idx = (data[offset + 2] >> 2) & 0x0F;
        int chan_cfg = ((data[offset + 2] & 0x01) << 2) | ((data[offset + 3] >> 6) & 0x03);
        int rate = utuimp_adts_rates[freq_idx];
        if (frame_len <= hdr_len || rate <= 0 || chan_cfg == 0) {
            self->synced = FALSE;
            offset++;
            continue;
        }
        if (self->synced &&
            (data[offset + 1] != self->hdr_proto[1] ||
             data[offset + 2] != self->hdr_proto[2] ||
             (data[offset + 3] & 0xFC) != (self->hdr_proto[3] & 0xFC))) {
            self->synced = FALSE;
            offset++;
            continue;
        }
        if (offset + frame_len > len) {
            break;
        }
        // When unsynced, verify that the byte immediately after this candidate
        // frame is another valid ADTS header with matching stream parameters.
        if (!self->synced) {
            if (offset + frame_len + 7 <= len) {
                const guint8* next = data + offset + frame_len;
                if (next[0] != 0xFF ||
                    next[1] != data[offset + 1] ||
                    next[2] != data[offset + 2] ||
                    (next[3] & 0xFC) != (data[offset + 3] & 0xFC)) {
                    offset++;
                    continue;
                }
            } else if (offset > 0) {
                // Wait for more bytes to confirm mid-buffer resync candidate.
                break;
            }
            self->synced = TRUE;
            memcpy(self->hdr_proto, data + offset, 4);
        }

        int profile = ((data[offset + 2] >> 6) & 0x03) + 1;
        int channels = (chan_cfg == 7) ? 8 : chan_cfg;
        if (!self->caps_set || self->rate != rate || self->channels != channels) {
            self->rate = rate;
            self->channels = channels;
            self->caps_set = TRUE;

            guint8 asc[2];
            asc[0] = (guint8)((profile << 3) | ((freq_idx >> 1) & 0x07));
            asc[1] = (guint8)(((freq_idx & 0x01) << 7) | (chan_cfg << 3));
            GstBuffer* codec_buf = gst_buffer_new_allocate(NULL, 2, NULL);
            gst_buffer_fill(codec_buf, 0, asc, 2);

            GstCaps* caps = gst_caps_new_simple("audio/mpeg",
                "mpegversion", G_TYPE_INT, 4,
                "framed", G_TYPE_BOOLEAN, TRUE,
                "stream-format", G_TYPE_STRING, "raw",
                "rate", G_TYPE_INT, rate,
                "channels", G_TYPE_INT, channels,
                "codec_data", GST_TYPE_BUFFER, codec_buf,
                NULL);
            gst_buffer_unref(codec_buf);
            gst_pad_set_caps(self->srcpad, caps);
            gst_caps_unref(caps);

            if (self->pending_segment) {
                gst_pad_push_event(self->srcpad, self->pending_segment);
                self->pending_segment = NULL;
            } else {
                GstSegment seg;
                gst_segment_init(&seg, GST_FORMAT_TIME);
                gst_pad_push_event(self->srcpad, gst_event_new_segment(&seg));
            }
        }

        guint raw_len = frame_len - hdr_len;
        GstBuffer* out = gst_buffer_new_allocate(NULL, raw_len, NULL);
        gst_buffer_fill(out, 0, data + offset + hdr_len, raw_len);
        GstClockTime pts = gst_util_uint64_scale_int(self->frames_pushed * 1024, GST_SECOND, rate);
        GstClockTime dur = gst_util_uint64_scale_int(1024, GST_SECOND, rate);
        GST_BUFFER_PTS(out) = pts;
        GST_BUFFER_DTS(out) = pts;
        GST_BUFFER_DURATION(out) = dur;
        self->frames_pushed++;

        offset += frame_len;
        ret = gst_pad_push(self->srcpad, out);
        if (ret != GST_FLOW_OK) {
            break;
        }
    }

    if (offset > 0) {
        g_byte_array_remove_range(self->adapter, 0, offset);
    }
    return ret;
}

static gboolean utuimp_aac_parse_sink_event(GstPad* pad, GstObject* parent, GstEvent* event) {
    UtuimpAacParse* self = (UtuimpAacParse*)parent;
    switch (GST_EVENT_TYPE(event)) {
    case GST_EVENT_STREAM_START:
        g_byte_array_set_size(self->adapter, 0);
        if (self->pending_segment) {
            gst_event_unref(self->pending_segment);
            self->pending_segment = NULL;
        }
        self->synced = FALSE;
        self->frames_pushed = 0;
        return gst_pad_event_default(pad, parent, event);
    case GST_EVENT_CAPS:
        gst_event_unref(event);
        return TRUE;
    case GST_EVENT_SEGMENT: {
        self->frames_pushed = 0;
        GstSegment seg;
        gst_segment_init(&seg, GST_FORMAT_TIME);
        GstEvent* tev = gst_event_new_segment(&seg);
        gst_event_set_seqnum(tev, gst_event_get_seqnum(event));
        gst_event_unref(event);
        if (self->caps_set) {
            return gst_pad_push_event(self->srcpad, tev);
        }
        if (self->pending_segment) gst_event_unref(self->pending_segment);
        self->pending_segment = tev;
        return TRUE;
    }
    case GST_EVENT_FLUSH_STOP:
        g_byte_array_set_size(self->adapter, 0);
        if (self->pending_segment) {
            gst_event_unref(self->pending_segment);
            self->pending_segment = NULL;
        }
        self->synced = FALSE;
        self->frames_pushed = 0;
        return gst_pad_event_default(pad, parent, event);
    case GST_EVENT_EOS:
        g_byte_array_set_size(self->adapter, 0);
        self->synced = FALSE;
        return gst_pad_event_default(pad, parent, event);
    default:
        return gst_pad_event_default(pad, parent, event);
    }
}

static GstStateChangeReturn utuimp_aac_parse_change_state(GstElement* element, GstStateChange transition) {
    UtuimpAacParse* self = (UtuimpAacParse*)element;
    if (transition == GST_STATE_CHANGE_PAUSED_TO_READY) {
        g_byte_array_set_size(self->adapter, 0);
        if (self->pending_segment) {
            gst_event_unref(self->pending_segment);
            self->pending_segment = NULL;
        }
        self->caps_set = FALSE;
        self->synced = FALSE;
        self->frames_pushed = 0;
    }
    return GST_ELEMENT_CLASS(utuimp_aac_parse_parent_class)->change_state(element, transition);
}

static void utuimp_aac_parse_finalize(GObject* object) {
    UtuimpAacParse* self = (UtuimpAacParse*)object;
    if (self->pending_segment) {
        gst_event_unref(self->pending_segment);
        self->pending_segment = NULL;
    }
    if (self->adapter) {
        g_byte_array_free(self->adapter, TRUE);
        self->adapter = NULL;
    }
    G_OBJECT_CLASS(utuimp_aac_parse_parent_class)->finalize(object);
}

static void utuimp_aac_parse_class_init(UtuimpAacParseClass* klass) {
    GObjectClass* gobject_class = G_OBJECT_CLASS(klass);
    GstElementClass* element_class = GST_ELEMENT_CLASS(klass);
    gobject_class->finalize = utuimp_aac_parse_finalize;
    element_class->change_state = utuimp_aac_parse_change_state;
    gst_element_class_add_static_pad_template(element_class, &utuimp_aac_sink_tmpl);
    gst_element_class_add_static_pad_template(element_class, &utuimp_aac_src_tmpl);
    gst_element_class_set_static_metadata(element_class,
        "utuimp AAC Parser", "Codec/Parser/Audio",
        "Embedded ADTS AAC frame parser", "utuimp");
}

static void utuimp_aac_parse_init(UtuimpAacParse* self) {
    self->adapter = g_byte_array_new();
    self->sinkpad = gst_pad_new_from_static_template(&utuimp_aac_sink_tmpl, "sink");
    gst_pad_set_chain_function(self->sinkpad, utuimp_aac_parse_chain);
    gst_pad_set_event_function(self->sinkpad, utuimp_aac_parse_sink_event);
    gst_element_add_pad(GST_ELEMENT(self), self->sinkpad);

    self->srcpad = gst_pad_new_from_static_template(&utuimp_aac_src_tmpl, "src");
    gst_pad_use_fixed_caps(self->srcpad);
    gst_element_add_pad(GST_ELEMENT(self), self->srcpad);
}

// ── Embedded GstElement: utuimp_mpa_parse ────────────────────────────────────
// Frames MPEG-1/2/2.5 Layer I–III audio (MP3) so decodebin3 can feed
// avdec_mp3 without gst-plugins-good's mpegaudioparse. Ranked below
// mpegaudioparse, which wins whenever it is installed.

typedef struct {
    GstElement parent;
    GstPad* sinkpad;
    GstPad* srcpad;
    GByteArray* adapter;
    GstEvent* pending_segment;
    gint rate;
    gint channels;
    gint layer;
    gboolean caps_set;
    gboolean synced;
    gboolean id3_checked;
    guint id3_skip; // bytes of an ID3v2 tag still to drop
    guint8 hdr_proto[3];
    guint64 samples_pushed;
} UtuimpMpaParse;

typedef struct {
    GstElementClass parent_class;
} UtuimpMpaParseClass;

static GType utuimp_mpa_parse_get_type(void);
G_DEFINE_TYPE(UtuimpMpaParse, utuimp_mpa_parse, GST_TYPE_ELEMENT)

static GstStaticPadTemplate utuimp_mpa_sink_tmpl = GST_STATIC_PAD_TEMPLATE(
    "sink", GST_PAD_SINK, GST_PAD_ALWAYS,
    // application/x-id3: MP3 files that start with an ID3v2 tag type-find as
    // that (id3demux, also gst-plugins-good, would normally strip it); the
    // parser skips the tag itself.
    GST_STATIC_CAPS("audio/mpeg, mpegversion=(int)1, parsed=(boolean)false; application/x-id3")
);

static GstStaticPadTemplate utuimp_mpa_src_tmpl = GST_STATIC_PAD_TEMPLATE(
    "src", GST_PAD_SRC, GST_PAD_ALWAYS,
    GST_STATIC_CAPS("audio/mpeg, mpegversion=(int)1, layer=(int)[ 1, 3 ], parsed=(boolean)true")
);

static const int utuimp_mpa_br[2][3][16] = {
    { // MPEG-1: layer I, II, III
        {0,32,64,96,128,160,192,224,256,288,320,352,384,416,448,0},
        {0,32,48,56,64,80,96,112,128,160,192,224,256,320,384,0},
        {0,32,40,48,56,64,80,96,112,128,160,192,224,256,320,0},
    },
    { // MPEG-2 / 2.5
        {0,32,48,56,64,80,96,112,128,144,160,176,192,224,256,0},
        {0,8,16,24,32,40,48,56,64,80,96,112,128,144,160,0},
        {0,8,16,24,32,40,48,56,64,80,96,112,128,144,160,0},
    },
};
static const int utuimp_mpa_sr[3][3] = {
    {44100, 48000, 32000}, // MPEG-1
    {22050, 24000, 16000}, // MPEG-2
    {11025, 12000, 8000},  // MPEG-2.5
};

// utuimp_mpa_header parses a 4-byte header; returns the frame length in
// bytes (0 if invalid) and fills rate, channels, layer and samples.
static guint utuimp_mpa_header(const guint8* h, int* rate, int* channels, int* layer, int* samples) {
    if (h[0] != 0xFF || (h[1] & 0xE0) != 0xE0) return 0;
    int ver = (h[1] >> 3) & 3;      // 3=MPEG1 2=MPEG2 0=MPEG2.5
    int lay = (h[1] >> 1) & 3;      // 3=I 2=II 1=III
    int bri = (h[2] >> 4) & 0xF;
    int sri = (h[2] >> 2) & 3;
    int pad = (h[2] >> 1) & 1;
    if (ver == 1 || lay == 0 || bri == 0 || bri == 15 || sri == 3) return 0;
    int v = (ver == 3) ? 0 : 1;
    int vs = (ver == 3) ? 0 : (ver == 2 ? 1 : 2);
    int l = 4 - lay; // 1..3
    int br = utuimp_mpa_br[v][l - 1][bri] * 1000;
    int sr = utuimp_mpa_sr[vs][sri];
    if (br <= 0 || sr <= 0) return 0;
    guint len;
    int spf;
    if (l == 1) {
        len = (guint)((12 * br / sr + pad) * 4);
        spf = 384;
    } else if (l == 2 || v == 0) {
        len = (guint)(144 * br / sr + pad);
        spf = 1152;
    } else {
        len = (guint)(72 * br / sr + pad);
        spf = 576;
    }
    *rate = sr;
    *channels = (((h[3] >> 6) & 3) == 3) ? 1 : 2;
    *layer = l;
    *samples = spf;
    return len > 4 ? len : 0;
}

static GstFlowReturn utuimp_mpa_parse_chain(GstPad* pad, GstObject* parent, GstBuffer* buf) {
    UtuimpMpaParse* self = (UtuimpMpaParse*)parent;
    GstMapInfo map;
    if (gst_buffer_map(buf, &map, GST_MAP_READ)) {
        g_byte_array_append(self->adapter, map.data, map.size);
        gst_buffer_unmap(buf, &map);
    }
    gst_buffer_unref(buf);

    if (self->id3_skip > 0) {
        guint n = MIN(self->id3_skip, self->adapter->len);
        g_byte_array_remove_range(self->adapter, 0, n);
        self->id3_skip -= n;
        if (self->id3_skip > 0) return GST_FLOW_OK;
    }

    guint len = self->adapter->len;
    const guint8* data = self->adapter->data;
    guint offset = 0;

    // Skip a leading ID3v2 tag.
    if (!self->id3_checked) {
        if (len < 10) return GST_FLOW_OK;
        self->id3_checked = TRUE;
        if (data[0] == 'I' && data[1] == 'D' && data[2] == '3') {
            guint tag = ((data[6] & 0x7F) << 21) | ((data[7] & 0x7F) << 14) | ((data[8] & 0x7F) << 7) | (data[9] & 0x7F);
            tag += 10;
            if (data[5] & 0x10) tag += 10;
            if (tag > len) {
                // A big tag (cover art): drop the rest as it arrives.
                self->id3_skip = tag - len;
                g_byte_array_set_size(self->adapter, 0);
                return GST_FLOW_OK;
            }
            offset = tag;
        }
    }

    GstFlowReturn ret = GST_FLOW_OK;
    while (offset + 4 <= len) {
        int rate, channels, layer, spf;
        guint flen = utuimp_mpa_header(data + offset, &rate, &channels, &layer, &spf);
        if (flen == 0 || (self->synced &&
            ((data[offset + 1] & 0xFE) != (self->hdr_proto[1] & 0xFE) ||
             (data[offset + 2] & 0x0C) != (self->hdr_proto[2] & 0x0C)))) {
            self->synced = FALSE;
            offset++;
            continue;
        }
        if (offset + flen > len) break;
        if (!self->synced) {
            // Confirm with the following header before trusting a sync.
            if (offset + flen + 4 <= len) {
                int r2, c2, l2, s2;
                const guint8* n = data + offset + flen;
                if (utuimp_mpa_header(n, &r2, &c2, &l2, &s2) == 0 || r2 != rate || l2 != layer) {
                    offset++;
                    continue;
                }
            } else {
                break;
            }
            self->synced = TRUE;
            memcpy(self->hdr_proto, data + offset, 3);
        }
        if (!self->caps_set || self->rate != rate || self->channels != channels || self->layer != layer) {
            self->rate = rate;
            self->channels = channels;
            self->layer = layer;
            self->caps_set = TRUE;
            GstCaps* caps = gst_caps_new_simple("audio/mpeg",
                "mpegversion", G_TYPE_INT, 1,
                "layer", G_TYPE_INT, layer,
                "parsed", G_TYPE_BOOLEAN, TRUE,
                "rate", G_TYPE_INT, rate,
                "channels", G_TYPE_INT, channels,
                NULL);
            gst_pad_set_caps(self->srcpad, caps);
            gst_caps_unref(caps);
            if (self->pending_segment) {
                gst_pad_push_event(self->srcpad, self->pending_segment);
                self->pending_segment = NULL;
            } else {
                GstSegment seg;
                gst_segment_init(&seg, GST_FORMAT_TIME);
                gst_pad_push_event(self->srcpad, gst_event_new_segment(&seg));
            }
        }
        GstBuffer* out = gst_buffer_new_allocate(NULL, flen, NULL);
        gst_buffer_fill(out, 0, data + offset, flen);
        GstClockTime pts = gst_util_uint64_scale_int(self->samples_pushed, GST_SECOND, rate);
        GST_BUFFER_PTS(out) = pts;
        GST_BUFFER_DTS(out) = pts;
        GST_BUFFER_DURATION(out) = gst_util_uint64_scale_int(spf, GST_SECOND, rate);
        self->samples_pushed += spf;
        offset += flen;
        ret = gst_pad_push(self->srcpad, out);
        if (ret != GST_FLOW_OK) break;
    }
    if (offset > 0) g_byte_array_remove_range(self->adapter, 0, MIN(offset, self->adapter->len));
    return ret;
}

static void utuimp_mpa_reset(UtuimpMpaParse* self) {
    g_byte_array_set_size(self->adapter, 0);
    if (self->pending_segment) {
        gst_event_unref(self->pending_segment);
        self->pending_segment = NULL;
    }
    self->synced = FALSE;
    self->id3_checked = FALSE;
    self->id3_skip = 0;
    self->samples_pushed = 0;
}

static gboolean utuimp_mpa_parse_sink_event(GstPad* pad, GstObject* parent, GstEvent* event) {
    UtuimpMpaParse* self = (UtuimpMpaParse*)parent;
    switch (GST_EVENT_TYPE(event)) {
    case GST_EVENT_STREAM_START:
    case GST_EVENT_FLUSH_STOP:
        utuimp_mpa_reset(self);
        return gst_pad_event_default(pad, parent, event);
    case GST_EVENT_CAPS:
        gst_event_unref(event);
        return TRUE;
    case GST_EVENT_SEGMENT: {
        self->samples_pushed = 0;
        GstSegment seg;
        gst_segment_init(&seg, GST_FORMAT_TIME);
        GstEvent* tev = gst_event_new_segment(&seg);
        gst_event_set_seqnum(tev, gst_event_get_seqnum(event));
        gst_event_unref(event);
        if (self->caps_set) return gst_pad_push_event(self->srcpad, tev);
        if (self->pending_segment) gst_event_unref(self->pending_segment);
        self->pending_segment = tev;
        return TRUE;
    }
    case GST_EVENT_EOS:
        g_byte_array_set_size(self->adapter, 0);
        self->synced = FALSE;
        return gst_pad_event_default(pad, parent, event);
    default:
        return gst_pad_event_default(pad, parent, event);
    }
}

static GstStateChangeReturn utuimp_mpa_parse_change_state(GstElement* element, GstStateChange transition) {
    UtuimpMpaParse* self = (UtuimpMpaParse*)element;
    if (transition == GST_STATE_CHANGE_PAUSED_TO_READY) {
        utuimp_mpa_reset(self);
        self->caps_set = FALSE;
    }
    return GST_ELEMENT_CLASS(utuimp_mpa_parse_parent_class)->change_state(element, transition);
}

static void utuimp_mpa_parse_finalize(GObject* object) {
    UtuimpMpaParse* self = (UtuimpMpaParse*)object;
    if (self->pending_segment) gst_event_unref(self->pending_segment);
    if (self->adapter) g_byte_array_free(self->adapter, TRUE);
    G_OBJECT_CLASS(utuimp_mpa_parse_parent_class)->finalize(object);
}

static void utuimp_mpa_parse_class_init(UtuimpMpaParseClass* klass) {
    GObjectClass* gobject_class = G_OBJECT_CLASS(klass);
    GstElementClass* element_class = GST_ELEMENT_CLASS(klass);
    gobject_class->finalize = utuimp_mpa_parse_finalize;
    element_class->change_state = utuimp_mpa_parse_change_state;
    gst_element_class_add_static_pad_template(element_class, &utuimp_mpa_sink_tmpl);
    gst_element_class_add_static_pad_template(element_class, &utuimp_mpa_src_tmpl);
    gst_element_class_set_static_metadata(element_class,
        "utuimp MPEG Audio Parser", "Codec/Parser/Audio",
        "Embedded MP3 frame parser", "utuimp");
}

static void utuimp_mpa_parse_init(UtuimpMpaParse* self) {
    self->adapter = g_byte_array_new();
    self->sinkpad = gst_pad_new_from_static_template(&utuimp_mpa_sink_tmpl, "sink");
    gst_pad_set_chain_function(self->sinkpad, utuimp_mpa_parse_chain);
    gst_pad_set_event_function(self->sinkpad, utuimp_mpa_parse_sink_event);
    gst_element_add_pad(GST_ELEMENT(self), self->sinkpad);
    self->srcpad = gst_pad_new_from_static_template(&utuimp_mpa_src_tmpl, "src");
    gst_pad_use_fixed_caps(self->srcpad);
    gst_element_add_pad(GST_ELEMENT(self), self->srcpad);
}

// ── Embedded GstElement: utuimp_http_src ─────────────────────────────────────
// Connects to the local loopback HTTP stream server and streams the response
// body through coreelements' fdsrc, removing any dependency on souphttpsrc.

typedef struct {
    GstBin parent;
    GstElement* fdsrc;
    gchar* uri;
    int sock_fd;
} UtuimpHttpSrc;

typedef struct {
    GstBinClass parent_class;
} UtuimpHttpSrcClass;

static void utuimp_http_src_uri_handler_init(gpointer g_iface, gpointer iface_data);
static GType utuimp_http_src_get_type(void);

G_DEFINE_TYPE_WITH_CODE(UtuimpHttpSrc, utuimp_http_src, GST_TYPE_BIN,
    G_IMPLEMENT_INTERFACE(GST_TYPE_URI_HANDLER, utuimp_http_src_uri_handler_init))

static GstStaticPadTemplate utuimp_http_src_tmpl = GST_STATIC_PAD_TEMPLATE(
    "src",
    GST_PAD_SRC,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS_ANY
);

static GstURIType utuimp_http_src_uri_get_type(GType type) {
    return GST_URI_SRC;
}

static const gchar* const* utuimp_http_src_uri_get_protocols(GType type) {
    static const gchar* protocols[] = {"http", NULL};
    return protocols;
}

static gchar* utuimp_http_src_uri_get_uri(GstURIHandler* handler) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)handler;
    GST_OBJECT_LOCK(self);
    gchar* r = g_strdup(self->uri);
    GST_OBJECT_UNLOCK(self);
    return r;
}

static gboolean utuimp_http_src_uri_set_uri(GstURIHandler* handler, const gchar* uri, GError** error) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)handler;
    GST_OBJECT_LOCK(self);
    g_free(self->uri);
    self->uri = g_strdup(uri);
    GST_OBJECT_UNLOCK(self);
    return TRUE;
}

static void utuimp_http_src_uri_handler_init(gpointer g_iface, gpointer iface_data) {
    GstURIHandlerInterface* iface = (GstURIHandlerInterface*)g_iface;
    iface->get_type = utuimp_http_src_uri_get_type;
    iface->get_protocols = utuimp_http_src_uri_get_protocols;
    iface->get_uri = utuimp_http_src_uri_get_uri;
    iface->set_uri = utuimp_http_src_uri_set_uri;
}

enum {
    UTUIMP_HTTP_PROP_0,
    UTUIMP_HTTP_PROP_URI
};

static int utuimp_connect_http_stream(const char* uri) {
    if (!uri) return -1;
    const char* p = uri;
    if (strncmp(p, "http://", 7) == 0) p += 7;
    const char* slash = strchr(p, '/');
    const char* path = slash ? slash : "/";
    char hostport[128] = {0};
    size_t hp_len = slash ? (size_t)(slash - p) : strlen(p);
    if (hp_len >= sizeof(hostport)) return -1;
    memcpy(hostport, p, hp_len);

    char host[64] = "127.0.0.1";
    int port = 80;
    char* colon = strchr(hostport, ':');
    if (colon) {
        *colon = '\0';
        snprintf(host, sizeof(host), "%s", hostport);
        port = atoi(colon + 1);
    } else if (hostport[0] != '\0') {
        snprintf(host, sizeof(host), "%s", hostport);
    }
    if (strcmp(host, "localhost") == 0) {
        snprintf(host, sizeof(host), "127.0.0.1");
    }

    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) return -1;

    struct sockaddr_in sin;
    memset(&sin, 0, sizeof(sin));
    sin.sin_family = AF_INET;
    sin.sin_port = htons((uint16_t)port);
    if (inet_pton(AF_INET, host, &sin.sin_addr) <= 0) {
        close(fd);
        return -1;
    }
    if (connect(fd, (struct sockaddr*)&sin, sizeof(sin)) < 0) {
        close(fd);
        return -1;
    }

    gchar* req = g_strdup_printf(
        "GET %s HTTP/1.0\r\nHost: 127.0.0.1:%d\r\nUser-Agent: utuimp/2.0\r\nConnection: close\r\n\r\n",
        path, port);
    ssize_t sent = send(fd, req, strlen(req), 0);
    g_free(req);
    if (sent < 0) {
        close(fd);
        return -1;
    }

    char hdr[4096];
    size_t hlen = 0;
    while (hlen < sizeof(hdr) - 1) {
        ssize_t n = recv(fd, &hdr[hlen], 1, 0);
        if (n <= 0) {
            close(fd);
            return -1;
        }
        hlen++;
        if (hlen >= 4 && memcmp(&hdr[hlen - 4], "\r\n\r\n", 4) == 0) {
            hdr[hlen] = '\0';
            if (strstr(hdr, " 200 ") == NULL) {
                close(fd);
                return -1;
            }
            return fd;
        }
    }
    close(fd);
    return -1;
}

static GstStateChangeReturn utuimp_http_src_change_state(GstElement* element, GstStateChange transition) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)element;
    if (transition == GST_STATE_CHANGE_READY_TO_PAUSED) {
        GST_OBJECT_LOCK(self);
        gchar* u = g_strdup(self->uri);
        GST_OBJECT_UNLOCK(self);

        int fd = utuimp_connect_http_stream(u);
        g_free(u);
        if (fd < 0) {
            GST_ELEMENT_ERROR(self, RESOURCE, OPEN_READ, ("Failed to connect to local audio stream"), (NULL));
            return GST_STATE_CHANGE_FAILURE;
        }
        self->sock_fd = fd;
        g_object_set(G_OBJECT(self->fdsrc), "fd", fd, NULL);
    }

    GstStateChangeReturn ret = GST_ELEMENT_CLASS(utuimp_http_src_parent_class)->change_state(element, transition);

    if (transition == GST_STATE_CHANGE_PAUSED_TO_READY) {
        if (self->sock_fd >= 0) {
            close(self->sock_fd);
            self->sock_fd = -1;
        }
    }
    return ret;
}

static void utuimp_http_src_set_property(GObject* object, guint prop_id, const GValue* value, GParamSpec* pspec) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)object;
    if (prop_id == UTUIMP_HTTP_PROP_URI) {
        utuimp_http_src_uri_set_uri((GstURIHandler*)self, g_value_get_string(value), NULL);
    } else {
        G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    }
}

static void utuimp_http_src_get_property(GObject* object, guint prop_id, GValue* value, GParamSpec* pspec) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)object;
    if (prop_id == UTUIMP_HTTP_PROP_URI) {
        g_value_take_string(value, utuimp_http_src_uri_get_uri((GstURIHandler*)self));
    } else {
        G_OBJECT_WARN_INVALID_PROPERTY_ID(object, prop_id, pspec);
    }
}

static void utuimp_http_src_finalize(GObject* object) {
    UtuimpHttpSrc* self = (UtuimpHttpSrc*)object;
    if (self->sock_fd >= 0) {
        close(self->sock_fd);
        self->sock_fd = -1;
    }
    g_free(self->uri);
    G_OBJECT_CLASS(utuimp_http_src_parent_class)->finalize(object);
}

static void utuimp_http_src_class_init(UtuimpHttpSrcClass* klass) {
    GObjectClass* gobject_class = G_OBJECT_CLASS(klass);
    GstElementClass* element_class = GST_ELEMENT_CLASS(klass);
    gobject_class->set_property = utuimp_http_src_set_property;
    gobject_class->get_property = utuimp_http_src_get_property;
    gobject_class->finalize = utuimp_http_src_finalize;
    element_class->change_state = utuimp_http_src_change_state;
    g_object_class_install_property(gobject_class, UTUIMP_HTTP_PROP_URI,
        g_param_spec_string("uri", "URI", "URI", NULL, G_PARAM_READWRITE | G_PARAM_STATIC_STRINGS));
    gst_element_class_add_static_pad_template(element_class, &utuimp_http_src_tmpl);
    gst_element_class_set_static_metadata(element_class,
        "utuimp Loopback HTTP Source", "Source/Network",
        "Embedded HTTP client source", "utuimp");
}

static void utuimp_http_src_init(UtuimpHttpSrc* self) {
    self->sock_fd = -1;
    self->fdsrc = gst_element_factory_make("fdsrc", "inner-fdsrc");
    gst_bin_add(GST_BIN(self), self->fdsrc);
    GstPad* target = gst_element_get_static_pad(self->fdsrc, "src");
    GstPad* ghost = gst_ghost_pad_new("src", target);
    gst_element_add_pad(GST_ELEMENT(self), ghost);
    gst_object_unref(target);
}
// AAC encoder delay: iTunes-encoded AAC starts with 2112 priming samples
// that ADTS has no field to describe, so the decoder plays them and every
// gapless join carries ~48 ms of near-silence. HE-AAC decoders add SBR delay
// on top; that part varies by decoder and is left alone.
#define UTUIMP_AAC_PRIMING 2112

// A start_seg stream begins mid-track (a seek), past the priming. Only the
// .aac streams carry AAC priming; library uploads are served as .mp3 or .m4a.
static gboolean utuimp_uri_is_whole_track(const gchar* uri) {
    return uri && strstr(uri, ".aac") && !strstr(uri, "start_seg=");
}

// on_filter_event moves each whole-track stream's segment start past the
// priming, so the sink drops those samples. Runs on the streaming thread.
static GstPadProbeReturn on_filter_event(GstPad* pad, GstPadProbeInfo* info, gpointer user_data) {
    UtuimpGstState* st = (UtuimpGstState*)user_data;
    GstEvent* ev = GST_PAD_PROBE_INFO_EVENT(info);
    if (GST_EVENT_TYPE(ev) == GST_EVENT_STREAM_START) {
        g_mutex_lock(&st->mu);
        st->trim_cur = st->trim_next;
        st->trim_next = FALSE;
        g_mutex_unlock(&st->mu);
        return GST_PAD_PROBE_OK;
    }
    if (GST_EVENT_TYPE(ev) != GST_EVENT_SEGMENT)
        return GST_PAD_PROBE_OK;

    const GstSegment* seg = NULL;
    gst_event_parse_segment(ev, &seg);
    // A base of zero is a fresh start (new URI or flush), even for a stream
    // that is not trimmed, such as a seek's start_seg stream.
    if (seg->base == 0) st->trimmed = 0;
    if (!st->trim_cur)
        return GST_PAD_PROBE_OK;
    gint rate = 0;
    GstCaps* caps = gst_pad_get_current_caps(pad);
    if (caps) {
        gst_structure_get_int(gst_caps_get_structure(caps, 0), "rate", &rate);
        gst_caps_unref(caps);
    }
    if (seg->format != GST_FORMAT_TIME || rate <= 0)
        return GST_PAD_PROBE_OK;
    st->trim_cur = FALSE;

    GstSegment s2;
    gst_segment_copy_into(seg, &s2);
    guint64 delay = gst_util_uint64_scale_int(UTUIMP_AAC_PRIMING, GST_SECOND, rate);
    // Upstream places each gapless stream at the untrimmed end of the
    // previous one, so pull it back by everything cut so far.
    s2.base -= MIN(s2.base, st->trimmed);
    st->trimmed += delay;
    s2.start += delay;
    if (s2.position < s2.start) s2.position = s2.start;
    GstEvent* nev = gst_event_new_segment(&s2);
    gst_event_set_seqnum(nev, gst_event_get_seqnum(ev));
    gst_event_unref(ev);
    GST_PAD_PROBE_INFO_DATA(info) = nev;
    return GST_PAD_PROBE_OK;
}

static void on_about_to_finish(GstElement* playbin, gpointer user_data) {
    UtuimpGstState* st = (UtuimpGstState*)user_data;
    if (!st) return;
    gchar* uri = NULL;
    g_mutex_lock(&st->mu);
    if (st->next_uri && st->next_uri[0] != '\0') {
        uri = st->next_uri;
        st->next_uri = NULL;
    }
    if (uri) st->trim_next = utuimp_uri_is_whole_track(uri);
    g_mutex_unlock(&st->mu);
    if (!uri) return;

    g_object_set(G_OBJECT(playbin), "uri", uri, NULL);
    GstStructure* s = gst_structure_new("utuimp-about-to-finish",
        "uri", G_TYPE_STRING, uri, NULL);
    gst_element_post_message(playbin, gst_message_new_application(GST_OBJECT(playbin), s));
    g_free(uri);
}

static UtuimpGstState* utuimp_gst_state_new(void) {
    UtuimpGstState* st = g_new0(UtuimpGstState, 1);
    g_mutex_init(&st->mu);
    return st;
}

static void utuimp_gst_state_set_next_uri(UtuimpGstState* st, const char* uri) {
    if (!st) return;
    g_mutex_lock(&st->mu);
    g_free(st->next_uri);
    st->next_uri = (uri && uri[0] != '\0') ? g_strdup(uri) : NULL;
    g_mutex_unlock(&st->mu);
}

static void utuimp_gst_state_free(UtuimpGstState* st) {
    if (!st) return;
    g_mutex_lock(&st->mu);
    g_free(st->next_uri);
    st->next_uri = NULL;
    g_mutex_unlock(&st->mu);
    g_mutex_clear(&st->mu);
    g_free(st);
}
static void utuimp_gst_init(void) {
    if (!gst_is_initialized())
        gst_init(NULL, NULL);
    // utuimpaacparse outranks aacparse (PRIMARY + 1). aacparse stays in
    // lost-sync mode after a stream starts and drops a whole frame whenever a
    // read ends a few bytes into the next ADTS header, which loses several
    // 23 ms frames at the start of every track. Our stream always comes from
    // our own server as clean ADTS, which utuimpaacparse frames exactly.
    gst_element_register(NULL, "utuimpaacparse", GST_RANK_PRIMARY + 2, utuimp_aac_parse_get_type());
    // utuimpmpaparse frames MP3 when gst-plugins-good's mpegaudioparse and
    // id3demux are missing. With them installed it must not exist at all: it
    // also takes files that start with an ID3 tag, and decodebin tries
    // parsers before demuxers whatever their rank, so it would be plugged
    // in front of the real ones. It starts every segment at zero and knows
    // nothing of seeking, so a seek in such a file landed in the wrong place
    // and the position restarted from 0:00.
    GstElementFactory* mpa = gst_element_factory_find("mpegaudioparse");
    GstElementFactory* id3 = gst_element_factory_find("id3demux");
    if (!mpa || !id3)
        gst_element_register(NULL, "utuimpmpaparse", GST_RANK_MARGINAL + 1, utuimp_mpa_parse_get_type());
    if (mpa) gst_object_unref(mpa);
    if (id3) gst_object_unref(id3);
    // utuimphttpsrc only takes over when souphttpsrc (gst-plugins-good) is absent.
    gst_element_register(NULL, "utuimphttpsrc", GST_RANK_MARGINAL + 1, utuimp_http_src_get_type());
}

static int utuimp_gst_has_element(const char* name) {
    GstElementFactory* f = gst_element_factory_find(name);
    if (f) {
        gst_object_unref(f);
        return 1;
    }
    return 0;
}

static void utuimp_gst_set_element_rank(const char* name, int rank) {
    GstElementFactory* f = gst_element_factory_find(name);
    if (f) {
        gst_plugin_feature_set_rank(GST_PLUGIN_FEATURE(f), (guint)rank);
        gst_object_unref(f);
    }
}

static int utuimp_gst_has_aac_decoder(void) {
    const char* candidates[] = {"avdec_aac", "faad", "fdkaacdec", NULL};
    for (int i = 0; candidates[i] != NULL; i++) {
        if (utuimp_gst_has_element(candidates[i])) {
            return 1;
        }
    }
    return 0;
}

// on_filter_buffer applies the built-in 10-band biquad equalizer when
// gstreamer1.0-plugins-good's equalizer-10bands is not installed.
static GstPadProbeReturn on_filter_buffer(GstPad* pad, GstPadProbeInfo* info, gpointer user_data) {
    UtuimpGstState* st = (UtuimpGstState*)user_data;
    GstBuffer* buf = GST_PAD_PROBE_INFO_BUFFER(info);
    if (!buf) return GST_PAD_PROBE_OK;

    gint rate = 0, channels = 0;
    const gchar* fmt = NULL;
    GstCaps* caps = gst_pad_get_current_caps(pad);
    if (caps) {
        GstStructure* s = gst_caps_get_structure(caps, 0);
        gst_structure_get_int(s, "rate", &rate);
        gst_structure_get_int(s, "channels", &channels);
        fmt = gst_structure_get_string(s, "format");
        gst_caps_unref(caps);
    }
    if (rate <= 0 || channels <= 0 || channels > 2 || !fmt)
        return GST_PAD_PROBE_OK;

    g_mutex_lock(&st->mu);
    if (st->eq_ver != st->eq_seen || st->eq_rate != rate) {
        for (int b = 0; b < UTUIMP_EQ_BANDS; b++) {
            utuimp_biquad_design(&st->eq_bank[b], (double)rate, utuimp_eq_freqs[b], st->eq_gains[b]);
        }
        st->eq_seen = st->eq_ver;
        st->eq_rate = rate;
    }
    gboolean any_active = FALSE;
    for (int b = 0; b < UTUIMP_EQ_BANDS; b++) {
        if (st->eq_bank[b].active) {
            any_active = TRUE;
            break;
        }
    }
    if (!any_active) {
        g_mutex_unlock(&st->mu);
        return GST_PAD_PROBE_OK;
    }

    buf = gst_buffer_make_writable(buf);
    GST_PAD_PROBE_INFO_DATA(info) = buf;
    GstMapInfo map;
    if (gst_buffer_map(buf, &map, GST_MAP_READWRITE)) {
        if (strcmp(fmt, "S16LE") == 0) {
            gint16* samples = (gint16*)map.data;
            size_t frames = map.size / (sizeof(gint16) * (size_t)channels);
            for (size_t f = 0; f < frames; f++) {
                for (int c = 0; c < channels; c++) {
                    double x = (double)samples[f * (size_t)channels + (size_t)c];
                    for (int b = 0; b < UTUIMP_EQ_BANDS; b++) {
                        if (st->eq_bank[b].active) x = utuimp_biquad_run(&st->eq_bank[b], x, c);
                    }
                    if (x > 32767.0) x = 32767.0;
                    if (x < -32768.0) x = -32768.0;
                    samples[f * (size_t)channels + (size_t)c] = (gint16)x;
                }
            }
        } else if (strcmp(fmt, "F32LE") == 0) {
            gfloat* samples = (gfloat*)map.data;
            size_t frames = map.size / (sizeof(gfloat) * (size_t)channels);
            for (size_t f = 0; f < frames; f++) {
                for (int c = 0; c < channels; c++) {
                    double x = (double)samples[f * (size_t)channels + (size_t)c];
                    for (int b = 0; b < UTUIMP_EQ_BANDS; b++) {
                        if (st->eq_bank[b].active) x = utuimp_biquad_run(&st->eq_bank[b], x, c);
                    }
                    if (x > 1.0) x = 1.0;
                    if (x < -1.0) x = -1.0;
                    samples[f * (size_t)channels + (size_t)c] = (gfloat)x;
                }
            }
        }
        gst_buffer_unmap(buf, &map);
    }
    g_mutex_unlock(&st->mu);
    return GST_PAD_PROBE_OK;
}

// ── Sample tap (for the visualizer) ──────────────────────────────────────────
// Every pipeline copies what it plays, mixed down to mono, into one shared
// ring. Only one track plays at a time, so sharing it across pipelines is
// fine; a pipeline that stops writing simply goes stale.

// Long enough for the sink's buffer (over a second with PipeWire) at high
// sample rates: the sound being heard must still be in the ring.
#define UTUIMP_TAP_LEN (1 << 19)

static GMutex utuimp_tap_mu;
static float utuimp_tap_buf[UTUIMP_TAP_LEN];
static guint64 utuimp_tap_written;
static gint utuimp_tap_rate;
static gint64 utuimp_tap_time; // monotonic µs of the last write
static gint64 utuimp_tap_heard; // monotonic µs at which the last written sample is heard; 0: unknown

// When sample 0 of the ring would have been heard (monotonic µs): while the
// sound plays on without a break this is one fixed moment, and sample n is
// heard n/rate after it. Each buffer gives a reading of it from the
// pipeline's clock, but that clock is not steady: for the first ten seconds
// or so of a stream the audio server is still working out its own timing,
// and the readings wander late by up to 80 ms and back. So while a stretch
// is young a low reading of it is trusted (the wandering only goes late);
// after that, the middle of the latest ones.
#define UTUIMP_TAP_OBS 256
#define UTUIMP_TAP_SETTLE 240 // readings (20 a second) until the clock is trusted
static gint64 utuimp_tap_obs[UTUIMP_TAP_OBS];
static int utuimp_tap_obs_n;      // readings taken in this stretch
static gint64 utuimp_tap_obs_at;  // when the last one was taken
static gint64 utuimp_tap_origin;  // the trusted value
static gboolean utuimp_tap_origin_ok;
static int utuimp_tap_off;        // buffers in a row that disagree with it

static int utuimp_cmp_i64(const void* a, const void* b) {
    gint64 x = *(const gint64*)a, y = *(const gint64*)b;
    return x < y ? -1 : x > y;
}

// utuimp_tap_observe takes one reading (call with the lock held): origin is
// when sample 0 would be heard according to the buffer just written.
static void utuimp_tap_observe(gint64 origin, gint64 now) {
    if (utuimp_tap_origin_ok) {
        gint64 d = origin - utuimp_tap_origin;
        if (d < -150000 || d > 150000) {
            // A pause, a seek or a new stream moved it: start over, once a
            // few buffers agree that it is not a glitch.
            if (++utuimp_tap_off >= 3) {
                utuimp_tap_obs_n = 0;
                utuimp_tap_origin_ok = FALSE;
            } else {
                return;
            }
        } else {
            utuimp_tap_off = 0;
        }
    }
    if (utuimp_tap_origin_ok && now - utuimp_tap_obs_at < 50000) return;
    utuimp_tap_obs[utuimp_tap_obs_n % UTUIMP_TAP_OBS] = origin;
    utuimp_tap_obs_n++;
    utuimp_tap_obs_at = now;
    utuimp_tap_off = 0;
    gint64 sorted[UTUIMP_TAP_OBS];
    int n;
    if (utuimp_tap_obs_n <= UTUIMP_TAP_SETTLE) {
        // The stream is young and the clock still wanders: a low reading of
        // all there are.
        n = utuimp_tap_obs_n;
        memcpy(sorted, utuimp_tap_obs, sizeof(gint64) * (size_t)n);
        qsort(sorted, (size_t)n, sizeof(gint64), utuimp_cmp_i64);
        utuimp_tap_origin = sorted[n / 5];
    } else {
        // The clock has settled and reads true within a few ms: the middle
        // of the last two seconds. That follows a real shift quickly: a
        // stream that stalled for a moment (the network, for one) plays
        // everything after it that much later.
        n = 40;
        for (int i = 0; i < n; i++)
            sorted[i] = utuimp_tap_obs[(utuimp_tap_obs_n - 1 - i) % UTUIMP_TAP_OBS];
        qsort(sorted, (size_t)n, sizeof(gint64), utuimp_cmp_i64);
        utuimp_tap_origin = sorted[n / 2];
    }
    utuimp_tap_origin_ok = TRUE;
}

// utuimp_tap_heard_at tells when the start of buf will come out of the
// speakers, as a monotonic time in µs, or 0 when that can't be told. The
// sink plays a buffer when the pipeline's clock reaches the buffer's running
// time, so the wait is the difference between the two.
static gint64 utuimp_tap_heard_at(GstPad* pad, GstBuffer* buf) {
    GstClockTime pts = GST_BUFFER_PTS(buf);
    if (!GST_CLOCK_TIME_IS_VALID(pts)) return 0;
    gint64 heard = 0;
    GstEvent* ev = gst_pad_get_sticky_event(pad, GST_EVENT_SEGMENT, 0);
    GstElement* el = gst_pad_get_parent_element(pad);
    GstClock* clock = el ? gst_element_get_clock(el) : NULL;
    if (ev && clock) {
        const GstSegment* seg = NULL;
        gst_event_parse_segment(ev, &seg);
        if (seg && seg->format == GST_FORMAT_TIME) {
            guint64 rt = gst_segment_to_running_time(seg, GST_FORMAT_TIME, pts);
            GstClockTime now = gst_clock_get_time(clock);
            GstClockTime base = gst_element_get_base_time(el);
            if (rt != (guint64)-1 && GST_CLOCK_TIME_IS_VALID(now) && now >= base) {
                gint64 wait_us = ((gint64)rt - (gint64)(now - base)) / 1000;
                // Anything else is a clock that isn't running yet, or a seek
                // in flight: better no answer than a wild one.
                if (wait_us > -100000 && wait_us < 3000000) heard = g_get_monotonic_time() + wait_us;
            }
        }
    }
    if (clock) gst_object_unref(clock);
    if (el) gst_object_unref(el);
    if (ev) gst_event_unref(ev);
    return heard;
}

static GstPadProbeReturn on_tap_buffer(GstPad* pad, GstPadProbeInfo* info, gpointer user_data) {
    GstBuffer* buf = GST_PAD_PROBE_INFO_BUFFER(info);
    if (!buf) return GST_PAD_PROBE_OK;
    gint rate = 0, channels = 0;
    const gchar* fmt = NULL;
    GstCaps* caps = gst_pad_get_current_caps(pad);
    if (!caps) return GST_PAD_PROBE_OK;
    GstStructure* s = gst_caps_get_structure(caps, 0);
    gst_structure_get_int(s, "rate", &rate);
    gst_structure_get_int(s, "channels", &channels);
    fmt = gst_structure_get_string(s, "format");
    if (rate <= 0 || channels <= 0 || !fmt) {
        gst_caps_unref(caps);
        return GST_PAD_PROBE_OK;
    }
    int kind = 0;
    if (strcmp(fmt, "S16LE") == 0) kind = 1;
    else if (strcmp(fmt, "F32LE") == 0) kind = 2;
    else if (strcmp(fmt, "S32LE") == 0) kind = 3;
    else if (strcmp(fmt, "F64LE") == 0) kind = 4;
    gst_caps_unref(caps);
    if (kind == 0) return GST_PAD_PROBE_OK;

    GstMapInfo map;
    if (!gst_buffer_map(buf, &map, GST_MAP_READ)) return GST_PAD_PROBE_OK;
    size_t width = kind == 1 ? 2 : kind == 4 ? 8 : 4;
    size_t frames = map.size / (width * (size_t)channels);
    gint64 heard = utuimp_tap_heard_at(pad, buf);
    if (heard != 0) heard += (gint64)frames * 1000000 / rate;
    gint64 now = g_get_monotonic_time();
    g_mutex_lock(&utuimp_tap_mu);
    utuimp_tap_heard = heard;
    if (utuimp_tap_rate != rate || now - utuimp_tap_time > 300000) {
        // Another stream, or the sound stopped for a while.
        utuimp_tap_obs_n = 0;
        utuimp_tap_origin_ok = FALSE;
    }
    if (utuimp_tap_rate != rate) {
        utuimp_tap_rate = rate;
        utuimp_tap_written = 0;
    }
    for (size_t f = 0; f < frames; f++) {
        double sum = 0.0;
        for (int c = 0; c < channels; c++) {
            size_t i = f * (size_t)channels + (size_t)c;
            switch (kind) {
            case 1: sum += ((const gint16*)map.data)[i] / 32768.0; break;
            case 2: sum += ((const gfloat*)map.data)[i]; break;
            case 3: sum += ((const gint32*)map.data)[i] / 2147483648.0; break;
            default: sum += ((const gdouble*)map.data)[i]; break;
            }
        }
        utuimp_tap_buf[utuimp_tap_written % UTUIMP_TAP_LEN] = (float)(sum / channels);
        utuimp_tap_written++;
    }
    utuimp_tap_time = now;
    if (heard != 0) {
        utuimp_tap_observe(heard - (gint64)((double)utuimp_tap_written * 1e6 / rate), now);
    }
    g_mutex_unlock(&utuimp_tap_mu);
    gst_buffer_unmap(buf, &map);
    return GST_PAD_PROBE_OK;
}


// utuimp_tap_read copies the n samples that end ahead_s seconds after the
// one being heard now into dst and returns the sample rate, or 0 when
// nothing is playing; got_ahead is how far ahead they really end (less than
// asked when the sink holds less than that). The tap sits before the audio sink, so it runs ahead
// of the sound: by how much is known from the pipeline's clock (see
// utuimp_tap_heard_at), or taken to be lead_s when it isn't.
static int utuimp_tap_read(float* dst, int n, double lead_s, double ahead_s, double* got_ahead, gint64* got_end) {
    g_mutex_lock(&utuimp_tap_mu);
    gint rate = utuimp_tap_rate;
    guint64 written = utuimp_tap_written;
    gint64 now = g_get_monotonic_time();
    gint64 age = now - utuimp_tap_time;
    gint64 heard = utuimp_tap_heard;
    if (rate <= 0 || written == 0 || age > 400000 || n <= 0 || n > UTUIMP_TAP_LEN / 2) {
        g_mutex_unlock(&utuimp_tap_mu);
        return 0;
    }
    double end = (double)written - lead_s * rate + (double)age * rate / 1e6;
    if (heard != 0) end = (double)written - (double)(heard - now) * rate / 1e6;
    if (utuimp_tap_origin_ok) end = (double)(now - utuimp_tap_origin) * rate / 1e6;
    double heard_now = end;
    end += ahead_s * rate;
    if (end > (double)written) end = (double)written; // no more than there is
    *got_ahead = (end - heard_now) / rate;
    gint64 e = (gint64)end;
    *got_end = e;
    gint64 oldest = (gint64)written - UTUIMP_TAP_LEN;
    for (int i = 0; i < n; i++) {
        gint64 idx = e - n + i;
        dst[i] = (idx < 0 || idx < oldest) ? 0.0f : utuimp_tap_buf[(guint64)idx % UTUIMP_TAP_LEN];
    }
    g_mutex_unlock(&utuimp_tap_mu);
    return rate;
}

// ── Raw PCM player ───────────────────────────────────────────────────────────
// Plays 16-bit 44.1 kHz stereo PCM handed over in pieces (librespot's
// output, for Spotify), through the same sample tap as everything else.

typedef struct {
    GstElement* pipeline;
    GstElement* src;
    GstClockTime next; // running time at which the next sample pushed plays
} UtuimpRaw;

static UtuimpRaw* utuimp_raw_new(void) {
    const char* null_env = getenv("UTUIMP_NULL_AUDIO");
    const char* sink = "autoaudiosink";
    if (null_env && strcmp(null_env, "1") == 0) {
        sink = "fakesink sync=true";
    } else if (utuimp_gst_has_element("pulsesink")) {
        // Little is held ahead of the speakers, so a pause that comes from
        // outside (the phone) is heard soon; but enough for the visualizer
        // to look ahead.
        sink = "pulsesink buffer-time=200000 latency-time=20000";
    }
    gchar* desc = g_strdup_printf(
        "appsrc name=utuimpraw format=time block=true max-bytes=17640 "
        "caps=audio/x-raw,format=S16LE,rate=44100,channels=2,layout=interleaved "
        "! audioconvert ! audioresample ! identity name=utuimptap "
        // A quarter of a second waits here, past the tap: that is the
        // visualizer's look ahead, whatever the sink holds.
        "! queue max-size-time=250000000 max-size-bytes=0 max-size-buffers=0 ! %s", sink);
    GError* err = NULL;
    GstElement* p = gst_parse_launch(desc, &err);
    g_free(desc);
    if (err) g_error_free(err);
    if (!p) return NULL;
    GstElement* src = gst_bin_get_by_name(GST_BIN(p), "utuimpraw");
    GstElement* tap = gst_bin_get_by_name(GST_BIN(p), "utuimptap");
    if (!src || !tap) {
        if (src) gst_object_unref(src);
        if (tap) gst_object_unref(tap);
        gst_object_unref(p);
        return NULL;
    }
    GstPad* tp = gst_element_get_static_pad(tap, "src");
    if (tp) {
        gst_pad_add_probe(tp, GST_PAD_PROBE_TYPE_BUFFER, on_tap_buffer, NULL, NULL);
        gst_object_unref(tp);
    }
    gst_object_unref(tap);
    UtuimpRaw* r = g_new0(UtuimpRaw, 1);
    r->pipeline = p;
    r->src = src;
    gst_element_set_state(p, GST_STATE_PLAYING);
    return r;
}

// utuimp_raw_push hands over len bytes of samples; it waits while the
// player has enough in hand, which is what paces the source. The samples are
// stamped to follow one another without a gap, except after the source has
// been silent for a while (paused, or between songs): then they are due at
// once, not at the moment they would have been without the silence, which is
// long past and would get them thrown away as late.
static int utuimp_raw_push(UtuimpRaw* r, const void* data, int len) {
    GstBuffer* buf = gst_buffer_new_allocate(NULL, (gsize)len, NULL);
    if (!buf) return -1;
    gst_buffer_fill(buf, 0, data, (gsize)len);
    GstState st = GST_STATE_NULL;
    gst_element_get_state(r->pipeline, &st, NULL, 0);
    GstClock* clock = gst_element_get_clock(r->pipeline);
    if (clock) {
        GstClockTime now = gst_clock_get_time(clock);
        GstClockTime base = gst_element_get_base_time(r->pipeline);
        if (st == GST_STATE_PLAYING && GST_CLOCK_TIME_IS_VALID(now) && now > base) {
            GstClockTime running = now - base;
            if (running > r->next + 60 * GST_MSECOND) {
                r->next = running + 30 * GST_MSECOND;
                GST_BUFFER_FLAG_SET(buf, GST_BUFFER_FLAG_DISCONT);
            }
        }
        gst_object_unref(clock);
    }
    GstClockTime dur = gst_util_uint64_scale((guint64)(len / 4), GST_SECOND, 44100);
    GST_BUFFER_PTS(buf) = r->next;
    GST_BUFFER_DURATION(buf) = dur;
    r->next += dur;
    GstFlowReturn ret = GST_FLOW_ERROR;
    g_signal_emit_by_name(r->src, "push-buffer", buf, &ret);
    gst_buffer_unref(buf);
    return (int)ret;
}

// utuimp_raw_flush throws away what has been handed over but not played yet
// (a skip or a seek: the old song must stop at once). Call it from the
// thread that pushes.
static void utuimp_raw_flush(UtuimpRaw* r) {
    gst_element_send_event(r->src, gst_event_new_flush_start());
    gst_element_send_event(r->src, gst_event_new_flush_stop(TRUE));
    r->next = 0;
}

static void utuimp_raw_play(UtuimpRaw* r)  { gst_element_set_state(r->pipeline, GST_STATE_PLAYING); }
static void utuimp_raw_pause(UtuimpRaw* r) { gst_element_set_state(r->pipeline, GST_STATE_PAUSED); }
static void utuimp_raw_stop(UtuimpRaw* r)  { gst_element_set_state(r->pipeline, GST_STATE_NULL); }

// utuimp_raw_restart starts the pipeline from nothing, as on its first day:
// after it was put to rest (state NULL), or to get it out of whatever state
// a long silence left the sound server's clock in.
static void utuimp_raw_restart(UtuimpRaw* r) {
    gst_element_set_state(r->pipeline, GST_STATE_NULL);
    r->next = 0;
    gst_element_set_state(r->pipeline, GST_STATE_PLAYING);
}

static void utuimp_raw_free(UtuimpRaw* r) {
    gst_object_unref(r->src);
    gst_object_unref(r->pipeline);
    g_free(r);
}

// utuimp_gst_new creates a playbin3 pipeline, falling back to playbin, and
// attaches a 10-band equalizer audio-filter when gstreamer1.0-plugins-good is
// installed (an identity filter with embedded biquad DSP otherwise, to carry
// both the priming trim and the 10-band EQ).
static GstElement* utuimp_gst_new(GstElement** eqOut, UtuimpGstState* st) {
    GstElement* p = gst_element_factory_make("playbin3", "utuimp-audio");
    if (!p) p = gst_element_factory_make("playbin", "utuimp-audio");
    if (p) {
        g_signal_connect(p, "about-to-finish", G_CALLBACK(on_about_to_finish), (gpointer)st);
        const char* null_env = getenv("UTUIMP_NULL_AUDIO");
        if (null_env && strcmp(null_env, "1") == 0) {
            GstElement* fake = gst_element_factory_make("fakesink", "utuimp-nullsink");
            if (fake) {
                g_object_set(G_OBJECT(fake), "sync", (gboolean)TRUE, NULL);
                g_object_set(G_OBJECT(p), "audio-sink", fake, NULL);
            }
        } else if (!utuimp_gst_has_element("autoaudiosink")) {
            // autoaudiosink ships in gst-plugins-good (libgstautodetect). When
            // it is absent, explicitly select pipewiresink, pulsesink, or
            // alsasink (which ships in gst-plugins-base).
            const char* sinks[] = {"pipewiresink", "pulsesink", "alsasink", NULL};
            for (int i = 0; sinks[i] != NULL; i++) {
                GstElement* sink = gst_element_factory_make(sinks[i], "utuimp-sink");
                if (sink) {
                    g_object_set(G_OBJECT(p), "audio-sink", sink, NULL);
                    break;
                }
            }
        }
        GstElement* filter = NULL;
        if (eqOut) {
            filter = gst_element_factory_make("equalizer-10bands", "utuimp-eq");
            *eqOut = filter;
        }
        // When equalizer-10bands is unavailable, identity carries both the
        // priming trim event probe and the embedded 10-band biquad buffer probe.
        gboolean use_embedded_eq = (filter == NULL);
        if (!filter) filter = gst_element_factory_make("identity", "utuimp-filter");
        if (filter) {
            g_object_set(G_OBJECT(p), "audio-filter", filter, NULL);
            GstPad* sp = gst_element_get_static_pad(filter, "sink");
            if (sp) {
                gst_pad_add_probe(sp, GST_PAD_PROBE_TYPE_EVENT_DOWNSTREAM, on_filter_event, st, NULL);
                if (use_embedded_eq) {
                    gst_pad_add_probe(sp, GST_PAD_PROBE_TYPE_BUFFER, on_filter_buffer, st, NULL);
                }
                gst_object_unref(sp);
            }
            GstPad* tp = gst_element_get_static_pad(filter, "src");
            if (tp) {
                gst_pad_add_probe(tp, GST_PAD_PROBE_TYPE_BUFFER, on_tap_buffer, NULL, NULL);
                gst_object_unref(tp);
            }
        }
    }
    return p;
}

static void utuimp_gst_set_eq_band(GstElement* eq, UtuimpGstState* st, gint idx, gdouble gain) {
    if (idx < 0 || idx >= UTUIMP_EQ_BANDS) return;
    if (gain < -24.0) gain = -24.0;
    if (gain > 12.0) gain = 12.0;
    if (st) {
        g_mutex_lock(&st->mu);
        st->eq_gains[idx] = gain;
        st->eq_ver++;
        g_mutex_unlock(&st->mu);
    }
    if (eq) {
        char prop[8];
        g_snprintf(prop, sizeof(prop), "band%d", idx);
        g_object_set(G_OBJECT(eq), prop, gain, NULL);
    }
}

static void utuimp_gst_set_uri(GstElement* p, UtuimpGstState* st, const char* uri) {
    // Use READY (not NULL) to release the current media while keeping the
    // pipeline and its PipeWire/PulseAudio connection alive. Going to NULL
    // would tear down the audio sink and create a new OS-level player entry
    // on every track change, causing dozens of stale entries in notification
    // centres that track MPRIS/PipeWire streams.
    gst_element_set_state(p, GST_STATE_READY);
    g_mutex_lock(&st->mu);
    st->trim_next = utuimp_uri_is_whole_track(uri);
    g_mutex_unlock(&st->mu);
    g_object_set(G_OBJECT(p), "uri", uri, NULL);
}

static void utuimp_gst_play(GstElement* p)  { gst_element_set_state(p, GST_STATE_PLAYING); }
static void utuimp_gst_pause(GstElement* p) { gst_element_set_state(p, GST_STATE_PAUSED);  }
static void utuimp_gst_stop(GstElement* p)  { gst_element_set_state(p, GST_STATE_NULL);    }

static void utuimp_gst_seek(GstElement* p, gint64 pos_ns) {
    gst_element_seek_simple(p, GST_FORMAT_TIME,
        (GstSeekFlags)(GST_SEEK_FLAG_FLUSH | GST_SEEK_FLAG_KEY_UNIT), pos_ns);
}

static void utuimp_gst_set_volume(GstElement* p, gdouble v) {
    g_object_set(G_OBJECT(p), "volume", v, NULL);
}

static gint64 utuimp_gst_position(GstElement* p) {
    gint64 v = GST_CLOCK_TIME_NONE;
    gst_element_query_position(p, GST_FORMAT_TIME, &v);
    return v;
}

static gint64 utuimp_gst_duration(GstElement* p) {
    gint64 v = GST_CLOCK_TIME_NONE;
    gst_element_query_duration(p, GST_FORMAT_TIME, &v);
    return v;
}

// utuimp_gst_pop_bus pops one message from the pipeline bus.
// Returns the GstMessageType (0 = no message).
// For ERROR or APPLICATION messages, *strOut is set to a g_strdup'd string (caller frees).
static gint utuimp_gst_pop_bus(GstElement* p, gchar** strOut) {
    GstBus* bus = gst_element_get_bus(p);
    if (!bus) return 0;
    GstMessage* msg = gst_bus_pop(bus);
    gst_object_unref(bus);
    if (!msg) return 0;
    gint t = (gint)GST_MESSAGE_TYPE(msg);
    if (t == (gint)GST_MESSAGE_STREAM_START) {
        if (GST_MESSAGE_SRC(msg) != GST_OBJECT(p)) {
            t = -1; // ignore internal child element stream-start
        }
    } else if (t == (gint)GST_MESSAGE_APPLICATION && strOut) {
        const GstStructure* s = gst_message_get_structure(msg);
        if (s && gst_structure_has_name(s, "utuimp-about-to-finish")) {
            const gchar* u = gst_structure_get_string(s, "uri");
            if (u) *strOut = g_strdup(u);
        } else {
            t = -1;
        }
    } else if (t == (gint)GST_MESSAGE_TAG && strOut) {
        GstTagList* tags = NULL;
        gchar *title = NULL, *artist = NULL, *album = NULL, *station = NULL;
        gst_message_parse_tag(msg, &tags);
        if (tags) {
            gst_tag_list_get_string(tags, GST_TAG_TITLE, &title);
            gst_tag_list_get_string(tags, GST_TAG_ARTIST, &artist);
            gst_tag_list_get_string(tags, GST_TAG_ALBUM, &album);
            gst_tag_list_get_string(tags, GST_TAG_ORGANIZATION, &station);
            if (title || artist || album || station) {
                *strOut = g_strdup_printf("%s\x1f%s\x1f%s\x1f%s", title ? title : "", artist ? artist : "", album ? album : "", station ? station : "");
            }
            g_free(title); g_free(artist); g_free(album); g_free(station);
            gst_tag_list_unref(tags);
        }
    } else if (t == (gint)GST_MESSAGE_ERROR && strOut) {
        GError* e = NULL;
        gchar* dbg = NULL;
        gst_message_parse_error(msg, &e, &dbg);
        if (e) {
            if (dbg && dbg[0] != '\0') {
                *strOut = g_strdup_printf("%s [%s]", e->message, dbg);
            } else {
                *strOut = g_strdup(e->message);
            }
            g_error_free(e);
        }
        g_free(dbg);
    }
    gst_message_unref(msg);
    return t;
}

static void utuimp_gst_free(gchar* s) { g_free(s); }

static void utuimp_gst_destroy(GstElement* p) {
    gst_element_set_state(p, GST_STATE_NULL);
    gst_object_unref(p);
}
*/
import "C"

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tphx37/utuimp/internal/player/tap"
)

// tapLead is how far the audio sink plays behind the tap (its buffer), used
// only when the pipeline's clock can't tell.
const tapLead = 0.2

// tapAhead is how far ahead of the sound the visualizer may read (the sink
// holds a second or so that hasn't been played yet): enough to find a hit
// before it sounds and show it exactly on time.
const tapAhead = 0.18

// readTap fills dst with mono samples ending ahead seconds after the sound
// being heard now and returns their sample rate, or 0 when nothing is playing.
func readTap(dst []float32) (rate int, ahead float64, end int64) {
	if len(dst) == 0 {
		return 0, 0, 0
	}
	var got C.double
	var e C.gint64
	rate = int(C.utuimp_tap_read((*C.float)(unsafe.Pointer(&dst[0])), C.int(len(dst)), C.double(tapLead), C.double(tapAhead), &got, &e))
	return rate, float64(got), int64(e)
}

const aacDecoderHelp = "install gstreamer1.0-libav or gstreamer1.0-plugins-bad (Debian/Ubuntu) / gst-libav or gst-plugins-bad (Arch/Fedora)"

const goodPluginsHelp = "FLAC, M4A and WAV files need gst-plugins-good: install gstreamer1.0-plugins-good (Debian/Ubuntu) / gst-plugins-good (Arch/Fedora)"

// missingGoodPlugins reports whether the demuxers local files need
// (gst-plugins-good) are absent.
func missingGoodPlugins() bool {
	return !hasElement("qtdemux") || !hasElement("wavparse") || !hasElement("flacparse")
}

const aiffParserHelp = "24/32-bit AIFF playback needs aiffparse: install gstreamer1.0-plugins-bad (Debian/Ubuntu) / gst-plugins-bad (Arch/Fedora)"

var once sync.Once

// initGst starts GStreamer and hands the visualizer the sample tap. Every
// entry point goes through it: whichever comes first (a plugin check, a
// player, the raw player) must leave the tap registered, or the visualizer
// hears nothing for the whole session.
func initGst() {
	once.Do(func() {
		C.utuimp_gst_init()
		tap.Register(readTap)
	})
}

// Player wraps a GStreamer playbin3 pipeline for URI-based audio playback.
type Player struct {
	mu            sync.Mutex
	pipeline      *C.GstElement
	eq            *C.GstElement
	st            *C.UtuimpGstState
	committedURI  string
	onEOS         func()
	onError       func(error)
	onStreamStart func(string)
	onTags        func(title, artist, album, station string)
	stop          chan struct{}
	destroyOnce   sync.Once
	wg            sync.WaitGroup
}

// New initialises GStreamer once and returns a ready Player.
func New() (*Player, error) {
	initGst()
	st := C.utuimp_gst_state_new()
	p := &Player{
		st:   st,
		stop: make(chan struct{}),
	}

	var eq *C.GstElement
	pl := C.utuimp_gst_new(&eq, st) //nolint:gocritic // CGO wrapper triggers false-positive dupSubExpr
	if pl == nil {
		C.utuimp_gst_state_free(st)
		return nil, fmt.Errorf("gst: playbin3/playbin element unavailable — is gstreamer1.0-plugins-base installed?")
	}
	p.pipeline = pl
	p.eq = eq
	p.wg.Add(1)
	go p.busLoop()
	return p, nil
}

// EnsureAACDecoder verifies that a GStreamer AAC decoder plugin (avdec_aac,
// faad, or fdkaacdec) is installed for Apple Music AAC streaming. HTTP source
// handling and ADTS frame parsing have embedded fallbacks (utuimphttpsrc and
// utuimpaacparse) so gstreamer1.0-plugins-good is not required.
func EnsureAACDecoder() error {
	initGst()
	if C.utuimp_gst_has_aac_decoder() == 0 {
		return fmt.Errorf("gst: no GStreamer AAC decoder plugin found — %s", aacDecoderHelp)
	}
	return nil
}

func setElementRank(name string, rank int) {
	initGst()
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	C.utuimp_gst_set_element_rank(cs, C.int(rank))
}

func hasElement(name string) bool {
	initGst()
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	return C.utuimp_gst_has_element(cs) != 0
}

// OnEOS registers a callback fired (in a goroutine) when the stream ends.
func (p *Player) OnEOS(fn func()) {
	p.mu.Lock()
	p.onEOS = fn
	p.mu.Unlock()
}

// OnError registers a callback fired (in a goroutine) on pipeline error.
func (p *Player) OnError(fn func(error)) {
	p.mu.Lock()
	p.onError = fn
	p.mu.Unlock()
}

// OnTags registers a callback for metadata carried by the current stream.
func (p *Player) OnTags(fn func(title, artist, album, station string)) {
	p.mu.Lock()
	p.onTags = fn
	p.mu.Unlock()
}

// OnStreamStart registers a callback fired (in a goroutine) with the URI of the
// gaplessly queued track when it begins playing.
func (p *Player) OnStreamStart(fn func(string)) {
	p.mu.Lock()
	p.onStreamStart = fn
	p.mu.Unlock()
}

// SetNextURI pre-stages the next URI to be handed to playbin on about-to-finish
// for gapless track transitions. Pass "" to clear any staged URI.
func (p *Player) SetNextURI(uri string) {
	var cs *C.char
	if uri != "" {
		cs = C.CString(uri)
		defer C.free(unsafe.Pointer(cs))
	}
	p.mu.Lock()
	if p.st != nil {
		C.utuimp_gst_state_set_next_uri(p.st, cs)
	}
	p.mu.Unlock()
}

// PlayURI loads the given URI (https:// or file://) and starts playback.
func (p *Player) PlayURI(uri string) {
	cs := C.CString(uri)
	defer C.free(unsafe.Pointer(cs))
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	if p.st != nil {
		C.utuimp_gst_state_set_next_uri(p.st, nil)
	}
	p.committedURI = ""
	C.utuimp_gst_set_uri(p.pipeline, p.st, cs)
	C.utuimp_gst_play(p.pipeline)
}

// Play resumes a paused pipeline.
func (p *Player) Play() {
	p.mu.Lock()
	if p.pipeline != nil {
		C.utuimp_gst_play(p.pipeline)
	}
	p.mu.Unlock()
}

// Pause pauses the pipeline.
func (p *Player) Pause() {
	p.mu.Lock()
	if p.pipeline != nil {
		C.utuimp_gst_pause(p.pipeline)
	}
	p.mu.Unlock()
}

// Stop resets the pipeline to NULL state, releasing network connections.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	if p.st != nil {
		C.utuimp_gst_state_set_next_uri(p.st, nil)
	}
	p.committedURI = ""
	C.utuimp_gst_stop(p.pipeline)
}

// Seek seeks to the given position within the current stream.
func (p *Player) Seek(pos time.Duration) {
	p.mu.Lock()
	if p.pipeline != nil {
		C.utuimp_gst_seek(p.pipeline, C.gint64(pos.Nanoseconds()))
	}
	p.mu.Unlock()
}

// SetVolume sets playback volume in [0.0, 1.0].
func (p *Player) SetVolume(v float64) {
	if math.IsNaN(v) {
		v = 0
	}
	v = min(max(v, 0), 1)
	p.mu.Lock()
	if p.pipeline != nil {
		C.utuimp_gst_set_volume(p.pipeline, C.gdouble(v))
	}
	p.mu.Unlock()
}

// SetEqualizer sets the 10-band equalizer gains in dB (clamped to [-24, +12]).
// Pass nil or an empty slice to reset all 10 bands to 0 dB.
func (p *Player) SetEqualizer(gains []float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	for i := range 10 {
		gain := 0.0
		if i < len(gains) && !math.IsNaN(gains[i]) {
			gain = gains[i]
		}
		C.utuimp_gst_set_eq_band(p.eq, p.st, C.gint(i), C.gdouble(gain))
	}
}

// Position returns the current playback position (0 if unknown).
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	if p.pipeline == nil {
		p.mu.Unlock()
		return 0
	}
	ns := int64(C.utuimp_gst_position(p.pipeline))
	p.mu.Unlock()
	if ns < 0 {
		return 0
	}
	return time.Duration(ns)
}

// Duration returns total stream duration (0 if unknown or live).
func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	if p.pipeline == nil {
		p.mu.Unlock()
		return 0
	}
	ns := int64(C.utuimp_gst_duration(p.pipeline))
	p.mu.Unlock()
	if ns < 0 {
		return 0
	}
	return time.Duration(ns)
}

// Destroy shuts down the bus poller and frees the GStreamer pipeline.
func (p *Player) Destroy() {
	p.destroyOnce.Do(func() {
		close(p.stop)
		p.wg.Wait()
		p.mu.Lock()
		if p.pipeline != nil {
			C.utuimp_gst_destroy(p.pipeline)
			p.pipeline = nil
		}
		p.eq = nil
		if p.st != nil {
			C.utuimp_gst_state_free(p.st)
			p.st = nil
		}
		p.mu.Unlock()
	})
}

func (p *Player) busLoop() {
	defer p.wg.Done()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-tick.C:
			p.drainBus()
		}
	}
}

func (p *Player) drainBus() {
	for {
		var strOut *C.gchar
		p.mu.Lock()
		t := C.utuimp_gst_pop_bus(p.pipeline, &strOut) //nolint:gocritic // false positive: CGO AST confuses dupSubExpr
		p.mu.Unlock()

		if t == 0 {
			return
		}
		if t < 0 {
			if strOut != nil {
				C.utuimp_gst_free(strOut)
			}
			continue
		}
		switch C.GstMessageType(t) {
		case C.GST_MESSAGE_APPLICATION:
			var uri string
			if strOut != nil {
				uri = C.GoString((*C.char)(unsafe.Pointer(strOut)))
				C.utuimp_gst_free(strOut)
			}
			if uri != "" {
				p.mu.Lock()
				p.committedURI = uri
				p.mu.Unlock()
			}
		case C.GST_MESSAGE_STREAM_START:
			p.mu.Lock()
			var cb func(string)
			uri := p.committedURI
			if uri != "" {
				p.committedURI = ""
				cb = p.onStreamStart
			}
			p.mu.Unlock()
			if cb != nil {
				go cb(uri)
			}
		case C.GST_MESSAGE_TAG:
			if strOut == nil {
				continue
			}
			value := C.GoString((*C.char)(unsafe.Pointer(strOut)))
			C.utuimp_gst_free(strOut)
			parts := strings.SplitN(value, "\x1f", 4)
			if len(parts) != 4 {
				continue
			}
			p.mu.Lock()
			cb := p.onTags
			p.mu.Unlock()
			if cb != nil {
				cb(parts[0], parts[1], parts[2], parts[3])
			}
		case C.GST_MESSAGE_EOS:
			p.mu.Lock()
			cb := p.onEOS
			p.mu.Unlock()
			if cb != nil {
				go cb()
			}
		case C.GST_MESSAGE_ERROR:
			var msg string
			if strOut != nil {
				msg = C.GoString((*C.char)(unsafe.Pointer(strOut)))
				C.utuimp_gst_free(strOut)
			}
			// gst-libav's AIFF demuxer only handles 16-bit, and playbin picks it
			// only when aiffparse (plugins-bad) is missing. The debug text names
			// the failing element; the pipeline URI may already be the next track.
			if strings.Contains(msg, "avdemux_aiff") && !hasElement("aiffparse") {
				msg = fmt.Sprintf("%s (%s)", msg, aiffParserHelp)
			} else if strings.Contains(msg, "No suitable plugins found") || strings.Contains(msg, "missing a plug-in") {
				help := aacDecoderHelp
				if C.utuimp_gst_has_aac_decoder() != 0 && missingGoodPlugins() {
					help = goodPluginsHelp
				}
				msg = fmt.Sprintf("%s (%s)", msg, help)
			}
			p.mu.Lock()
			cb := p.onError
			p.mu.Unlock()
			if cb != nil {
				go cb(fmt.Errorf("gst: %s", msg))
			}
		}
	}
}

// ── RawPlayer ────────────────────────────────────────────────────────────────

// RawPlayer plays a stream of raw PCM (16-bit little-endian, 44.1 kHz,
// stereo), read from src as fast as it is played: a full buffer makes the
// reads wait, and that is what paces whoever writes the stream. Spotify's
// librespot is played this way, so that its sound goes through the same
// pipeline, and the same visualizer tap, as every other service.
type RawPlayer struct {
	r      *C.UtuimpRaw
	flush  atomic.Bool
	closed atomic.Bool
	fed    chan struct{}
	once   sync.Once

	// hold drops the stream (at the speed it would play, so its writer
	// keeps going at its own pace) while a skip or a seek is on its way to
	// the source; skip is how much more to drop at once after it, and
	// resume plays the pipeline again with the first sound that follows.
	hold   atomic.Bool
	skip   atomic.Int64
	resume atomic.Bool
}

// NewRaw starts playing src. The error means GStreamer can't build the
// pipeline here; nothing has been read from src then.
func NewRaw(src io.Reader) (*RawPlayer, error) {
	initGst()
	r := C.utuimp_raw_new()
	if r == nil {
		return nil, errors.New("gst: cannot build the raw audio pipeline")
	}
	p := &RawPlayer{r: r, fed: make(chan struct{})}
	go p.feed(src)
	return p, nil
}

// rawChunk is 20 ms of sound.
const rawChunk = 3528

// feed reads src and hands it to the pipeline in pieces of 20 ms.
//
// The pipeline must never run dry while the source merely hesitates (a song
// loading after a skip, a pause given from a phone): the sound server's
// clock jumps when it does, and the sink then throws away seconds of what
// comes next as late. So a silent source is covered with silence. Only when
// it has been silent for seconds is the pipeline shut down, and the first
// sound after that starts it from nothing.
func (p *RawPlayer) feed(src io.Reader) {
	defer close(p.fed)
	chunks := make(chan []byte, 1)
	go func() {
		defer close(chunks)
		buf := make([]byte, rawChunk)
		have := 0
		for {
			n, err := src.Read(buf[have:])
			have += n
			if whole := have &^ 3; whole > 0 {
				chunks <- append([]byte(nil), buf[:whole]...)
				have = copy(buf, buf[whole:have])
			}
			if err != nil {
				return
			}
		}
	}()
	push := func(c []byte) bool {
		if p.closed.Load() {
			return false
		}
		if p.flush.Swap(false) {
			C.utuimp_raw_flush(p.r)
		}
		if p.resume.Swap(false) {
			C.utuimp_raw_play(p.r)
		}
		return C.utuimp_raw_push(p.r, unsafe.Pointer(&c[0]), C.int(len(c))) == 0
	}
	// sound plays c; false when it was dropped (closed, or held).
	sound := func(c []byte, resting bool) bool {
		if p.closed.Load() {
			return false
		}
		if p.hold.Load() {
			// Still the old place: drop it as fast as it would play.
			time.Sleep(time.Duration(len(c)/4) * time.Second / 44100)
			return false
		}
		if n := p.skip.Load(); n > 0 {
			d := min(n, int64(len(c)))
			p.skip.Add(-d)
			if c = c[d:]; len(c) < 4 {
				return false
			}
			c = c[:len(c)&^3]
		}
		if resting {
			p.flush.Store(false)
			C.utuimp_raw_restart(p.r)
		}
		push(c)
		return true
	}
	silence := make([]byte, rawChunk)
	const rest = 5 * time.Second
	resting := false
	wait := time.NewTimer(time.Hour)
	defer wait.Stop()
	for {
		wait.Reset(40 * time.Millisecond)
		select {
		case c, ok := <-chunks:
			if !ok {
				return
			}
			if sound(c, resting) {
				resting = false
			}
			continue
		case <-wait.C:
		}
		// The source is silent.
		for quiet := time.Duration(0); !resting && !p.closed.Load(); quiet += 20 * time.Millisecond {
			select {
			case c, ok := <-chunks:
				if !ok {
					return
				}
				sound(c, false)
				quiet = -time.Hour // back to sound: leave the loop
			default:
				switch {
				case p.hold.Load():
					time.Sleep(20 * time.Millisecond) // nothing is to be heard now
				case quiet >= rest:
					// Closed down: no stream is held open on the sound
					// server while nothing plays.
					resting = true
					C.utuimp_raw_stop(p.r)
				case !push(silence):
					time.Sleep(20 * time.Millisecond) // paused or flushing: don't spin
				}
			}
			if quiet < 0 {
				break
			}
		}
		if p.closed.Load() {
			return
		}
	}
}

// Play resumes after Pause.
// While held it does nothing: Release says whether to play.
func (p *RawPlayer) Play() {
	if !p.closed.Load() && !p.hold.Load() {
		C.utuimp_raw_play(p.r)
	}
}

// Pause stops the sound at once; what has been read stays in hand.
func (p *RawPlayer) Pause() {
	if !p.closed.Load() {
		p.resume.Store(false)
		C.utuimp_raw_pause(p.r)
	}
}

// Flush drops the sound read but not played yet: after a skip or a seek the
// next thing heard is the new place.
func (p *RawPlayer) Flush() { p.flush.Store(true) }

// Hold silences the player at once and throws away what it holds, and from
// then on drops the stream, until Release: a skip or a seek has been asked
// for, but the source goes on with the old place until it gets there.
// Playing that would be heard as the old song going on, or jumping ahead.
func (p *RawPlayer) Hold() {
	if p.closed.Load() {
		return
	}
	p.resume.Store(false)
	p.hold.Store(true)
	C.utuimp_raw_pause(p.r)
	// From here: a push waiting in the paused pipeline must give up now.
	C.utuimp_raw_flush(p.r)
	p.flush.Store(true)
}

// Release ends Hold: skip more bytes of the stream (what was written before
// the new place started) are dropped, and what follows is played, or kept
// paused when play is false.
func (p *RawPlayer) Release(play bool, skip int) {
	if p.closed.Load() || !p.hold.Load() {
		return
	}
	p.skip.Store(int64(max(skip, 0)))
	p.resume.Store(play)
	p.hold.Store(false)
}

// Holding reports whether a Hold is in force.
func (p *RawPlayer) Holding() bool { return p.hold.Load() }

// Close stops the player. src must be closed (or at its end) by then, so
// that the read it may be waiting in returns.
func (p *RawPlayer) Close() {
	p.once.Do(func() {
		p.closed.Store(true)
		C.utuimp_raw_stop(p.r)
		<-p.fed
		C.utuimp_raw_free(p.r)
	})
}
