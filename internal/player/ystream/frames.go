// Package ystream plays Yandex Music tracks: it resolves each track's short-
// lived download link, downloads the audio (MP3 or ADTS AAC) and feeds it to
// GStreamer through a loopback HTTP server, which also handles seeking by
// starting the stream at the right audio frame.
package ystream

import "time"

var mpaBitrates = [2][3][16]int{
	{
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
	},
	{
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
	},
}

var mpaRates = [3][3]int{{44100, 48000, 32000}, {22050, 24000, 16000}, {11025, 12000, 8000}}

var adtsRates = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

// mp3Frame parses an MPEG audio header at h; it returns the frame length and
// its duration, or 0 when h is not a valid header.
func mp3Frame(h []byte) (int, time.Duration) {
	if len(h) < 4 || h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
		return 0, 0
	}
	ver := int(h[1]>>3) & 3
	lay := int(h[1]>>1) & 3
	bri := int(h[2]>>4) & 0xF
	sri := int(h[2]>>2) & 3
	pad := int(h[2]>>1) & 1
	if ver == 1 || lay == 0 || bri == 0 || bri == 15 || sri == 3 {
		return 0, 0
	}
	v, vs := 1, 2
	switch ver {
	case 3:
		v, vs = 0, 0
	case 2:
		vs = 1
	}
	l := 4 - lay
	br := mpaBitrates[v][l-1][bri] * 1000
	sr := mpaRates[vs][sri]
	var n, spf int
	switch {
	case l == 1:
		n, spf = (12*br/sr+pad)*4, 384
	case l == 2 || v == 0:
		n, spf = 144*br/sr+pad, 1152
	default:
		n, spf = 72*br/sr+pad, 576
	}
	if n <= 4 {
		return 0, 0
	}
	return n, time.Duration(spf) * time.Second / time.Duration(sr)
}

// adtsFrame parses an ADTS header at h.
func adtsFrame(h []byte) (int, time.Duration) {
	if len(h) < 7 || h[0] != 0xFF || h[1]&0xF6 != 0xF0 {
		return 0, 0
	}
	sri := int(h[2]>>2) & 0x0F
	n := int(h[3]&0x03)<<11 | int(h[4])<<3 | int(h[5]>>5)
	if sri >= len(adtsRates) || n < 7 {
		return 0, 0
	}
	blocks := int(h[6]&0x03) + 1
	return n, time.Duration(blocks*1024) * time.Second / time.Duration(adtsRates[sri])
}

// id3Size returns the length of a leading ID3v2 tag, or 0.
func id3Size(b []byte) int {
	if len(b) < 10 || b[0] != 'I' || b[1] != 'D' || b[2] != '3' {
		return 0
	}
	n := int(b[6]&0x7F)<<21 | int(b[7]&0x7F)<<14 | int(b[8]&0x7F)<<7 | int(b[9]&0x7F)
	n += 10
	if b[5]&0x10 != 0 {
		n += 10
	}
	return n
}

// frameOffset walks audio frames from the start of data and returns the byte
// offset of the first frame at or after target, with that frame's start
// time. complete reports whether data holds the whole file; when it doesn't
// and the target lies beyond it, ok is false (more data is needed).
func frameOffset(data []byte, codec string, target time.Duration, complete bool) (off int, at time.Duration, ok bool) {
	parse := mp3Frame
	if codec == "aac" {
		parse = adtsFrame
	} else {
		off = id3Size(data)
	}
	for off < len(data) {
		if at >= target {
			return off, at, true
		}
		n, d := parse(data[off:])
		if n == 0 {
			off++ // resync
			continue
		}
		if off+n > len(data) {
			break
		}
		off += n
		at += d
	}
	if complete {
		return len(data), at, true
	}
	return 0, 0, false
}
