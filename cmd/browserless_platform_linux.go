//go:build linux

package cmd

import (
	"fmt"

	"github.com/tphx37/utuimp/internal/player/browserless"
	"github.com/tphx37/utuimp/internal/player/mpris"
	"github.com/tphx37/utuimp/internal/player/mux"
)

func browserlessBackendLabel(audioBitrateKbps int) string {
	return fmt.Sprintf("Browserless (In-Process Widevine + GStreamer) · %d kbps AAC", audioBitrateKbps)
}

// setupBrowserlessPlatform has nothing to do on Linux: MPRIS is served for
// all services at once by setupMuxPlatform.
func setupBrowserlessPlatform(*browserless.Player, func(string)) {}

// setupMuxPlatform exports the active service over MPRIS (media keys,
// desktop widgets).
func setupMuxPlatform(mx *mux.Mux) func() {
	srv, err := mpris.NewServer(mx)
	if err != nil {
		return func() {}
	}
	go func() {
		for st := range mx.Subscribe() {
			srv.Update(st)
		}
	}()
	return func() { _ = srv.Close() }
}
