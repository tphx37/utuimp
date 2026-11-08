package local

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"
	"time"
)

// fileDuration reads an audio file's length from its headers (the tag
// library doesn't provide it). Zero when it can't tell.
func fileDuration(path string) time.Duration {
	f, err := os.Open(path) //nolint:gosec // user's music dir
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	switch strings.ToLower(path[strings.LastIndex(path, ".")+1:]) {
	case "mp3":
		return mp3Duration(f, st.Size())
	case "flac":
		return flacDuration(f)
	case "m4a", "mp4", "aac", "alac":
		return mp4Duration(f, st.Size())
	case "wav", "wave":
		return wavDuration(f)
	}
	return 0
}

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

// mp3Duration uses the Xing/Info or VBRI frame count when present (VBR
// files), else the bitrate of the first frame and the file size (CBR).
func mp3Duration(f io.ReaderAt, size int64) time.Duration {
	head := make([]byte, 10)
	if _, err := f.ReadAt(head, 0); err != nil {
		return 0
	}
	start := int64(0)
	if bytes.HasPrefix(head, []byte("ID3")) {
		start = int64(head[6]&0x7F)<<21 | int64(head[7]&0x7F)<<14 | int64(head[8]&0x7F)<<7 | int64(head[9]&0x7F) + 10
		if head[5]&0x10 != 0 {
			start += 10
		}
	}
	buf := make([]byte, 64<<10)
	n, _ := f.ReadAt(buf, start)
	buf = buf[:n]
	for i := 0; i+4 <= len(buf); i++ {
		h := buf[i:]
		if h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
			continue
		}
		ver := int(h[1]>>3) & 3
		lay := int(h[1]>>1) & 3
		bri := int(h[2]>>4) & 0xF
		sri := int(h[2]>>2) & 3
		if ver == 1 || lay == 0 || bri == 0 || bri == 15 || sri == 3 {
			continue
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
		spf := 1152
		switch {
		case l == 1:
			spf = 384
		case l == 3 && v == 1:
			spf = 576
		}
		// Xing/Info header: after the side info.
		side := 32
		if v == 1 {
			side = 17
		}
		if h[3]>>6 == 3 { // mono
			side = map[bool]int{true: 17, false: 9}[v == 0]
		}
		if x := i + 4 + side; x+12 <= len(buf) {
			tag := string(buf[x : x+4])
			if (tag == "Xing" || tag == "Info") && buf[x+7]&1 != 0 {
				frames := binary.BigEndian.Uint32(buf[x+8 : x+12])
				return time.Duration(int64(frames)*int64(spf)) * time.Second / time.Duration(sr)
			}
		}
		if x := i + 36; x+18 <= len(buf) && string(buf[x:x+4]) == "VBRI" {
			frames := binary.BigEndian.Uint32(buf[x+14 : x+18])
			return time.Duration(int64(frames)*int64(spf)) * time.Second / time.Duration(sr)
		}
		audio := size - start - int64(i)
		if tail := make([]byte, 3); size > 128 {
			if _, err := f.ReadAt(tail, size-128); err == nil && string(tail) == "TAG" {
				audio -= 128
			}
		}
		if br <= 0 || audio <= 0 {
			return 0
		}
		return time.Duration(audio * 8 * int64(time.Second) / int64(br))
	}
	return 0
}

// flacDuration reads total samples and sample rate from STREAMINFO.
func flacDuration(f io.ReaderAt) time.Duration {
	b := make([]byte, 4+4+34)
	if _, err := f.ReadAt(b, 0); err != nil || string(b[:4]) != "fLaC" || b[4]&0x7F != 0 {
		return 0
	}
	si := b[8:]
	rate := int64(si[10])<<12 | int64(si[11])<<4 | int64(si[12])>>4
	samples := int64(si[13]&0x0F)<<32 | int64(binary.BigEndian.Uint32(si[14:18]))
	if rate == 0 {
		return 0
	}
	return time.Duration(samples * int64(time.Second) / rate)
}

// mp4Duration finds moov/mvhd and reads its timescale and duration.
func mp4Duration(f io.ReaderAt, size int64) time.Duration {
	var walk func(start, end int64, depth int) time.Duration
	walk = func(start, end int64, depth int) time.Duration {
		hdr := make([]byte, 16)
		for pos := start; pos+8 <= end; {
			if _, err := f.ReadAt(hdr[:8], pos); err != nil {
				return 0
			}
			n := int64(binary.BigEndian.Uint32(hdr[:4]))
			typ := string(hdr[4:8])
			hl := int64(8)
			if n == 1 {
				if _, err := f.ReadAt(hdr[8:16], pos+8); err != nil {
					return 0
				}
				n, hl = int64(binary.BigEndian.Uint64(hdr[8:16])), 16
			} else if n == 0 {
				n = end - pos
			}
			if n < hl {
				return 0
			}
			switch typ {
			case "moov":
				if depth < 2 {
					return walk(pos+hl, pos+n, depth+1)
				}
			case "mvhd":
				b := make([]byte, 32)
				if _, err := f.ReadAt(b, pos+hl); err != nil {
					return 0
				}
				var scale, dur int64
				if b[0] == 1 {
					scale = int64(binary.BigEndian.Uint32(b[20:24]))
					dur = int64(binary.BigEndian.Uint64(b[24:32]))
				} else {
					scale = int64(binary.BigEndian.Uint32(b[12:16]))
					dur = int64(binary.BigEndian.Uint32(b[16:20]))
				}
				if scale == 0 {
					return 0
				}
				return time.Duration(dur * int64(time.Second) / scale)
			}
			pos += n
		}
		return 0
	}
	return walk(0, size, 0)
}

// wavDuration divides the data chunk size by the byte rate.
func wavDuration(f io.ReaderAt) time.Duration {
	b := make([]byte, 12)
	if _, err := f.ReadAt(b, 0); err != nil || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0
	}
	var byteRate int64
	hdr := make([]byte, 8)
	for pos := int64(12); ; {
		if _, err := f.ReadAt(hdr, pos); err != nil {
			return 0
		}
		n := int64(binary.LittleEndian.Uint32(hdr[4:8]))
		switch string(hdr[:4]) {
		case "fmt ":
			fmtb := make([]byte, 12)
			if _, err := f.ReadAt(fmtb, pos+8); err != nil {
				return 0
			}
			byteRate = int64(binary.LittleEndian.Uint32(fmtb[8:12]))
		case "data":
			if byteRate == 0 {
				return 0
			}
			return time.Duration(n * int64(time.Second) / byteRate)
		}
		pos += 8 + n + n%2
	}
}
