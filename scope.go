package samurai

import (
	"context"
	"strings"
	"sync/atomic"
)

// TestScope is the generic scope builder for defining test structure with shared state.
// The type parameter V determines the test context type passed to all callbacks.
// Variables declared in the builder function are allocated fresh per path,
// making them parallel-safe without any special framework types.
//
// Methods:
//
//	s.Test(name, fn)          — leaf test (no children)
//	s.Test(name, fn, builder) — parent test with children
//	s.Skip()                  — skip all tests in this scope and descendants
//
// Multiple Test calls per scope are allowed — they become siblings in the test tree.
//
// Example with RunWith:
//
//	type MyCtx struct {
//	    *samurai.BaseContext
//	    *assert.Assertions
//	}
//	type S = samurai.TestScope[*MyCtx]
//
//	samurai.RunWith(t, func(w samurai.W) *MyCtx {
//	    return &MyCtx{BaseContext: w, Assertions: assert.New(w.Testing())}
//	}, func(s *S) {
//	    var db *DB
//
//	    s.Test("with database", func(ctx context.Context, w *MyCtx) {
//	        db = setupDB(ctx)
//	        w.Cleanup(func() { db.Close() })
//	    }, func(s *S) {
//	        s.Test("has tables", func(_ context.Context, c *MyCtx) {
//	            c.NotEmpty(db.Tables())
//	        })
//	    })
//	})
type TestScope[V Context] struct {
	mode     scopeMode
	children []*scopedChild[V]
	sealed   atomic.Bool // true after builder returns; prevents late mutation
	skipped  bool        // true when Skip() has been called
}

// Scope is the default non-generic scope used by Run.
// It is a type alias for TestScope[W], where W = *BaseContext.
type Scope = TestScope[W]

type scopeMode int

const (
	modeDiscovery scopeMode = iota
	modeExecution
)

type scopedChild[V Context] struct {
	name    string
	fn      func(context.Context, V)
	builder func(*TestScope[V]) // nil for leaf tests
}

// Test registers a named test in this scope.
// The name appears in the test tree (go test -v output and IDE).
//
// Without a builder argument, Test creates a leaf test:
//
//	s.Test("check value", func(_ context.Context, w W) {
//	    assert.Equal(w.Testing(), expected, actual)
//	})
//
// With a builder argument, Test creates a parent with children:
//
//	s.Test("setup db", func(ctx context.Context, w W) {
//	    db = setupDB(ctx)
//	    w.Cleanup(func() { db.Close() })
//	}, func(s *Scope) {
//	    s.Test("has tables", func(_ context.Context, w W) { ... })
//	})
//
// Multiple Test calls per scope are allowed — they become siblings.
// In discovery mode: records the test without executing fn.
// In execution mode: records the test; fn is executed later by the execution engine.
func (s *TestScope[V]) Test(name string, fn func(context.Context, V), builders ...func(*TestScope[V])) {
	if s.sealed.Load() {
		panic(&samuraiErr{message: "Test called on a sealed scope (builder has already returned)"})
	}
	if name == "" {
		panic(&samuraiErr{message: "Test called with empty name"})
	}
	if strings.Contains(name, "/") {
		panic(&samuraiErr{message: "Test name must not contain '/' (reserved as path separator in go test -run)"})
	}
	if fn == nil {
		panic(&samuraiErr{message: "Test called with nil function"})
	}
	if len(builders) > 1 {
		panic(&samuraiErr{message: "Test accepts at most one builder function"})
	}

	var builder func(*TestScope[V])
	if len(builders) == 1 {
		builder = builders[0]
		if builder == nil {
			panic(&samuraiErr{message: "Test called with nil builder"})
		}
	}

	s.children = append(s.children, &scopedChild[V]{
		name:    name,
		fn:      fn,
		builder: builder,
	})
}

// Skip marks all tests in this scope as skipped.
// Skipped tests appear in output as SKIP but their callbacks never execute.
// The call order relative to Test does not matter — Skip affects the entire scope.
//
// Skip propagates to all nested scopes: if a parent scope is skipped,
// all descendants are skipped regardless of whether they call Skip themselves.
//
//	s.Test("WIP feature", fn, func(s *Scope) {
//	    s.Skip()
//	    s.Test("todo", fn) // skipped
//	})
func (s *TestScope[V]) Skip() {
	if s.sealed.Load() {
		panic(&samuraiErr{message: "Skip called on a sealed scope (builder has already returned)"})
	}
	s.skipped = true
}
