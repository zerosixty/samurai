# Wrong Decisions & Failed Approaches

This document records approaches that were tried and failed during GoLand plugin development. Do NOT retry these.

## 1. Custom console with UIUtil.invokeLaterIfNeeded for initConsoleView

**What**: Override `createConsoleInner()` to create a custom `SMTRunnerConsoleView` with custom console properties, calling `SMTestRunnerConnectionUtil.initConsoleView()` inside `UIUtil.invokeLaterIfNeeded`.

**Why it failed**: `initConsoleView` registers an `AttachToProcessListener`. The parent `GoRunningState.execute()` calls `attachToProcess()` immediately after `createConsoleInner()` returns. When `initConsoleView` is deferred to EDT, the listener isn't registered when `attachToProcess` fires. Result: events arrive before the tree builder is wired up, producing a **flat test list** instead of a hierarchical tree.

**Lesson**: `initConsoleView` must complete synchronously before `createConsoleInner()` returns.

## 2. Custom console with synchronous initConsoleView

**What**: Same as above but calling `initConsoleView` synchronously (no `UIUtil.invokeLaterIfNeeded`).

**Why it failed**: Still produced a **flat test list**. The `AttachToProcessListener` registered by `initConsoleView` reads BOTH `createTestEventsConverter()` AND `getTestLocator()` from the console properties when `attachToProcess()` fires. Our custom console properties' `createTestEventsConverter()` delegates to `config.createTestEventsConverter()`, but the result is not identical to what `GoTestConsoleProperties` produces internally. The native `GoTestConsoleProperties` (which is `final`) does something additional in its converter setup that produces the hierarchical tree. Our delegation breaks it.

**Lesson**: Cannot replicate `GoTestConsoleProperties` behavior by delegation. The native console setup must be used as-is for tree hierarchy.

## 3. Swapping myProperties via reflection (after super.createConsoleInner)

**What**: Let `super.createConsoleInner()` create the native console, then use reflection to replace the `myProperties` field on `BaseTestsOutputConsoleView` with custom console properties.

**Why it failed**: The `AttachToProcessListener` (registered by `initConsoleView` inside `super.createConsoleInner()`) reads from `myProperties` when `attachToProcess()` fires. Swapping properties meant the listener used custom properties for both the events converter and the locator. Same as approach #2 — the events converter delegation breaks tree hierarchy.

**Variant tested**: Swapping properties to get navigation working, then accepting broken grouping. Navigation worked but grouping was flat. Not acceptable.

**Lesson**: `myProperties` is read by the `AttachToProcessListener` for converter creation. Swapping it breaks the tree.

## 4. Reflection to find locator field on SMTRunnerConsoleProperties

**What**: After `super.createConsoleInner()`, use reflection to find and replace a "locator" field on the console properties.

**Why it failed**: `SMTRunnerConsoleProperties.getTestLocator()` is a **method override** that returns `null` by default. There is no locator field. The locator is obtained by calling `getTestLocator()` on the properties — it's not stored in a field. Subclasses like `GoTestConsoleProperties` override the method.

**Lesson**: There is no locator field to replace via reflection. The locator comes from a virtual method call.

## 5. Registering as a GoTestFramework

**What**: Register Samurai as a custom `GoTestFramework` so GoLand natively supports it.

**Why it failed**: The `GoTestFramework` list is **hardcoded** in `GoTestFramework$Lazy` (static initializer). There is no extension point to register custom frameworks. Third-party plugins cannot add themselves to this list.

**Lesson**: Cannot register custom Go test frameworks in GoLand. Must use standalone configuration type.

## 6. Custom command-line execution (original approach)

**What**: Use `LocatableConfigurationBase` with a custom `RunningState` that manually ran `go test -json -v -run PATTERN ./...` via `GeneralCommandLine`.

**Why it failed**: Lacked Go environment setup (GOPATH, module config, build tags, etc.) that `GoTestRunConfiguration` provides. Tests would fail or use wrong Go versions.

**Lesson**: Always extend `GoTestRunConfiguration`/`GoTestRunningState` for proper Go environment setup.

## 7. Setting isIdBasedTestTree = true

**What**: Set `isIdBasedTestTree = true` on console properties.

**Why it failed**: `GoTestConsoleProperties` does NOT set this flag. Go test events use name-based tree construction (implicit hierarchy from `TestFunc/Sub/Sub2` naming). Setting ID-based mode caused a mismatch between name-based events from the converter and ID-based expectations in the tree builder, resulting in "test framework quit unexpectedly".

**Lesson**: Go test output uses name-based tree construction. Never enable ID-based mode.

## 8. Using GotestEventsConverter directly

**What**: Use `GotestEventsConverter` (2-arg constructor) for parsing test output.

**Why it failed**: `GotestEventsConverter` is the **legacy plain-text parser**. Modern Go (1.14+) uses JSON output. The correct converter is `GoTestEventsJsonConverter`, selected automatically by `GoTestRunConfiguration.createTestEventsConverter()`.

**Lesson**: Always delegate converter creation to `config.createTestEventsConverter()` or let the native pipeline handle it.

## 9. Double attachToProcess

**What**: Call `console.attachToProcess(processHandler)` inside `createConsoleInner()`.

**Why it failed**: The parent `GoRunningState.execute()` ALSO calls `console.attachToProcess(processHandler)` after `createConsole()` returns. Double attachment causes double output, confusing the test framework. Result: "test framework quit unexpectedly".

**Lesson**: Never call `attachToProcess` in `createConsoleInner()`. The parent handles it.

## What Actually Works

The working approach (documented in ARCHITECTURE.md):
1. `super.createConsoleInner()` creates the native GoLand console untouched
2. Subscribe to `SMTRunnerEventsListener.TEST_STATUS` via project message bus
3. In `onTestStarted`/`onSuiteStarted`: call `proxy.setLocator(samuraiLocator)` on each test proxy
4. In `onTestFinished`/`onTestFailed`: update `SamuraiTestResultCache`
5. Dispose the message bus connection when the process terminates

This gives us: native tree hierarchy (from GoTestConsoleProperties) + custom navigation (from SamuraiTestLocator injected per-proxy) + gutter icon status updates.
