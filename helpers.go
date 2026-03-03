package samurai

import (
	"runtime"
	"sync"
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

// cleanupStack is a thread-safe stack of cleanup slices used by executeScope.
// Each scope level pushes a new slice. Cleanup() always appends to the top.
// Pop returns the top slice for execution, ensuring inner cleanups run before outer.
type cleanupStack struct {
	mu     sync.Mutex
	levels [][]func()
}

func (cs *cleanupStack) push() {
	cs.mu.Lock()
	cs.levels = append(cs.levels, nil)
	cs.mu.Unlock()
}

func (cs *cleanupStack) add(fn func()) {
	cs.mu.Lock()
	cs.levels[len(cs.levels)-1] = append(cs.levels[len(cs.levels)-1], fn)
	cs.mu.Unlock()
}

func (cs *cleanupStack) pop() []func() {
	cs.mu.Lock()
	top := len(cs.levels) - 1
	fns := cs.levels[top]
	cs.levels = cs.levels[:top]
	cs.mu.Unlock()
	return fns
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
