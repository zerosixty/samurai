package samurai

import (
	"context"
	"testing"
)

// BaseContext is the framework-provided test context.
// It implements the Context interface, providing Testing() and Cleanup().
// It also provides Context() to access the current scope's context.Context.
//
// Users can embed *BaseContext in their own struct to create custom test contexts
// for use with RunWith. The embedded methods (Testing, Cleanup, Context) are promoted,
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

// Context returns the context.Context for the current test scope.
// The returned context is canceled when the test completes.
// This is equivalent to calling b.Testing().Context().
//
// Context is available in RunWith factories, enabling initialization
// that requires a context:
//
//	samurai.RunWith(t, func(w samurai.W) *MyCtx {
//	    ctx := w.Context()
//	    return &MyCtx{BaseContext: w, db: connectDB(ctx)}
//	}, builder)
func (b *BaseContext) Context() context.Context {
	return b.tt.Context()
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
