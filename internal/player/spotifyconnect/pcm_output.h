#ifndef UTUIMP_PCM_OUTPUT_H
#define UTUIMP_PCM_OUTPUT_H
#include "../tap/pcm.h"
#include <stdlib.h>
#include <string.h>
#ifdef __APPLE__
#include <AudioToolbox/AudioToolbox.h>
#include <sys/ioctl.h>
#else
#include <mmsystem.h>
#endif

#define UTUIMP_OUT_BUFFERS 6
#define UTUIMP_OUT_BYTES 8192
// All entry points are serialized by the Go owner. Only AudioQueue's return
// callback runs concurrently; it touches the free list under its own mutex.
typedef struct {
    utuimp_pcm_tap tap;
    int paused;
#ifdef __APPLE__
    AudioQueueRef queue;
    AudioQueueBufferRef buffers[UTUIMP_OUT_BUFFERS];
    int free_buffer[UTUIMP_OUT_BUFFERS];
    pthread_mutex_t mu;
#else
    HWAVEOUT wave;
    WAVEHDR headers[UTUIMP_OUT_BUFFERS];
    char buffers[UTUIMP_OUT_BUFFERS][UTUIMP_OUT_BYTES];
    int prepared[UTUIMP_OUT_BUFFERS];
#endif
} utuimp_pcmout;

#ifdef __APPLE__
static void utuimp_pcmout_return(void *ctx, AudioQueueRef queue, AudioQueueBufferRef buffer) {
    (void)queue;
    utuimp_pcmout *out = (utuimp_pcmout *)ctx;
    pthread_mutex_lock(&out->mu);
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) if (out->buffers[i] == buffer) out->free_buffer[i] = 1;
    pthread_mutex_unlock(&out->mu);
}
#endif

static void utuimp_pcmout_destroy(utuimp_pcmout *out) {
    if (!out) return;
#ifdef __APPLE__
    if (out->queue) AudioQueueDispose(out->queue, true);
    pthread_mutex_destroy(&out->mu);
#else
    if (out->wave) {
        waveOutReset(out->wave);
        for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) if (out->prepared[i]) waveOutUnprepareHeader(out->wave, &out->headers[i], sizeof(WAVEHDR));
        waveOutClose(out->wave);
    }
#endif
    utuimp_pcm_destroy(&out->tap);
    free(out);
}

static utuimp_pcmout *utuimp_pcmout_create(void) {
    utuimp_pcmout *out = (utuimp_pcmout *)calloc(1, sizeof(*out));
    if (!out) return NULL;
    utuimp_pcm_init(&out->tap);
#ifdef __APPLE__
    pthread_mutex_init(&out->mu, NULL);
    AudioStreamBasicDescription format = {0};
    format.mSampleRate = 44100;
    format.mFormatID = kAudioFormatLinearPCM;
    format.mFormatFlags = kAudioFormatFlagIsSignedInteger | kAudioFormatFlagIsPacked;
    format.mChannelsPerFrame = 2; format.mBitsPerChannel = 16;
    format.mBytesPerPacket = format.mBytesPerFrame = 4; format.mFramesPerPacket = 1;
    if (AudioQueueNewOutput(&format, utuimp_pcmout_return, out, NULL, NULL, 0, &out->queue) != noErr) {
        utuimp_pcmout_destroy(out); return NULL;
    }
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) {
        if (AudioQueueAllocateBuffer(out->queue, UTUIMP_OUT_BYTES, &out->buffers[i]) != noErr) {
            utuimp_pcmout_destroy(out); return NULL;
        }
        out->free_buffer[i] = 1;
    }
#else
    WAVEFORMATEX format = {0};
    format.wFormatTag = WAVE_FORMAT_PCM;
    format.nChannels = 2; format.nSamplesPerSec = 44100; format.wBitsPerSample = 16;
    format.nBlockAlign = 4; format.nAvgBytesPerSec = 44100 * 4;
    if (waveOutOpen(&out->wave, WAVE_MAPPER, &format, 0, 0, CALLBACK_NULL) != MMSYSERR_NOERROR) {
        utuimp_pcmout_destroy(out); return NULL;
    }
#endif
    return out;
}

// Nonblocking: 0 means the six audio buffers are full, -1 an output error.
static int utuimp_pcmout_write(utuimp_pcmout *out, const char *data, int bytes) {
    if (bytes <= 0 || bytes > UTUIMP_OUT_BYTES || bytes % 4 != 0) return -1;
    int slot = -1;
#ifdef __APPLE__
    pthread_mutex_lock(&out->mu);
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) if (out->free_buffer[i]) { slot = i; out->free_buffer[i] = 0; break; }
    pthread_mutex_unlock(&out->mu);
    if (slot < 0) return 0;
    AudioQueueBufferRef buffer = out->buffers[slot];
    memcpy(buffer->mAudioData, data, (size_t)bytes);
    buffer->mAudioDataByteSize = (UInt32)bytes;
    if (AudioQueueEnqueueBuffer(out->queue, buffer, 0, NULL) != noErr) return -1;
    if (!out->paused) AudioQueueStart(out->queue, NULL);
#else
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) {
        if (!out->prepared[i] || (out->headers[i].dwFlags & WHDR_DONE)) { slot = i; break; }
    }
    if (slot < 0) return 0;
    WAVEHDR *header = &out->headers[slot];
    if (out->prepared[slot]) { waveOutUnprepareHeader(out->wave, header, sizeof(*header)); out->prepared[slot] = 0; }
    memcpy(out->buffers[slot], data, (size_t)bytes);
    memset(header, 0, sizeof(*header));
    header->lpData = out->buffers[slot]; header->dwBufferLength = (DWORD)bytes;
    if (waveOutPrepareHeader(out->wave, header, sizeof(*header)) != MMSYSERR_NOERROR) return -1;
    out->prepared[slot] = 1;
    if (waveOutWrite(out->wave, header, sizeof(*header)) != MMSYSERR_NOERROR) return -1;
#endif
    utuimp_pcm_write(&out->tap, data, (size_t)bytes / 4, 2, 44100, 0);
    return 1;
}
static void utuimp_pcmout_pause(utuimp_pcmout *out, int paused) {
    out->paused = paused;
#ifdef __APPLE__
    if (paused) AudioQueuePause(out->queue); else AudioQueueStart(out->queue, NULL);
#else
    if (paused) waveOutPause(out->wave); else waveOutRestart(out->wave);
#endif
}
static double utuimp_pcmout_position(utuimp_pcmout *out) {
#ifdef __APPLE__
    AudioTimeStamp ts = {0};
    if (AudioQueueGetCurrentTime(out->queue, NULL, &ts, NULL) != noErr || !(ts.mFlags & kAudioTimeStampSampleTimeValid)) return -1;
    return ts.mSampleTime;
#else
    return utuimp_pcm_wave_position(out->wave, 44100, 44100 * 4);
#endif
}
static void utuimp_pcmout_flush(utuimp_pcmout *out) {
#ifdef __APPLE__
    AudioQueueStop(out->queue, true);
    pthread_mutex_lock(&out->mu);
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) out->free_buffer[i] = 1;
    pthread_mutex_unlock(&out->mu);
#else
    waveOutReset(out->wave);
    for (int i = 0; i < UTUIMP_OUT_BUFFERS; ++i) {
        if (out->prepared[i]) waveOutUnprepareHeader(out->wave, &out->headers[i], sizeof(WAVEHDR));
        out->prepared[i] = 0;
    }
    // waveOutReset resumes a paused device on some drivers.
    if (out->paused) waveOutPause(out->wave);
#endif
    double origin = utuimp_pcmout_position(out);
    utuimp_pcm_reset(&out->tap, origin >= 0 ? origin : 0);
}
static int utuimp_pcmout_read(utuimp_pcmout *out, float *dst, int n, double *ahead, int64_t *end) {
    if (out->paused) return 0;
    double played = utuimp_pcmout_position(out);
    if (played < 0) return 0;
    return utuimp_pcm_read(&out->tap, played, dst, n, ahead, end);
}
static int utuimp_pcmout_pending(uintptr_t fd) {
#ifdef __APPLE__
    int n = 0;
    return ioctl((int)fd, FIONREAD, &n) == 0 ? n : 0;
#else
    DWORD n = 0;
    return PeekNamedPipe((HANDLE)fd, NULL, 0, NULL, &n, NULL) ? (int)n : 0;
#endif
}
#endif
