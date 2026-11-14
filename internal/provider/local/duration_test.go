package local

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWavDuration(t *testing.T) {
	data := make([]byte, 44100*4*2) // 2 s of 16-bit stereo at 44.1 kHz
	b := []byte("RIFF\x00\x00\x00\x00WAVEfmt ")
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = binary.LittleEndian.AppendUint32(b, 44100)
	b = binary.LittleEndian.AppendUint32(b, 44100*4)
	b = binary.LittleEndian.AppendUint16(b, 4)
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data))) //nolint:gosec // a short test file
	b = append(b, data...)
	p := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if d := fileDuration(p); d != 2*time.Second {
		t.Fatalf("got %v, want 2s", d)
	}
}

func TestFlacDuration(t *testing.T) {
	si := make([]byte, 34)
	rate, samples := int64(48000), int64(48000*3)
	// The sample rate's 20 bits, across three bytes (the conversions keep
	// the low byte of each piece on purpose).
	si[10] = byte(rate >> 12)     //nolint:gosec // see above
	si[11] = byte(rate >> 4)      //nolint:gosec // see above
	si[12] = byte(rate<<4) | 0x02 //nolint:gosec // see above
	binary.BigEndian.PutUint32(si[14:18], uint32(samples))
	b := append([]byte("fLaC\x80\x00\x00\x22"), si...)
	p := filepath.Join(t.TempDir(), "a.flac")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if d := fileDuration(p); d != 3*time.Second {
		t.Fatalf("got %v, want 3s", d)
	}
}

func TestMP3CBRDuration(t *testing.T) {
	// MPEG-1 Layer III, 128 kbps, 44.1 kHz: 16000 bytes/s.
	frame := make([]byte, 417)
	copy(frame, []byte{0xFF, 0xFB, 0x90, 0x00})
	var b []byte
	for len(b) < 16000*4 {
		b = append(b, frame...)
	}
	b = b[:16000*4]
	p := filepath.Join(t.TempDir(), "a.mp3")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if d := fileDuration(p); d != 4*time.Second {
		t.Fatalf("got %v, want 4s", d)
	}
}
