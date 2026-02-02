package samurai

import (
	"runtime"
	"testing"
)

// runCleanups executes cleanup functions in LIFO order (last registered runs first),
// matching the behavior of Go's t.Cleanup and defer.
// Each cleanup runs in its own deferred recovery to ensure all cleanups execute
// even if one panics.
func runCleanups(cleanups []func(), t *testing.T) {
	t.Helper()
	for i := len(cleanups) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Helper()
					t.Errorf("cleanup panic: %v\n%s", r, captureCurrentStack())
				}
			}()
			cleanups[i]()
		}()
	}
}

// samuraiErr represents an internal samurai error (programming mistake).
// These are re-panicked and not caught by normal recovery.
type samuraiErr struct {
	message string
}

func (e *samuraiErr) Error() string {
	return "samurai: " + e.message
}

// maxStackTraceSize is the maximum buffer size for stack trace capture.
const maxStackTraceSize = 65536

// captureCurrentStack captures the current goroutine's stack trace for error
// reporting. When called from a deferred recovery handler, the trace includes
// the deferred call chain through the panic site, which helps diagnose the
// source of panics in user test code.
func captureCurrentStack() string {
	buf := make([]byte, 4096)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			return string(buf[:n])
		}
		if len(buf) >= maxStackTraceSize {
			return string(buf[:n]) + "\n... (stack trace truncated)"
		}
		buf = make([]byte, len(buf)*2)
	}
}
