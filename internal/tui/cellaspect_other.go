//go:build !linux && !darwin

package tui

// cellAspect returns the conventional cell height/width ratio on platforms
// without a TIOCGWINSZ query. See the unix build for details.
func cellAspect() float64 { return 2.0 }

// cellPixels returns the size of a terminal cell in pixels, or zeros when
// the terminal doesn't say.
func cellPixels() (w, h int) { return 0, 0 }
