//go:build linux && cgo

package local

import (
	"testing"
	"time"
)

// TestVolumeInState: the state carries the volume, so the volume line and
// its number show it instead of 0 while the song plays.
func TestVolumeInState(t *testing.T) {
	p, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	st, err := p.GetState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Volume != 1 {
		t.Errorf("volume at the start = %v, want 1 (GStreamer's own)", st.Volume)
	}
	sub := p.Subscribe()
	if err := p.SetVolume(0.4); err != nil {
		t.Fatal(err)
	}
	if st, _ = p.GetState(); st.Volume != 0.4 {
		t.Errorf("volume after SetVolume(0.4) = %v", st.Volume)
	}
	// The poll may send a state of its own first; the new volume follows.
	told := false
	for wait := time.After(2 * time.Second); !told; {
		select {
		case s := <-sub:
			told = s.Volume == 0.4
		case <-wait:
			t.Fatal("subscribers were not told the new volume")
		}
	}
	_ = p.SetVolume(3)
	if st, _ = p.GetState(); st.Volume != 1 {
		t.Errorf("SetVolume(3) left %v, want it kept at 1", st.Volume)
	}
}
