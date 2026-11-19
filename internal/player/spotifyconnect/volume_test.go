package spotifyconnect

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestLocalVolumeScalesPCM(t *testing.T) {
	data := make([]byte, 4)
	binary.LittleEndian.PutUint16(data, uint16(20000))
	binary.LittleEndian.PutUint16(data[2:], uint16(65536-10000))
	r := &volumeReader{source: bytes.NewReader(data)}
	r.SetVolume(0.5)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if int16(binary.LittleEndian.Uint16(got)) != 10000 || int16(binary.LittleEndian.Uint16(got[2:])) != -5000 { //nolint:gosec // interpret signed PCM samples
		t.Fatalf("scaled samples: %v", got)
	}
}
