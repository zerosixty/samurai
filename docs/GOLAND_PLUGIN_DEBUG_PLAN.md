# GoLand Plugin Debug Plan: Fix Test Result Navigation

## Current Status

**Run + Debug from gutter: WORKING** (both modes execute tests correctly with proper test framework UI).

**Navigation from test results to source: BROKEN** — clicking a subtest (e.g., `B3`) in the test results panel navigates to the `TestScopeMultipleBranches` function declaration instead of the specific `s.Then("B3", ...)` call.

## What Was Fixed (Don't Revert These)

### 1. "Test framework quit unexpectedly" — Fixed

**Root cause**: Two bugs in `SamuraiRunningState.createConsoleInner()`:

- **Double `attachToProcess`**: Our code called `console.attachToProcess(processHandler)` inside `UIUtil.invokeLaterIfNeeded`, but the parent `GoRunningState.execute()` ALSO calls `console.attachToProcess(processHandler)` after `createConsole()` returns. Double attachment = double output = framework confusion.
- **Wrong events converter**: Used `GotestEventsConverter` (legacy plain-text parser) instead of `GoTestEventsJsonConverter` (JSON parser for modern Go SDKs). Also had `isIdBasedTestTree = true` which `GoTestConsoleProperties` does NOT set.

**Fix applied**:
- Removed `console.attachToProcess(processHandler)` from the `UIUtil.invokeLaterIfNeeded` block
- Added `myCompilationExitCode != 0` check to fall back to parent console
- Changed `SamuraiConsoleProperties.createTestEventsConverter()` to delegate to `config.createTestEventsConverter(consoleProperties, module)` (matches GoTestConsoleProperties behavior)
- Removed `isIdBasedTestTree = true`
- Added `setPrintTestingStartedTime(false)`

### 2. Unqualified `Run()` detection — Fixed

**Root cause**: `SamuraiTestLocator.findSamuraiRunCall()` and `SamuraiPathResolver.isRootSamuraiRun()` only matched qualified `samurai.Run(...)` calls. When tests are in the same package (`package samurai`), the call is just `Run(t, ...)`.

**Fix applied**: Both methods now match `exprText == "Run" || exprText.endsWith(".Run")`.

## What Didn't Work (Wrong Approaches)

### 1. Custom command-line execution (original approach)
The original plugin used `LocatableConfigurationBase` with a custom `RunningState` that manually ran `go test -json -v -run PATTERN ./...` via `GeneralCommandLine`. This broke because it lacked Go environment setup (GOPATH, module config, build tags, etc.). **Replaced with** extending `GoTestRunConfiguration` + `GoTestRunningState`.

### 2. Setting `isIdBasedTestTree = true`
`GoTestConsoleProperties` does NOT set this flag. Setting it caused a mismatch between name-based events from the converter and ID-based expectations in the tree, causing "test framework quit unexpectedly".

### 3. Using `GotestEventsConverter` directly
This is the legacy plain-text parser. Modern Go uses JSON output. The correct approach is to delegate to `GoTestRunConfiguration.createTestEventsConverter()` which calls `GoTestFramework.createTestEventsConverter()` which selects `GoTestEventsJsonConverter` for modern Go SDKs.

### 4. Registering as a GoTestFramework
The `GoTestFramework` list is **hardcoded** in `GoTestFramework$Lazy` (static initializer, no extension point). Third-party plugins cannot register custom frameworks. The standalone configuration type approach is correct.

## Remaining Bug: Navigation Doesn't Work

### Symptom
Clicking subtest `B3` in test results navigates to `TestScopeMultipleBranches` function (line 178) instead of `s.Then("B3", ...)` (line 213).

### How Navigation Works

1. User clicks test node in results panel
2. IntelliJ calls `SMTestLocator.getLocation(protocol, path, project, scope)`
3. Protocol is `"gotest"`, path is like `"github.com/zerosixty/samurai#TestScopeMultipleBranches/B/B3"`
4. `SamuraiTestLocator.getLocation()` tries Samurai-specific lookup first
5. If empty, falls back to `GoTestLocator` which navigates to the test function

### What to Investigate

The `SamuraiTestLocator.getSamuraiLocation()` is returning empty, causing the `GoTestLocator` fallback to navigate to the function level. There are several possible failure points:

#### Investigation Step 1: Verify what protocol/path values are actually received

Add logging at the entry point of `getLocation()` and check GoLand's `idea.log`:
```
Help > Show Log in Finder > idea.log
```
Look for lines like:
```
SamuraiTestLocator.getLocation: protocol=gotest, path=github.com/zerosixty/samurai#TestScopeMultipleBranches/B/B3
```

**Key question**: Is `getLocation()` even being called? The `SamuraiTestLocator` is only used when the `SamuraiConsoleProperties` is active, which only happens for `SamuraiRunConfiguration` runs. If the user ran via GoLand's native "Go Test" configuration (not the Samurai gutter icon), the standard `GoTestConsoleProperties` with its `GoTestLocator` would be used instead.

#### Investigation Step 2: Verify `FilenameIndex.getAllFilesByExt` returns files

The locator searches `_test.go` files via:
```kotlin
FilenameIndex.getAllFilesByExt(project, "go", scope)
    .filter { it.name.endsWith("_test.go") }
```

The `scope` parameter comes from `SamuraiConsoleProperties.getTestLocator()`, which is called by the framework. **The scope might be too narrow** — it could be the module's search scope which may not include the test files. Try using `GlobalSearchScope.projectScope(project)` instead.

#### Investigation Step 3: Verify PSI tree structure

The `findSamuraiRunCall()` now matches unqualified `Run(...)`, but there may be issues with:

1. **`GoFunctionLit` vs `GoLiteral`**: The second argument to `Run(t, func(s *Scope) {...})` — is it actually a `GoFunctionLit` in the PSI tree? Use GoLand's PSI Viewer (`Tools > View PSI Structure`) on `scope_test.go` to verify.

2. **`isDirectChildOfScope` may be too restrictive**: The `s.Test("B", ...)` call is inside the builder closure of `Run()`. When we look for it in `searchInScope`, we check `isDirectChildOfScope(call, block)`. This walks up from the call to check it's not inside a nested `GoFunctionLit`. But `s.Test("B", ...)` IS a direct statement in the builder block, so it should work. However, there may be intermediate PSI nodes (`GoSimpleStatement`, `GoExpressionStatement`, etc.) between the `GoCallExpr` and the `GoBlock`. The `isDirectChildOfScope` walks up via `.parent` and checks for `GoFunctionLit` — it should still work because those intermediate nodes aren't `GoFunctionLit`. But verify with PSI Viewer.

3. **The `s.When(func(w W) {...})` closure**: The `When` call has a `GoFunctionLit` argument. Inside this closure, there are no `s.Test`/`s.Then` calls, but `PsiTreeUtil.findChildrenOfType(block, GoCallExpr::class.java)` returns ALL descendants, not just direct children. The `isDirectChildOfScope` filter should exclude calls inside When's closure since it would hit a `GoFunctionLit` parent. Verify this is working correctly.

#### Investigation Step 4: Verify the search actually finds the Run call

Add more logging to `findMatchingSamuraiCall`:
```kotlin
LOG.warn("findSamuraiRunCall found: ${runCall.text.take(80)}")
LOG.warn("Builder funcLit found, block has ${block.statementList.size} statements")
```

And in `searchInScope`:
```kotlin
LOG.warn("searchInScope depth=$depth, target=$targetSegment, block children=${calls.size}")
for (call in calls) {
    val methodName = getMethodName(call)
    val name = extractFirstStringArg(call)
    LOG.warn("  call: method=$methodName, name=$name, isDirectChild=${isDirectChildOfScope(call, block)}")
}
```

#### Investigation Step 5: Test with GoLand's PSI Viewer

1. Open `scope_test.go` in GoLand
2. `Tools > View PSI Structure of Current File`
3. Click on `s.Then("B3", func(c C) {`
4. Verify the PSI tree shows:
   - `GoCallExpr` for `s.Then("B3", ...)`
   - Inside a `GoBlock` of a `GoFunctionLit` (the `s.Test("B", func(s *Scope) {...})` closure)
   - The `GoFunctionLit` is an argument of another `GoCallExpr` (`s.Test("B", ...)`)
   - Which is inside the `GoBlock` of the top-level `GoFunctionLit` (the `Run(t, func(s *Scope) {...})` builder)

### Architecture Reference

```
GoTestRunConfiguration (GoLand native, final GoTestConsoleProperties)
  └── GoTestLocator → navigates to func TestXxx and t.Run("name") via GoRunUtil.getSubTests()

SamuraiRunConfiguration extends GoTestRunConfiguration
  └── newRunningState() → SamuraiRunningState extends GoTestRunningState
      └── createConsoleInner() → SMTRunnerConsoleView with SamuraiConsoleProperties
          └── getTestLocator() → SamuraiTestLocator(fallback=GoTestLocator)
              ├── getSamuraiLocation() → PSI walk to find s.Test()/s.Then()
              └── fallback → GoTestLocator → navigates to func TestXxx
```

### GoLand API Constraints (confirmed from bytecode)

| Class | Extensible? | Notes |
|-------|------------|-------|
| `GoTestConsoleProperties` | **final** | Cannot extend |
| `GoTestRunConfiguration` | Not final | Can extend, override `newRunningState()` |
| `GoTestRunningState` | Not final | Can extend, override `createConsoleInner()` |
| `GoRunningState` fields | `myModule` (protected, nullable `Module?`), `myEnvironment` (protected), `myConfiguration` (protected) | |
| `GoBuildingRunningState` fields | `myCompilationExitCode` (protected int) | |
| `GoTestFramework` | Abstract, but list is **hardcoded** | No extension point to register custom frameworks |
| `GoTestEventsJsonConverter` | Public, 3-arg constructor `(String, String, TestConsoleProperties)` | |
| `GotestEventsConverter` | Public, 2-arg constructor `(String, TestConsoleProperties)` | Legacy text parser |

### Execution Pipeline (from bytecode analysis)

```
execute(Executor, ProgramRunner)              // GoRunningState
  ├── startProcess()                           // GoBuildingRunningState
  │   ├── if build failed: GoNopProcessHandler
  │   └── if build OK: super.startProcess()    // creates real process via GoExecutor
  └── execute(Executor, ProgramRunner, ProcessHandler)  // GoRunningState
      ├── createConsole(Executor, ProcessHandler)       // GoBuildingRunningState
      │   ├── createConsoleInner(...)                   // ★ our override point
      │   └── if myHistoryProcessHandler: replay build output
      ├── console.addMessageFilter(GoConsoleFilter)
      ├── console.addMessageFilter(computed filters)
      ├── console.attachToProcess(processHandler)       // ★ DO NOT also call this in createConsoleInner!
      └── return DefaultExecutionResult(console, processHandler)
```

### Current File States

All files are in `plugin-goland/src/main/kotlin/com/samurai/plugin/`:

| File | Status | Purpose |
|------|--------|---------|
| `SamuraiRunConfiguration.kt` | Working | Extends `GoTestRunConfiguration`, overrides `newRunningState()` |
| `SamuraiConfigurationType.kt` | Working | `ConfigurationTypeBase` with factory |
| `SamuraiRunningState.kt` | Working | Extends `GoTestRunningState`, overrides `createConsoleInner()` |
| `SamuraiConsoleProperties.kt` | Working | `SMTRunnerConsoleProperties` + `SMCustomMessagesParsing` |
| `SamuraiRunLineMarkerProvider.kt` | Working | Gutter icons on `s.Test()`/`s.Then()` lines |
| `SamuraiPathResolver.kt` | Working | Resolves PSI call → full test path for gutter icons |
| `SamuraiTestLocator.kt` | **BROKEN** | Should navigate from test results to `s.Test()`/`s.Then()` source |
| `SamuraiTestResultCache.kt` | Working | Pass/fail cache for gutter icon status |
| `plugin.xml` | Working | Extension registrations |
| `build.gradle.kts` | Working | Gradle 9.3.1, IntelliJ Platform Plugin 2.11.0, GoLand 2025.3 |

### Build & Install

```bash
cd plugin-goland
./gradlew clean buildPlugin    # must succeed
./install-plugin.sh            # copies to GoLand plugins dir
# Restart GoLand
```

### Test Case

File: `scope_test.go`, function `TestScopeMultipleBranches` (line 178):
```go
func TestScopeMultipleBranches(t *testing.T) {
    Run(t, func(s *Scope) {
        s.When(func(w W) { prefix = "root" })
        s.Then("A", func(c C) { ... })
        s.Test("B", func(s *Scope) {
            s.When(func(w W) { bValue = prefix + "-B" })
            s.Then("B1", func(c C) { ... })
            s.Then("B2", func(c C) { ... })
            s.Then("B3", func(c C) { ... })  // ← navigation should land HERE
        })
    })
}
```

Expected test path for B3: `gotest://github.com/zerosixty/samurai#TestScopeMultipleBranches/B/B3`
Expected navigation target: the `"B3"` string literal on line 213.

### Ginkgo Plugin Reference

Cloned at `/private/tmp/claude/.../scratchpad/Intellij-Ginkgo/` (may not persist). Key differences:
- Ginkgo uses `LocatableConfigurationBase` (not `GoTestRunConfiguration`)
- Ginkgo uses `RunLineMarkerContributor` (not `LineMarkerProvider`)
- Ginkgo has its own `GinkgoTestEventsConverter`
- The Samurai plugin's approach of extending `GoTestRunConfiguration` is better since it inherits Go env setup

### Useful GoLand Debugging

- **idea.log**: `Help > Show Log in Finder` — search for "SamuraiTestLocator" log messages
- **PSI Viewer**: `Tools > View PSI Structure of Current File` — inspect the PSI tree of test files
- **Debug the plugin itself**: Run GoLand with the plugin in debug mode via `./gradlew runIde` and set breakpoints in `SamuraiTestLocator`
