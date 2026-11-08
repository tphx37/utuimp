package ystream

import (
	"testing"
	"time"
)

func mp3Header() []byte { return []byte{0xFF, 0xFB, 0x90, 0x64} } // MPEG1 L3 128k 44.1k

func TestMP3FrameAndOffset(t *testing.T) {
	n, d := mp3Frame(mp3Header())
	if n != 417 || d.Round(time.Millisecond) != 26*time.Millisecond {
		t.Fatalf("frame = %d bytes, %v", n, d)
	}
	frame := make([]byte, n)
	copy(frame, mp3Header())
	id3 := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 5, 1, 2, 3, 4, 5}
	data := append([]byte{}, id3...)
	for range 100 {
		data = append(data, frame...)
	}
	off, at, ok := frameOffset(data, "mp3", time.Second, true)
	if !ok || (off-len(id3))%n != 0 || at < time.Second || at > time.Second+30*time.Millisecond {
		t.Errorf("offset %d at %v ok %v", off, at, ok)
	}
	if _, _, ok := frameOffset(data[:2000], "mp3", time.Second, false); ok {
		t.Error("needs more data, must not report ok")
	}
}

func TestADTSFrameOffset(t *testing.T) {
	f := make([]byte, 20)
	f[0], f[1], f[2] = 0xFF, 0xF1, 4<<2 // 44.1 kHz
	f[3], f[4], f[5] = 0, byte(20>>3), byte(20&7)<<5
	var data []byte
	for range 100 {
		data = append(data, f...)
	}
	off, at, ok := frameOffset(data, "aac", 500*time.Millisecond, true)
	if !ok || off%20 != 0 || at < 500*time.Millisecond {
		t.Errorf("offset %d at %v", off, at)
	}
}
