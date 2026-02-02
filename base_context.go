package samurai

import (
	"testing"
)

// BaseContext is the framework-provided test context.
// It implements the Context interface, providing Testing() and Cleanup().
//
// Users can embed *BaseContext in their own struct to create custom test contexts
// for use with RunWith. The embedded methods (Testing, Cleanup) are promoted,
// so the custom type satisfies Context automatically.
//
// Example:
//
//	type MyCtx struct {
//	    *samurai.BaseContext
//	    *assert.Assertions
//	}
type BaseContext struct {
	tt         *testing.T
	addCleanup func(func())
}

// Testing returns the underlying *testing.T for use with any assertion library.
// Always returns a valid *testing.T (never nil).
func (b *BaseContext) Testing() *testing.T {
	return b.tt
}

// Cleanup registers a cleanup function that runs after the current scope completes.
// Multiple cleanups execute in LIFO order (last registered runs first),
// matching the behavior of Go's t.Cleanup and defer.
// Cleanups run even if the step panics.
func (b *BaseContext) Cleanup(fn func()) {
	if fn == nil {
		panic(&samuraiErr{message: "Cleanup called with nil function"})
	}
	b.addCleanup(fn)
}
