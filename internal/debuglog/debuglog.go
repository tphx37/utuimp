// Package debuglog records the diagnostic messages shown in the TUI.
package debuglog

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var mu sync.Mutex
var output io.Writer

// SetOutput enables or disables persistent diagnostic logging.
func SetOutput(w io.Writer) { mu.Lock(); defer mu.Unlock(); output = w }

// Print records a diagnostic event when logging is enabled.
func Print(line string) {
	mu.Lock()
	defer mu.Unlock()
	if output != nil {
		_, _ = fmt.Fprintf(output, "%s  %s\n", time.Now().Format(time.RFC3339), line)
	}
}
