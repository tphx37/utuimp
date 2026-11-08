//go:build !linux && !darwin

package cmd

// captureStderr is a no-op where native libraries don't share the
// terminal's stderr in a way that corrupts the TUI.
func captureStderr() (restore func()) { return func() {} }
