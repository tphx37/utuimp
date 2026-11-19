package spotifyconnect

import (
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"
)

// volumeReader scales the signed 16-bit little-endian PCM sent by librespot.
type volumeReader struct {
	source io.Reader
	level  atomic.Uint64
}

func (v *volumeReader) SetVolume(level float64) { v.level.Store(math.Float64bits(level)) }
func (v *volumeReader) Read(b []byte) (int, error) {
	if len(b) < 2 {
		return 0, io.ErrShortBuffer
	}
	n, err := io.ReadFull(v.source, b[:len(b)/2*2])
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	level := math.Float64frombits(v.level.Load())
	for i := 0; i+1 < n; i += 2 {
		sample := int16(binary.LittleEndian.Uint16(b[i:]))                         //nolint:gosec // interpret the PCM bits as a signed sample
		binary.LittleEndian.PutUint16(b[i:], uint16(int16(float64(sample)*level))) //nolint:gosec // preserve the signed sample's two's-complement bits
	}
	return n, err
}

type volumeOutput struct {
	audioOut
	volume *volumeReader
}

func (o *volumeOutput) SetVolume(v float64) { o.volume.SetVolume(v) }
