//go:build !unix

package styles

// QueryPalette does nothing here: the palette stays unknown.
func QueryPalette() {}
