package samurai

import (
	"fmt"
	"sync"
	"testing"
)

// executeScope runs the builder in execution mode for one leaf path.
// The builder runs fresh, allocating new local variables for this path.
// Each scope level creates its own BaseContext with a cleanup slice, so cleanup
// ownership is explicit and doesn't require save/restore on a shared struct.
func executeScope[V Context](t *testing.T, builder func(*TestScope[V]), factory func(W) V, path []string) {
	var mu sync.Mutex
	var cleanups []func()
	base := &BaseContext{
		tt: t,
		addCleanup: func(fn func()) {
			mu.Lock()
			cleanups = append(cleanups, fn)
			mu.Unlock()
		},
	}
	v := factory(base)

	// Single defer handles both panic recovery and cleanup execution.
	// Order: recover panic first (so cleanups still run), then run cleanups.
	defer func() {
		if r := recover(); r != nil {
			// Re-panic internal samurai errors (programming mistakes)
			if err, ok := r.(*samuraiErr); ok {
				panic(err)
			}
			// Report user panics
			t.Helper()
			t.Errorf("panic: %v\n%s", r, captureCurrentStack())
		}

		// Snapshot cleanups under the lock to prevent races with user goroutines
		// that may still be calling Cleanup() after the callback returned.
		mu.Lock()
		snapshot := make([]func(), len(cleanups))
		copy(snapshot, cleanups)
		mu.Unlock()
		runCleanups(snapshot, t)
	}()

	// Run builder in EXECUTION mode.
	// This allocates fresh local variables. Test() only records children.
	s := &TestScope[V]{mode: modeExecution}
	builder(s)
	s.sealed.Store(true)

	if len(path) == 0 {
		if len(s.children) > 0 {
			panic(&samuraiErr{message: "reached leaf execution but scope has children (internal error)"})
		}
		return
	}

	// Find the child matching the next path element
	targetName := path[0]
	for _, child := range s.children {
		if child.name == targetName {
			// Execute this child's fn with the current scope's context
			child.fn(t.Context(), v)

			if len(path) == 1 {
				// Last path element — this is a leaf, we're done
				return
			}

			// More path elements — recurse into child's builder
			if child.builder == nil {
				panic(&samuraiErr{message: fmt.Sprintf("path continues after leaf %q (internal error)", targetName)})
			}
			executeScope[V](t, child.builder, factory, path[1:])
			return
		}
	}

	panic(&samuraiErr{message: fmt.Sprintf("path element %q not found in scope (builder produced different structure between discovery and execution)", targetName)})
}

// discoveredPath represents a leaf path found during discovery.
// The skipped flag is true if any ancestor scope called Skip().
type discoveredPath struct {
	segments []string
	skipped  bool
}

// collectScopedPaths runs builder in discovery mode to get all leaf paths.
// Returns the discovered paths and a validation error (nil if valid).
// Test() callbacks don't execute in discovery mode!
func collectScopedPaths[V Context](builder func(*TestScope[V])) (paths []discoveredPath, err error) {
	s := &TestScope[V]{mode: modeDiscovery}

	// Recover panics from inline builder code (e.g., nil pointer in variable init).
	// Discovery mode should not execute user code, but if it does, report clearly.
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*samuraiErr); ok {
				err = e
			} else {
				err = fmt.Errorf("samurai: panic during discovery: %v", r)
			}
		}
	}()

	builder(s)
	s.sealed.Store(true)

	if len(s.children) == 0 {
		return nil, fmt.Errorf("samurai: no tests defined")
	}

	return collectPathsFromChildren[V](nil, s.children, s.skipped)
}

// collectPathsFromChildren recursively collects all leaf paths from a list of children.
// parentSkipped is true if any ancestor scope called Skip().
func collectPathsFromChildren[V Context](prefix []string, children []*scopedChild[V], parentSkipped bool) ([]discoveredPath, error) {
	var paths []discoveredPath
	seen := make(map[string]bool)

	for _, child := range children {
		if seen[child.name] {
			return nil, fmt.Errorf("samurai: duplicate test name %q in scope", child.name)
		}
		seen[child.name] = true

		// Build the path for this child
		childPath := make([]string, len(prefix)+1)
		copy(childPath, prefix)
		childPath[len(prefix)] = child.name

		if child.builder == nil {
			// Leaf test — complete path
			paths = append(paths, discoveredPath{segments: childPath, skipped: parentSkipped})
		} else {
			// Parent test — probe builder to discover children
			inner, err := probeBuilder[V](child.builder)
			if err != nil {
				return nil, fmt.Errorf("samurai: in scope %q: %w", child.name, err)
			}

			if len(inner.children) == 0 {
				return nil, fmt.Errorf("samurai: in scope %q: builder contains no tests", child.name)
			}

			childSkipped := parentSkipped || inner.skipped
			childPaths, err := collectPathsFromChildren[V](childPath, inner.children, childSkipped)
			if err != nil {
				return nil, err
			}
			paths = append(paths, childPaths...)
		}
	}
	return paths, nil
}

// probeBuilder runs a builder in discovery mode with panic recovery.
// Returns the discovered scope or an error (including recovered panics).
func probeBuilder[V Context](builder func(*TestScope[V])) (nested *TestScope[V], err error) {
	nested = &TestScope[V]{mode: modeDiscovery}

	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*samuraiErr); ok {
				err = e
			} else {
				err = fmt.Errorf("panic during discovery: %v", r)
			}
		}
	}()

	builder(nested)
	nested.sealed.Store(true)

	return nested, nil
}

// pathNode represents a node in the test path tree.
// Intermediate nodes group shared prefixes; leaf nodes hold the full path to execute.
type pathNode struct {
	name     string
	children []*pathNode
	path     []string // non-nil only for leaf nodes
	skipped  bool     // true if this leaf path should be skipped
}

// buildPathTree groups flat leaf paths into a trie so they can be emitted as nested t.Run calls.
func buildPathTree(paths []discoveredPath) *pathNode {
	root := &pathNode{}
	for _, dp := range paths {
		node := root
		for i, segment := range dp.segments {
			var found *pathNode
			for _, child := range node.children {
				if child.name == segment {
					found = child
					break
				}
			}
			if found == nil {
				found = &pathNode{name: segment}
				node.children = append(node.children, found)
			}
			node = found
			if i == len(dp.segments)-1 {
				node.path = dp.segments
				node.skipped = dp.skipped
			}
		}
	}
	return root
}

// executeTree recursively emits nested t.Run calls from the path trie.
func executeTree[V Context](t *testing.T, node *pathNode, builder func(*TestScope[V]), factory func(W) V, cfg *runConfig) {
	for _, child := range node.children {
		t.Run(child.name, func(t *testing.T) {
			if !cfg.sequential {
				t.Parallel()
			}
			if child.path != nil {
				// Leaf node
				if child.skipped {
					t.Skip("samurai: skipped")
					return
				}
				executeScope[V](t, builder, factory, child.path)
			} else {
				// Intermediate node: recurse to create nested t.Run calls
				executeTree[V](t, child, builder, factory, cfg)
			}
		})
	}
}
