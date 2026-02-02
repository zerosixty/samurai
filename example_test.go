package samurai

import (
	"context"
	"fmt"
	"strings"
)

// ExampleRun demonstrates the test tree structure built by Run.
// Run is called inside a Go test function: func TestXxx(t *testing.T).
// This example shows path discovery — the first phase of the two-phase model.
// In real tests, Run handles both discovery and execution automatically.
func ExampleRun() {
	paths, _ := collectScopedPaths(func(s *Scope) {
		s.Test("with database", func(_ context.Context, _ W) {
			// setup: create DB, register cleanup
		}, func(s *Scope) {
			s.Test("can query", func(_ context.Context, _ W) {})
			s.Test("can insert", func(_ context.Context, _ W) {})
		})
	})
	for _, p := range paths {
		fmt.Println(strings.Join(p.segments, "/"))
	}
	// Output:
	// with database/can query
	// with database/can insert
}

// ExampleRunWith demonstrates the generic RunWith variant with a custom context.
// RunWith lets callbacks receive a custom type (e.g. an assertion helper)
// instead of the default *BaseContext.
func ExampleRunWith() {
	type myCtx struct{ *BaseContext }

	paths, _ := collectScopedPaths(func(s *TestScope[*myCtx]) {
		s.Test("setup", func(_ context.Context, _ *myCtx) {
			// factory creates *myCtx from *BaseContext per scope level
		}, func(s *TestScope[*myCtx]) {
			s.Test("check A", func(_ context.Context, _ *myCtx) {})
			s.Test("check B", func(_ context.Context, _ *myCtx) {})
		})
	})
	for _, p := range paths {
		fmt.Println(strings.Join(p.segments, "/"))
	}
	// Output:
	// setup/check A
	// setup/check B
}

// ExampleRun_nested demonstrates deeply nested test paths with multiple branches.
// Each leaf path executes independently with fresh parent setup.
func ExampleRun_nested() {
	paths, _ := collectScopedPaths(func(s *Scope) {
		s.Test("database", func(_ context.Context, _ W) {}, func(s *Scope) {
			s.Test("users", func(_ context.Context, _ W) {}, func(s *Scope) {
				s.Test("create", func(_ context.Context, _ W) {})
				s.Test("delete", func(_ context.Context, _ W) {})
			})
			s.Test("posts", func(_ context.Context, _ W) {}, func(s *Scope) {
				s.Test("list", func(_ context.Context, _ W) {})
			})
		})
	})
	for _, p := range paths {
		fmt.Println(strings.Join(p.segments, "/"))
	}
	// Output:
	// database/users/create
	// database/users/delete
	// database/posts/list
}
