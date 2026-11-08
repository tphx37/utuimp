//go:build !linux

package cmd

import "github.com/tphx37/utuimp/internal/player/mux"

// setupMuxPlatform: platform media integration (SMTC on Windows) is set up
// per player by setupBrowserlessPlatform.
func setupMuxPlatform(*mux.Mux) func() { return func() {} }
