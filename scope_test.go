package samurai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestScopeBasicVariables(t *testing.T) {
	Run(t, func(s *Scope) {
		var counter int

		s.Test("setup", func(_ context.Context, w W) {
			counter = 42
		}, func(s *Scope) {
			s.Test("Check", func(_ context.Context, w W) {
				if counter != 42 {
					w.Testing().Errorf("expected 42, got %d", counter)
				}
			})
		})
	})
}

func TestScopeParallelIsolation(t *testing.T) {
	Run(t, func(s *Scope) {
		var value int // Each path gets its own copy (builder re-runs per path)

		s.Test("setup", func(_ context.Context, w W) {
			// value starts at 0 for every path — proves no cross-path leakage
			if value != 0 {
				w.Testing().Errorf("expected value=0 before setup, got %d", value)
			}
			value = 100
		}, func(s *Scope) {
			s.Test("Path1", func(_ context.Context, w W) {
				// value was set to 100 by parent setup for THIS path's execution
				if value != 100 {
					w.Testing().Errorf("Path1: expected value=100, got %d", value)
				}
				value = 1 // mutation only visible to this path
			})

			s.Test("Path2", func(_ context.Context, w W) {
				// value is still 100, not 1 — Path1's mutation didn't leak
				if value != 100 {
					w.Testing().Errorf("Path2: expected value=100, got %d", value)
				}
				value = 2
			})

			s.Test("Path3", func(_ context.Context, w W) {
				if value != 100 {
					w.Testing().Errorf("Path3: expected value=100, got %d", value)
				}
				value = 3
			})
		})
	})
}

func TestScopeNestedVariables(t *testing.T) {
	Run(t, func(s *Scope) {
		var db string

		s.Test("setup database", func(_ context.Context, w W) {
			db = "test-db"
		}, func(s *Scope) {
			var user string

			s.Test("create user", func(_ context.Context, w W) {
				user = db + "-user"
			}, func(s *Scope) {
				s.Test("Check", func(_ context.Context, w W) {
					if user != "test-db-user" {
						w.Testing().Errorf("expected test-db-user, got %s", user)
					}
				})
			})
		})
	})
}

func TestScopeNoDoubleExecution(t *testing.T) {
	var execCount int
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("setup", func(_ context.Context, w W) {
			mu.Lock()
			execCount++
			mu.Unlock()
		}, func(s *Scope) {
			s.Test("A", func(_ context.Context, _ W) {})
			s.Test("B", func(_ context.Context, _ W) {})
		})
	}, Sequential())

	// Root's Test should execute twice (once for path A, once for path B)
	// NOT during discovery + execution = would be 4
	if execCount != 2 {
		t.Errorf("expected 2 executions, got %d", execCount)
	}
}

func TestScopeWithReset(t *testing.T) {
	var resetOrder []string
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		var resource string

		s.Test("open resource", func(_ context.Context, w W) {
			resource = "opened"
			w.Cleanup(func() {
				mu.Lock()
				resetOrder = append(resetOrder, "root-reset")
				mu.Unlock()
			})
		}, func(s *Scope) {
			s.Test("use resource", func(_ context.Context, w W) {
				w.Cleanup(func() {
					mu.Lock()
					resetOrder = append(resetOrder, "child-reset")
					mu.Unlock()
				})
			}, func(s *Scope) {
				s.Test("it", func(_ context.Context, w W) {
					if resource != "opened" {
						w.Testing().Error("expected resource to be opened")
					}
				})
			})
		})
	}, Sequential())

	// Cleanups run in LIFO order within each scope, inner scopes first
	if len(resetOrder) != 2 {
		t.Errorf("expected 2 resets, got %d", len(resetOrder))
	}
	if len(resetOrder) == 2 && (resetOrder[0] != "child-reset" || resetOrder[1] != "root-reset") {
		t.Errorf("expected [child-reset, root-reset], got %v", resetOrder)
	}
}

func TestScopeDeeplyNested(t *testing.T) {
	Run(t, func(s *Scope) {
		var level1 string

		s.Test("Level1", func(_ context.Context, w W) {
			level1 = "L1"
		}, func(s *Scope) {
			var level2 string

			s.Test("Level2", func(_ context.Context, w W) {
				level2 = level1 + "-L2"
			}, func(s *Scope) {
				var level3 string

				s.Test("Level3", func(_ context.Context, w W) {
					level3 = level2 + "-L3"
				}, func(s *Scope) {
					s.Test("Level4", func(_ context.Context, w W) {
						expected := "L1-L2-L3"
						if level3 != expected {
							w.Testing().Errorf("expected %s, got %s", expected, level3)
						}
					})
				})
			})
		})
	})
}

func TestScopeMultipleBranches(t *testing.T) {
	Run(t, func(s *Scope) {
		var prefix string

		s.Test("setup", func(_ context.Context, w W) {
			prefix = "root"
		}, func(s *Scope) {
			s.Test("A", func(_ context.Context, w W) {
				expected := "root"
				if prefix != expected {
					w.Testing().Errorf("A: expected prefix=%s, got %s", expected, prefix)
				}
			})

			s.Test("B leaf", func(_ context.Context, w W) {
				// just consume prefix — setup set it to "root"
				if prefix != "root" {
					w.Testing().Errorf("B: expected prefix=root, got %s", prefix)
				}
			})
		})
	})
}

func TestScopeMultipleBranchesV2(t *testing.T) {
	Run(t, func(s *Scope) {
		var prefix string

		s.Test("setup", func(_ context.Context, w W) {
			prefix = "root"
		}, func(s *Scope) {
			s.Test("A", func(_ context.Context, w W) {
				if prefix != "root" {
					w.Testing().Errorf("A: expected prefix=root, got %s", prefix)
				}
			})

			s.Test("B setup", func(_ context.Context, w W) {
				// bValue computed inside fn
			}, func(s *Scope) {
				s.Test("B1", func(_ context.Context, w W) {
					bValue := prefix + "-B"
					if bValue != "root-B" {
						w.Testing().Errorf("B1: expected root-B, got %s", bValue)
					}
				})

				s.Test("B2", func(_ context.Context, w W) {
					bValue := prefix + "-B"
					if bValue != "root-B" {
						w.Testing().Errorf("B2: expected root-B, got %s", bValue)
					}
				})

				s.Test("B3", func(_ context.Context, w W) {
					bValue := prefix + "-B"
					if bValue != "root-B" {
						w.Testing().Errorf("B3: expected root-B, got %s", bValue)
					}
				})
			})
		})
	})
}

func TestScopeHighConcurrency(t *testing.T) {
	const numPaths = 100

	Run(t, func(s *Scope) {
		var pathValue int

		for i := range numPaths {
			name := fmt.Sprintf("Path%03d", i)
			expected := i
			s.Test(name, func(_ context.Context, w W) {
				pathValue = expected
				// Yield to encourage scheduler interleaving
				runtime.Gosched()
				if pathValue != expected {
					w.Testing().Errorf("%s: expected %d, got %d", name, expected, pathValue)
				}
			})
		}
	})
}

func TestScopeSequential(t *testing.T) {
	var order []string
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("First", func(_ context.Context, w W) {
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
		})

		s.Test("Second", func(_ context.Context, w W) {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
		})

		s.Test("Third", func(_ context.Context, w W) {
			mu.Lock()
			order = append(order, "third")
			mu.Unlock()
		})
	}, Sequential())

	// With Sequential(), order should be deterministic
	if len(order) != 3 {
		t.Errorf("expected 3 items, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" || order[2] != "third" {
		t.Errorf("unexpected order: %v", order)
	}
}

func TestScopeContextAccess(t *testing.T) {
	Run(t, func(s *Scope) {
		s.Test("CheckContext", func(ctx context.Context, w W) {
			if ctx == nil {
				w.Testing().Error("context should not be nil")
			}
			if w.Testing() == nil {
				t.Error("testing.T should not be nil")
			}
			// Context should be live (not canceled) during test execution
			if err := ctx.Err(); err != nil {
				w.Testing().Errorf("context should not be canceled during test, got: %v", err)
			}
		})
	})
}

func TestScopeTestWithChildren(t *testing.T) {
	Run(t, func(s *Scope) {
		var db string
		var user string

		s.Test("setup db", func(_ context.Context, w W) {
			db = "test-db"
		}, func(s *Scope) {
			s.Test("user exists", func(_ context.Context, w W) {
				user = db + "-user"
			}, func(s *Scope) {
				s.Test("can update email", func(_ context.Context, w W) {
					expected := "test-db-user"
					if user != expected {
						w.Testing().Errorf("expected %s, got %s", expected, user)
					}
				})

				s.Test("can delete", func(_ context.Context, w W) {
					if user == "" {
						w.Testing().Error("user should exist")
					}
				})
			})

			s.Test("can query all", func(_ context.Context, w W) {
				if db != "test-db" {
					w.Testing().Errorf("expected test-db, got %s", db)
				}
			})
		})
	})
}

func TestScopeThenTestShortcut(t *testing.T) {
	var executed []string
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("setup", func(_ context.Context, w W) {
			mu.Lock()
			executed = append(executed, "root")
			mu.Unlock()
		}, func(s *Scope) {
			s.Test("Leaf1", func(_ context.Context, w W) {
				mu.Lock()
				executed = append(executed, "leaf1")
				mu.Unlock()
			})

			s.Test("Leaf2", func(_ context.Context, w W) {
				mu.Lock()
				executed = append(executed, "leaf2")
				mu.Unlock()
			})
		})
	}, Sequential())

	// Should have executed root twice (once per leaf) + each leaf once
	// Path 1: setup -> leaf1
	// Path 2: setup -> leaf2
	if len(executed) != 4 {
		t.Errorf("expected 4 executions, got %d: %v", len(executed), executed)
	}
}

// --- Validation tests ---

func TestScopeDuplicateSiblingNamesFatal(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		s.Test("same", func(_ context.Context, _ W) {})
		s.Test("same", func(_ context.Context, _ W) {}) // duplicate!
	})
	if err == nil {
		t.Fatal("expected validation error for duplicate sibling names")
	}
}

func TestScopeDuplicateNameNestedFatal(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		s.Test("parent", func(_ context.Context, _ W) {}, func(s *Scope) {
			s.Test("same", func(_ context.Context, _ W) {})
			s.Test("same", func(_ context.Context, _ W) {}) // duplicate in nested scope!
		})
	})
	if err == nil {
		t.Fatal("expected validation error for duplicate sibling names in nested scope")
	}
}

func TestScopeEmptyBuilderFatal(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		// empty builder - no Test calls
	})
	if err == nil {
		t.Fatal("expected validation error for empty builder")
	}
}

func TestScopeTestWithEmptyBuilderFatal(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		s.Test("parent", func(_ context.Context, _ W) {}, func(s *Scope) {
			// builder with no tests inside
		})
	})
	if err == nil {
		t.Fatal("expected validation error for Test with empty builder")
	}
}

// Standalone Test (no children) is valid — it's a self-contained leaf test
func TestScopeStandaloneTest(t *testing.T) {
	var executed bool
	Run(t, func(s *Scope) {
		s.Test("standalone", func(_ context.Context, w W) {
			executed = true
		})
	}, Sequential())
	if !executed {
		t.Error("standalone Test should have executed")
	}
}

// Multiple standalone Tests are valid — they are siblings
func TestScopeMultipleStandaloneTests(t *testing.T) {
	var count int
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("A", func(_ context.Context, w W) {
			mu.Lock()
			count++
			mu.Unlock()
		})
		s.Test("B", func(_ context.Context, w W) {
			mu.Lock()
			count++
			mu.Unlock()
		})
		s.Test("C", func(_ context.Context, w W) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}, Sequential())

	if count != 3 {
		t.Errorf("expected 3 executions, got %d", count)
	}
}

// --- Edge case tests ---

func TestScopeLeafHasCleanup(t *testing.T) {
	var cleanupRan bool

	Run(t, func(s *Scope) {
		s.Test("leaf with cleanup", func(_ context.Context, w W) {
			w.Cleanup(func() {
				cleanupRan = true
			})
		})
	}, Sequential())

	if !cleanupRan {
		t.Error("cleanup should have run for leaf test")
	}
}

// --- Panic recovery test ---

// testPanicCleanupSubprocess is the in-subprocess test for TestScopePanicRecovery.
func testPanicCleanupSubprocess(t *testing.T) {
	Run(t, func(s *Scope) {
		s.Test("setup", func(_ context.Context, w W) {
			w.Cleanup(func() {
				fmt.Println("CLEANUP_RAN")
			})
		}, func(s *Scope) {
			s.Test("it panics", func(_ context.Context, w W) {
				panic("intentional panic")
			})
		})
	}, Sequential())
}

func TestScopePanicRecovery(t *testing.T) {
	if os.Getenv("TEST_PANIC_CLEANUP") == "1" {
		testPanicCleanupSubprocess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestScopePanicRecovery$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_PANIC_CLEANUP=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	// The subprocess should fail (the inner test panics and is marked failed)
	if !strings.Contains(output, "panic: intentional panic") {
		t.Errorf("expected panic to be reported, output:\n%s", output)
	}

	// Verify the cleanup actually ran despite the panic
	if !strings.Contains(output, "CLEANUP_RAN") {
		t.Errorf("cleanup should have run after panic, output:\n%s", output)
	}
}

// --- Test-panic cleanup test ---

func testTestPanicCleanupSubprocess(t *testing.T) {
	Run(t, func(s *Scope) {
		s.Test("panicking setup", func(_ context.Context, w W) {
			w.Cleanup(func() {
				fmt.Println("TEST_CLEANUP_RAN")
			})
			panic("test panic")
		}, func(s *Scope) {
			s.Test("unreachable", func(_ context.Context, _ W) {})
		})
	}, Sequential())
}

func TestScopeTestPanicCleanup(t *testing.T) {
	if os.Getenv("TEST_TEST_PANIC_CLEANUP") == "1" {
		testTestPanicCleanupSubprocess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestScopeTestPanicCleanup$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_TEST_PANIC_CLEANUP=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	if !strings.Contains(output, "panic: test panic") {
		t.Errorf("expected Test panic to be reported, output:\n%s", output)
	}
	if !strings.Contains(output, "TEST_CLEANUP_RAN") {
		t.Errorf("cleanup registered before Test panic should have run, output:\n%s", output)
	}
}

// --- Nil function and empty name validation tests ---

func TestScopeTestNilFnPanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	s := &Scope{mode: modeDiscovery}
	s.Test("x", nil)
}

func TestScopeTestEmptyNamePanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	s := &Scope{mode: modeDiscovery}
	s.Test("", func(_ context.Context, _ W) {})
}

func TestScopeTestMultipleBuildersError(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	s := &Scope{mode: modeDiscovery}
	b := func(s *Scope) {}
	s.Test("x", func(_ context.Context, _ W) {}, b, b)
}

func TestScopeTestNilBuilderArgError(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	s := &Scope{mode: modeDiscovery}
	s.Test("x", func(_ context.Context, _ W) {}, nil)
}

func testCleanupNilSubprocess(t *testing.T) {
	Run(t, func(s *Scope) {
		s.Test("setup", func(_ context.Context, w W) {
			w.Cleanup(nil)
		}, func(s *Scope) {
			s.Test("x", func(_ context.Context, _ W) {})
		})
	}, Sequential())
}

func TestScopeCleanupNilPanic(t *testing.T) {
	if os.Getenv("TEST_CLEANUP_NIL") == "1" {
		testCleanupNilSubprocess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestScopeCleanupNilPanic$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_CLEANUP_NIL=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	if !strings.Contains(output, "Cleanup called with nil function") {
		t.Errorf("expected Cleanup nil panic to be reported, output:\n%s", output)
	}
}

func TestScopeDiscoveryPanicRecovery(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		panic("boom during discovery")
	})
	if err == nil {
		t.Fatal("expected error from discovery panic")
	}
	if !strings.Contains(err.Error(), "boom during discovery") {
		t.Errorf("expected panic message in error, got: %v", err)
	}
}

// --- Conflicting options test ---

func TestScopeConflictingOptionsLastWins(t *testing.T) {
	var order []string
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("First", func(_ context.Context, w W) {
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
		})

		s.Test("Second", func(_ context.Context, w W) {
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
		})
	}, Sequential(), Parallel(), Sequential()) // last wins: Sequential

	if len(order) != 2 {
		t.Fatalf("expected 2 items, got %d", len(order))
	}
	if order[0] != "first" || order[1] != "second" {
		t.Errorf("expected sequential order [first, second], got %v", order)
	}
}

// --- Scope sealed after builder returns ---

func TestScopeSealedAfterBuilder(t *testing.T) {
	var captured *Scope

	Run(t, func(s *Scope) {
		captured = s
		s.Test("ok", func(_ context.Context, _ W) {})
	}, Sequential())

	// After Run returns, calling Test on the captured scope should panic.
	t.Run("Test", func(t *testing.T) {
		defer func() { requireSamuraiPanic(t, recover()) }()
		captured.Test("x", func(_ context.Context, _ W) {})
	})
}

// --- Reject names containing / ---

func TestScopeTestSlashInNamePanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	s := &Scope{mode: modeDiscovery}
	s.Test("foo/bar", func(_ context.Context, _ W) {})
}

// --- Run with nil builder ---

func TestRunNilBuilderPanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	Run(t, nil)
}

// --- Builder structure divergence between discovery and execution ---

func testStructureDivergenceSubprocess(t *testing.T) {
	callCount := 0
	Run(t, func(s *Scope) {
		callCount++
		if callCount == 1 {
			// Discovery phase: declare child "A"
			s.Test("A", func(_ context.Context, _ W) {})
		} else {
			// Execution phase: declare child "B" instead — different structure!
			s.Test("B", func(_ context.Context, _ W) {})
		}
	}, Sequential())
}

func TestStructureDivergence(t *testing.T) {
	if os.Getenv("TEST_STRUCTURE_DIVERGENCE") == "1" {
		testStructureDivergenceSubprocess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStructureDivergence$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_STRUCTURE_DIVERGENCE=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	// The subprocess should panic with a samuraiErr about path not found
	if !strings.Contains(output, "not found in scope") {
		t.Errorf("expected structure divergence error, output:\n%s", output)
	}
}

// --- Discovery panic recovery includes scope name ---

func TestDiscoveryPanicIncludesScopeName(t *testing.T) {
	_, err := collectScopedPaths(func(s *Scope) {
		s.Test("outer", func(_ context.Context, _ W) {}, func(s *Scope) {
			s.Test("problematic-scope", func(_ context.Context, _ W) {}, func(s *Scope) {
				panic("kaboom in child")
			})
		})
	})
	if err == nil {
		t.Fatal("expected error from discovery panic in nested scope")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "problematic-scope") {
		t.Errorf("error should identify the scope that panicked, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "kaboom in child") {
		t.Errorf("error should include the panic message, got: %v", errMsg)
	}
}

// --- RunWith tests ---

type testContext struct {
	*BaseContext
	label string
}

func newTestContext(w W) *testContext {
	return &testContext{BaseContext: w, label: "custom"}
}

func TestRunWithBasic(t *testing.T) {
	RunWith(t, newTestContext, func(s *TestScope[*testContext]) {
		var value int

		s.Test("setup", func(_ context.Context, c *testContext) {
			value = 42
		}, func(s *TestScope[*testContext]) {
			s.Test("receives factory value", func(_ context.Context, c *testContext) {
				if c.Testing() == nil {
					t.Fatal("expected non-nil testing.T in testContext")
				}
				if c.label != "custom" {
					c.Testing().Errorf("expected label=custom, got %s", c.label)
				}
				if value != 42 {
					c.Testing().Errorf("expected 42, got %d", value)
				}
			})
		})
	})
}

func TestRunWithFactoryPerLevel(t *testing.T) {
	var factoryCalls int
	var mu sync.Mutex

	factory := func(w W) *testContext {
		mu.Lock()
		factoryCalls++
		mu.Unlock()
		return &testContext{BaseContext: w, label: "custom"}
	}

	RunWith(t, factory, func(s *TestScope[*testContext]) {
		s.Test("setup", func(_ context.Context, c *testContext) {
			// setup
		}, func(s *TestScope[*testContext]) {
			s.Test("A", func(_ context.Context, c *testContext) {})
			s.Test("B", func(_ context.Context, c *testContext) {})
		})
	}, Sequential())

	// Factory is called once per executeScope call (once per scope level).
	// Path A: root level + setup level = 2 calls (A's fn runs with setup level's v)
	// Path B: root level + setup level = 2 calls (B's fn runs with setup level's v)
	// Total: 4 calls
	if factoryCalls != 4 {
		t.Errorf("expected 4 factory calls, got %d", factoryCalls)
	}
}

func TestRunWithCleanup(t *testing.T) {
	var cleanupOrder []string
	var mu sync.Mutex

	RunWith(t, newTestContext, func(s *TestScope[*testContext]) {
		s.Test("setup", func(_ context.Context, c *testContext) {
			c.Cleanup(func() {
				mu.Lock()
				cleanupOrder = append(cleanupOrder, "root-cleanup")
				mu.Unlock()
			})
		}, func(s *TestScope[*testContext]) {
			s.Test("child", func(_ context.Context, c *testContext) {
				c.Cleanup(func() {
					mu.Lock()
					cleanupOrder = append(cleanupOrder, "child-cleanup")
					mu.Unlock()
				})
			})
		})
	}, Sequential())

	if len(cleanupOrder) != 2 {
		t.Fatalf("expected 2 cleanups, got %d: %v", len(cleanupOrder), cleanupOrder)
	}
	if cleanupOrder[0] != "child-cleanup" || cleanupOrder[1] != "root-cleanup" {
		t.Errorf("expected [child-cleanup, root-cleanup], got %v", cleanupOrder)
	}
}

func TestRunWithDeeplyNested(t *testing.T) {
	RunWith(t, newTestContext, func(s *TestScope[*testContext]) {
		var l1 string

		s.Test("L1", func(_ context.Context, c *testContext) {
			l1 = "L1"
			if c.label != "custom" {
				c.Testing().Errorf("expected label=custom at L1, got %s", c.label)
			}
		}, func(s *TestScope[*testContext]) {
			var l2 string

			s.Test("L2", func(_ context.Context, c *testContext) {
				l2 = l1 + "-L2"
			}, func(s *TestScope[*testContext]) {
				var l3 string

				s.Test("L3", func(_ context.Context, c *testContext) {
					l3 = l2 + "-L3"
				}, func(s *TestScope[*testContext]) {
					s.Test("L4", func(_ context.Context, c *testContext) {
						expected := "L1-L2-L3"
						if l3 != expected {
							c.Testing().Errorf("expected %s, got %s", expected, l3)
						}
					})
				})
			})
		})
	})
}

func TestRunWithNilFactoryPanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	RunWith[*testContext](t, nil, func(s *TestScope[*testContext]) {
		s.Test("x", func(_ context.Context, _ *testContext) {})
	})
}

func TestRunWithNilBuilderPanic(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	RunWith(t, newTestContext, nil)
}

func TestRunWithTypeAlias(t *testing.T) {
	// Verify that user-defined type aliases work
	type MyScope = TestScope[*testContext]

	RunWith(t, newTestContext, func(s *MyScope) {
		s.Test("works with alias", func(_ context.Context, c *testContext) {
			if c.Testing() == nil {
				t.Fatal("expected non-nil testing.T")
			}
		})
	})
}

func TestRunWithFactoryContext(t *testing.T) {
	// Verify that w.Context() in the factory returns the same context
	// that Test callbacks receive as their first parameter.
	var factoryCtx context.Context

	RunWith(t, func(w W) *testContext {
		factoryCtx = w.Context()
		return &testContext{BaseContext: w, label: "custom"}
	}, func(s *TestScope[*testContext]) {
		s.Test("context matches", func(ctx context.Context, c *testContext) {
			if factoryCtx == nil {
				c.Testing().Fatal("factory context was nil")
			}
			if ctx != factoryCtx {
				c.Testing().Fatal("Test callback context differs from factory context")
			}
		})
	}, Sequential())
}

// --- Benchmark ---

func BenchmarkDiscovery(b *testing.B) {
	builder := func(s *Scope) {
		for i := 0; i < 10; i++ {
			branchName := fmt.Sprintf("Branch%d", i)
			s.Test(branchName, func(_ context.Context, _ W) {}, func(s *Scope) {
				for j := 0; j < 10; j++ {
					leafName := fmt.Sprintf("Leaf%d", j)
					s.Test(leafName, func(_ context.Context, _ W) {})
				}
			})
		}
	}

	for i := 0; i < b.N; i++ {
		paths, err := collectScopedPaths(builder)
		if err != nil {
			b.Fatal(err)
		}
		if len(paths) != 100 {
			b.Fatalf("expected 100 paths, got %d", len(paths))
		}
	}
}

// --- Fix: executeScope empty path with children guard ---

func TestExecuteScopeEmptyPathWithChildren(t *testing.T) {
	defer func() { requireSamuraiPanic(t, recover()) }()

	builder := func(s *Scope) {
		s.Test("child", func(_ context.Context, _ W) {})
	}
	// Empty path on a builder that produces children — should panic
	executeScope(t, builder, identityFactory, nil)
}

// --- Concurrent Cleanup safety ---

func TestScopeConcurrentCleanup(t *testing.T) {
	var cleanupCount int
	var mu sync.Mutex

	Run(t, func(s *Scope) {
		s.Test("concurrent cleanups", func(_ context.Context, w W) {
			// Register cleanups from multiple goroutines simultaneously
			var wg sync.WaitGroup
			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					w.Cleanup(func() {
						mu.Lock()
						cleanupCount++
						mu.Unlock()
					})
				}()
			}
			wg.Wait()
		})
	}, Sequential())

	if cleanupCount != 50 {
		t.Errorf("expected 50 cleanups to run, got %d", cleanupCount)
	}
}

// --- Cascading cleanup panics ---

func testCascadingCleanupPanicsSubprocess(t *testing.T) {
	Run(t, func(s *Scope) {
		s.Test("setup", func(_ context.Context, w W) {
			w.Cleanup(func() {
				fmt.Println("CLEANUP_3_RAN")
			})
			w.Cleanup(func() {
				fmt.Println("CLEANUP_2_PANIC")
				panic("cleanup 2 panics")
			})
			w.Cleanup(func() {
				fmt.Println("CLEANUP_1_PANIC")
				panic("cleanup 1 panics")
			})
		})
	}, Sequential())
}

func TestScopeCascadingCleanupPanics(t *testing.T) {
	if os.Getenv("TEST_CASCADING_CLEANUP_PANICS") == "1" {
		testCascadingCleanupPanicsSubprocess(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestScopeCascadingCleanupPanics$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_CASCADING_CLEANUP_PANICS=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	// All three cleanups should run even though two of them panic
	if !strings.Contains(output, "CLEANUP_1_PANIC") {
		t.Errorf("cleanup 1 should have run, output:\n%s", output)
	}
	if !strings.Contains(output, "CLEANUP_2_PANIC") {
		t.Errorf("cleanup 2 should have run, output:\n%s", output)
	}
	if !strings.Contains(output, "CLEANUP_3_RAN") {
		t.Errorf("cleanup 3 should have run despite earlier panics, output:\n%s", output)
	}
}

// --- Context cancellation test ---

func TestScopeContextCancelledAfterTest(t *testing.T) {
	var capturedCtx context.Context

	Run(t, func(s *Scope) {
		s.Test("capture context", func(ctx context.Context, w W) {
			capturedCtx = ctx
			// Context should be live during the test
			if err := ctx.Err(); err != nil {
				w.Testing().Errorf("context should not be canceled during test, got: %v", err)
			}
		})
	}, Sequential())

	// After Run returns, the captured context should be canceled
	// (because the subtest that created it has completed)
	if capturedCtx != nil && capturedCtx.Err() == nil {
		t.Error("context should be canceled after test completes")
	}
}

// --- Slash in name error message test ---

func TestScopeSlashErrorMessage(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for Test name containing /")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error, got %T: %v", r, r)
		}
		if !strings.Contains(err.Error(), "path separator") {
			t.Errorf("error message should explain why / is forbidden, got: %s", err.Error())
		}
	}()

	s := &Scope{mode: modeDiscovery}
	s.Test("foo/bar", func(_ context.Context, _ W) {})
}

// --- Skip tests ---

func TestScopeSkipLeaf(t *testing.T) {
	var executed bool

	Run(t, func(s *Scope) {
		s.Skip()
		s.Test("should be skipped", func(_ context.Context, w W) {
			executed = true
		})
	}, Sequential())

	if executed {
		t.Error("skipped test should not have executed")
	}
}

func TestScopeSkipParent(t *testing.T) {
	var parentRan, childRan bool

	Run(t, func(s *Scope) {
		s.Test("parent", func(_ context.Context, w W) {
			parentRan = true
		}, func(s *Scope) {
			s.Skip()
			s.Test("child", func(_ context.Context, w W) {
				childRan = true
			})
		})
	}, Sequential())

	// Parent fn should not run because all descendants are skipped.
	// executeScope is never entered for skipped paths.
	if parentRan {
		t.Error("parent callback should not have run when all children are skipped")
	}
	if childRan {
		t.Error("child callback should not have run in skipped scope")
	}
}

func TestScopeSkipMixedWithNonSkipped(t *testing.T) {
	var workingRan bool
	var skippedRan bool

	Run(t, func(s *Scope) {
		s.Test("working", func(_ context.Context, w W) {
			workingRan = true
		})

		s.Test("WIP", func(_ context.Context, w W) {}, func(s *Scope) {
			s.Skip()
			s.Test("todo", func(_ context.Context, w W) {
				skippedRan = true
			})
		})
	}, Sequential())

	if !workingRan {
		t.Error("non-skipped test should have executed")
	}
	if skippedRan {
		t.Error("skipped test should not have executed")
	}
}

func TestScopeSkipPropagatesNested(t *testing.T) {
	var innerRan bool

	Run(t, func(s *Scope) {
		s.Test("outer", func(_ context.Context, w W) {}, func(s *Scope) {
			s.Skip()
			s.Test("middle", func(_ context.Context, w W) {}, func(s *Scope) {
				// Inner scope does NOT call Skip — but inherits from parent
				s.Test("inner", func(_ context.Context, w W) {
					innerRan = true
				})
			})
		})
	}, Sequential())

	if innerRan {
		t.Error("inner test should be skipped via parent scope Skip propagation")
	}
}

func TestScopeSkipOrderDoesNotMatter(t *testing.T) {
	var firstRan, secondRan bool

	Run(t, func(s *Scope) {
		s.Test("parent", func(_ context.Context, w W) {}, func(s *Scope) {
			s.Test("first", func(_ context.Context, w W) {
				firstRan = true
			})
			s.Skip() // called AFTER first Test registration
			s.Test("second", func(_ context.Context, w W) {
				secondRan = true
			})
		})
	}, Sequential())

	if firstRan {
		t.Error("first test (registered before Skip) should still be skipped")
	}
	if secondRan {
		t.Error("second test (registered after Skip) should be skipped")
	}
}

func TestScopeSkipAtRootLevel(t *testing.T) {
	var count int

	Run(t, func(s *Scope) {
		s.Skip()
		s.Test("A", func(_ context.Context, w W) { count++ })
		s.Test("B", func(_ context.Context, w W) { count++ })
		s.Test("C", func(_ context.Context, w W) { count++ })
	}, Sequential())

	if count != 0 {
		t.Errorf("expected 0 executions with root-level Skip, got %d", count)
	}
}

func TestScopeSkipSealedPanic(t *testing.T) {
	var captured *Scope

	Run(t, func(s *Scope) {
		captured = s
		s.Test("ok", func(_ context.Context, _ W) {})
	}, Sequential())

	defer func() { requireSamuraiPanic(t, recover()) }()
	captured.Skip()
}

func TestScopeSkipNoCleanupRuns(t *testing.T) {
	var cleanupRan bool

	Run(t, func(s *Scope) {
		s.Test("parent", func(_ context.Context, w W) {
			w.Cleanup(func() { cleanupRan = true })
		}, func(s *Scope) {
			s.Skip()
			s.Test("child", func(_ context.Context, w W) {})
		})
	}, Sequential())

	// Since executeScope is never entered for skipped paths,
	// no cleanup functions are registered or executed.
	if cleanupRan {
		t.Error("cleanup should not run for skipped paths")
	}
}

func TestScopeSkipWithRunWith(t *testing.T) {
	var factoryCalled bool
	factory := func(w W) *testContext {
		factoryCalled = true
		return &testContext{BaseContext: w, label: "custom"}
	}

	RunWith(t, factory, func(s *TestScope[*testContext]) {
		s.Skip()
		s.Test("skipped", func(_ context.Context, c *testContext) {
			c.Testing().Error("should not execute")
		})
	}, Sequential())

	// Factory should NOT be called — executeScope is never entered
	if factoryCalled {
		t.Error("factory should not be called for fully skipped suite")
	}
}

func TestScopeSkipDiscovery(t *testing.T) {
	// Verify that skipped paths are still discovered (appear in output)
	paths, err := collectScopedPaths(func(s *Scope) {
		s.Test("working", func(_ context.Context, _ W) {})
		s.Test("parent", func(_ context.Context, _ W) {}, func(s *Scope) {
			s.Skip()
			s.Test("child1", func(_ context.Context, _ W) {})
			s.Test("child2", func(_ context.Context, _ W) {})
		})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(paths) != 3 {
		t.Fatalf("expected 3 discovered paths, got %d", len(paths))
	}

	// Check that non-skipped path is not marked as skipped
	if paths[0].skipped {
		t.Error("working path should not be skipped")
	}
	// Check that skipped paths are marked
	if !paths[1].skipped {
		t.Error("parent/child1 should be marked as skipped")
	}
	if !paths[2].skipped {
		t.Error("parent/child2 should be marked as skipped")
	}
}

func TestScopeSkipSubprocessOutput(t *testing.T) {
	if os.Getenv("TEST_SKIP_OUTPUT") == "1" {
		Run(t, func(s *Scope) {
			s.Test("working", func(_ context.Context, w W) {
				fmt.Println("WORKING_EXECUTED")
			})
			s.Test("skipped parent", func(_ context.Context, w W) {}, func(s *Scope) {
				s.Skip()
				s.Test("skipped child", func(_ context.Context, w W) {
					fmt.Println("SKIPPED_SHOULD_NOT_PRINT")
				})
			})
		}, Sequential())
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestScopeSkipSubprocessOutput$", "-test.v")
	cmd.Env = append(os.Environ(), "TEST_SKIP_OUTPUT=1")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if len(output) == 0 && err != nil {
		t.Fatalf("subprocess produced no output (failed to run?): %v", err)
	}

	// Working test should execute
	if !strings.Contains(output, "WORKING_EXECUTED") {
		t.Errorf("working test should have executed, output:\n%s", output)
	}

	// Skipped test should NOT execute
	if strings.Contains(output, "SKIPPED_SHOULD_NOT_PRINT") {
		t.Errorf("skipped test should not have executed, output:\n%s", output)
	}

	// Should show SKIP in output
	if !strings.Contains(output, "SKIP") {
		t.Errorf("expected SKIP in output, output:\n%s", output)
	}
}

// requireSamuraiPanic asserts that a recovered value is a *samuraiErr.
// Uses errors.As to handle wrapped errors correctly.
func requireSamuraiPanic(t *testing.T, r any) {
	t.Helper()
	if r == nil {
		t.Fatal("expected panic")
	}
	err, ok := r.(error)
	if !ok {
		t.Fatalf("expected error, got %T: %v", r, r)
	}
	var se *samuraiErr
	if !errors.As(err, &se) {
		t.Fatalf("expected *samuraiErr, got %T: %v", r, r)
	}
}
