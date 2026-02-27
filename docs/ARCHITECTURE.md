# GoLand Plugin Architecture

## Overview

The Samurai GoLand plugin provides IDE integration for the Samurai scoped testing framework. It adds gutter icons for running tests, navigates from test results to source code, and tracks pass/fail status.

All plugin source is in `plugin-goland/src/main/kotlin/com/samurai/plugin/`.

## How It All Fits Together

```
User clicks gutter icon on s.Test("B", ...)
  → SamuraiRunLineMarkerProvider detects the call
  → SamuraiPathResolver walks PSI tree upward to build path: TestFunc/B
  → Creates SamuraiRunConfiguration with pattern ^TestFunc/B$
  → SamuraiRunConfiguration.newRunningState() → SamuraiRunningState
  → SamuraiRunningState.createConsoleInner()
      → super.createConsoleInner() builds native GoLand console (tree hierarchy)
      → Subscribes to SMTRunnerEventsListener
          → onTestStarted/onSuiteStarted: injects SamuraiTestLocator on each proxy
          → onTestFinished/onTestFailed: updates SamuraiTestResultCache
  → Go test runs with -run flag, JSON output parsed by GoTestEventsJsonConverter
  → Test results appear in hierarchical tree
  → User clicks "B3" in results → SamuraiTestLocator navigates to s.Test("B3", ...)

User clicks native GoLand gutter icon on func TestXxx
  → SamuraiRunConfigurationProducer intercepts (registered with order="first")
  → Detects samurai.Run() or samurai.RunWith() call inside the function
  → Creates SamuraiRunConfiguration instead of GoTestRunConfiguration
  → Same execution pipeline as above
```

## Core Framework: Why `Test` Re-executes Per Path

Each leaf path triggers a full re-run of the builder from scratch. Every scope variable (`var db *DB`, `var user *User`) is allocated fresh. Every parent `Test` callback runs against that fresh state. This means sibling leaf tests never share mutable state — each operates on its own database, its own records, its own resources, with its own cleanups.

If a parent `Test` callback ran only once and multiple leaf `Test`s shared the result, you'd have concurrent subtests operating on the same state. That requires mutexes, sequential execution, or careful test design to avoid conflicts — exactly what samurai eliminates by design.

This design also enables the GoLand plugin to work correctly: the framework emits nested `t.Run` calls where each `Test` scope becomes an intermediate subtest and each leaf `Test` becomes a leaf subtest. This produces a proper hierarchical test tree in GoLand, and individual assertions can be run, debugged, and navigated to independently.

## Components

### SamuraiRunConfigurationProducer.kt

**Extension point**: `runConfigurationProducer` (order="first")

Intercepts GoLand's native "Run Test" actions and produces a `SamuraiRunConfiguration` instead of `GoTestRunConfiguration` when the test uses `samurai.Run()` or `samurai.RunWith()`. Without this, clicking GoLand's native gutter icon on `func TestXxx` would use `GoTestRunConfiguration` which has `GoTestLocator` (navigates to function only, not to `s.Test()`).

Handles three contexts:
1. **Cursor on `s.Test()`**: resolves full path, sets `-run` pattern
2. **Cursor on `func TestXxx` containing `samurai.Run()`/`samurai.RunWith()`**: sets pattern to `^TestXxx$`
3. **Package-level run with samurai tests in file**: runs all tests in package

Detection uses `SamuraiPathResolver.isRootSamuraiRun()` to check for `Run`/`*.Run` and `RunWith`/`*.RunWith` calls.

### SamuraiRunConfiguration.kt

Extends `GoTestRunConfiguration`. Single override: `newRunningState()` returns `SamuraiRunningState`. Inherits all native Go test execution behavior (environment, modules, build tags, etc.).

### SamuraiConfigurationType.kt

`ConfigurationTypeBase` with ID `SamuraiTestRunConfiguration`. Exposes `factory` for programmatic configuration creation by the gutter icon handler and the producer.

### SamuraiRunningState.kt

Extends `GoTestRunningState`. Overrides `createConsoleInner()` with a critical design:

1. **Calls `super.createConsoleInner()`** to get the native GoLand console. This preserves the hierarchical test tree built by `GoTestEventsJsonConverter` inside `GoTestConsoleProperties`.
2. **Subscribes to `SMTRunnerEventsListener.TEST_STATUS`** on the project message bus:
   - `onTestStarted`/`onSuiteStarted`: calls `test.setLocator(samuraiLocator)` on each proxy node
   - `onTestFinished`/`onTestFailed`: updates `SamuraiTestResultCache` and schedules a debounced `DaemonCodeAnalyzer.restart()` to refresh gutter icons

This is the key architectural decision: we do NOT replace the console or its properties. We let the native pipeline handle tree building and event conversion, then inject our locator per-proxy via event callbacks. The message bus connection is disposed when the process terminates.

### SamuraiTestLocator.kt

Implements `SMTestLocator`. Navigates from test result URLs to source code.

**Input**: `gotest://package#TestFunc/segment1/segment2`

**Algorithm**:
1. Parse URL to extract `testFuncName` and `runSegments`
2. If no sub-segments, return empty (let fallback handle top-level function)
3. Search all `_test.go` files using `GlobalSearchScope.projectScope(project)` (not the framework scope, which may be too narrow)
4. Find the test function, locate `samurai.Run()`/`samurai.RunWith()` inside it
5. Get the builder func literal (2nd argument of `Run`, 3rd argument of `RunWith` — determined by `SamuraiPathResolver.builderArgIndex()`)
6. Recursively search nested `s.Test()` calls matching each segment
7. Return the string literal PSI element for precise navigation

**Fallback**: delegates to `GoTestLocator` for non-samurai subtests (e.g., `t.Run()`).

**Scope fix**: uses `GlobalSearchScope.projectScope(project)` instead of the framework-provided scope, which can be too narrow (module scope) and exclude test files.

### SamuraiPathResolver.kt

Resolves a PSI `GoCallExpr` (an `s.Test()` call) to a full test path. Used by both the gutter icon provider and the run configuration producer.

**Algorithm**: walks UP the PSI tree from the call site:
1. Find containing `GoFunctionLit` (the closure)
2. Find parent `GoCallExpr` (the `s.Test()` or `samurai.Run()`/`samurai.RunWith()` that owns the closure)
3. If parent is `samurai.Run()`/`samurai.RunWith()`: extract test function name, done
4. If parent is `s.Test()`: add its name to segments, continue walking up
5. Return `SamuraiPath(testFunctionName, segments)`

**Method detection**:
- `isSamuraiNamedCall()` — matches `methodName == "Test"`
- `isRootSamuraiRun()` — matches `methodName == "Run"` or `methodName == "RunWith"`
- `builderArgIndex()` — returns 1 for `Run`, 2 for `RunWith`

**SamuraiPath** provides:
- `toRunPattern()`: `TestFunc/segment1/segment2` for the `-run` flag (with regex escaping)
- `toDisplayName()`: `segment1 > segment2` for UI

### SamuraiRunLineMarkerProvider.kt

`LineMarkerProvider` that adds gutter icons next to `s.Test()` calls and `samurai.Run()`/`samurai.RunWith()` calls.

- Detects `Test` calls via `SamuraiPathResolver.isSamuraiNamedCall()`
- Detects `Run`/`RunWith` calls via `SamuraiPathResolver.isRootSamuraiRun()`
- Resolves full path via `SamuraiPathResolver.resolvePath()`
- Icon reflects status from `SamuraiTestResultCache`: play (not run), green check (pass), red X (fail)
- Click handler shows Run/Debug popup, creates `SamuraiRunConfiguration`, executes via `ExecutionUtil.runConfiguration()`

### SamuraiTestResultCache.kt

Project-level `@Service` with `ConcurrentHashMap<String, TestResult>`. Maps test paths to PASS/FAIL. Updated by `SamuraiRunningState` event callbacks. Read by `SamuraiRunLineMarkerProvider` for gutter icon status. Cleared at the start of each test run to avoid stale results.

### plugin.xml

Registers four extensions:
- `codeInsight.lineMarkerProvider` → `SamuraiRunLineMarkerProvider`
- `configurationType` → `SamuraiConfigurationType`
- `runConfigurationProducer` → `SamuraiRunConfigurationProducer` (order="first")
- `notificationGroup` → "Samurai" (balloon notifications for errors)

## GoLand API Constraints

| Class | Extensible? | Notes |
|-------|------------|-------|
| `GoTestConsoleProperties` | **final** | Cannot extend; this is why we inject locator per-proxy |
| `GoTestRunConfiguration` | Not final | We extend it in `SamuraiRunConfiguration` |
| `GoTestRunningState` | Not final | We extend it in `SamuraiRunningState` |
| `GoTestFramework` | Hardcoded list | No extension point; cannot register custom frameworks |
| `SMTestProxy.setLocator()` | Public | Our injection point for navigation |
| `SMTRunnerEventsListener` | Message bus topic | Our hook for per-proxy locator injection |

## Execution Pipeline

```
GoRunningState.execute(Executor, ProgramRunner)
  ├── startProcess()                           // GoBuildingRunningState
  │   ├── if build failed: GoNopProcessHandler
  │   └── if build OK: super.startProcess()    // creates real process
  └── execute(Executor, ProgramRunner, ProcessHandler)
      ├── createConsole(Executor, ProcessHandler)
      │   ├── createConsoleInner(...)           // ★ our override point
      │   │   ├── super.createConsoleInner()    // native console + tree
      │   │   └── subscribe to test events      // inject locator per-proxy
      │   └── if myHistoryProcessHandler: replay build output
      ├── console.addMessageFilter(GoConsoleFilter)
      ├── console.attachToProcess(processHandler)  // ★ fires AttachToProcessListener
      │   └── listener reads GoTestConsoleProperties.getTestLocator()
      │       → sets GoTestLocator on processor (we override per-proxy later)
      └── return DefaultExecutionResult(console, processHandler)
```

## Build & Install

```bash
cd plugin-goland
./gradlew clean buildPlugin    # must succeed
./install-plugin.sh            # copies to GoLand plugins dir
# Restart GoLand
```

## Debugging

- **idea.log**: `Help > Show Log in Finder` — search for `SamuraiTestLocator`, `SamuraiRunConfigurationProducer`, `SamuraiRunningState`
- **PSI Viewer**: `Tools > View PSI Structure of Current File` — inspect Go PSI tree
- **Debug plugin**: `./gradlew runIde` runs GoLand with the plugin in debug mode
- **Debug logging**: Enable for `com.samurai.plugin.SamuraiTestLocator` in Help → Diagnostic Tools → Debug Log Settings
