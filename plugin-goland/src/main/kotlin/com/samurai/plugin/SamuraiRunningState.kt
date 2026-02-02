package com.samurai.plugin

import com.goide.execution.testing.GoTestLocator
import com.goide.execution.testing.GoTestRunConfiguration
import com.goide.execution.testing.GoTestRunningState
import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer
import com.intellij.execution.Executor
import com.intellij.execution.process.ProcessEvent
import com.intellij.execution.process.ProcessHandler
import com.intellij.execution.process.ProcessListener
import com.intellij.execution.testframework.sm.runner.SMTRunnerEventsAdapter
import com.intellij.execution.testframework.sm.runner.SMTRunnerEventsListener
import com.intellij.execution.testframework.sm.runner.SMTestProxy
import com.intellij.execution.ui.ConsoleView
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.module.Module
import com.intellij.openapi.project.Project
import com.intellij.execution.runners.ExecutionEnvironment
import com.intellij.util.Alarm

/**
 * Running state for Samurai tests.
 *
 * Extends GoTestRunningState to inherit all native Go test execution behavior
 * including the hierarchical test tree. Injects SamuraiTestLocator onto each
 * test proxy node as it's created, enabling navigation from test results to
 * s.Test()/s.Then() source locations without breaking the native tree structure.
 */
class SamuraiRunningState(
    env: ExecutionEnvironment,
    module: Module,
    configuration: GoTestRunConfiguration
) : GoTestRunningState(env, module, configuration) {

    companion object {
        private val LOG = Logger.getInstance(SamuraiRunningState::class.java)
        private const val DAEMON_RESTART_DELAY_MS = 500
    }

    private val restartAlarm = Alarm(Alarm.ThreadToUse.SWING_THREAD)

    override fun createConsoleInner(
        executor: Executor,
        processHandler: ProcessHandler
    ): ConsoleView {
        // Let the parent create the fully native console — preserves
        // hierarchical test tree and all GoLand test framework behavior.
        val console = super.createConsoleInner(executor, processHandler)
            ?: throw com.intellij.execution.ExecutionException("Failed to create console")

        val project = myEnvironment.project
        val module = myModule

        // Create our locator (with GoTestLocator fallback for non-samurai subtests)
        val samuraiLocator = if (module != null) {
            SamuraiTestLocator(GoTestLocator(module))
        } else {
            null
        }

        // Clear stale results from previous runs so gutter icons reset
        SamuraiTestResultCache.getInstance(project).clear()

        // Subscribe to test events to:
        // 1. Inject SamuraiTestLocator on each proxy as it appears
        // 2. Track pass/fail for gutter icon updates
        val connection = project.messageBus.connect()
        connection.subscribe(
            SMTRunnerEventsListener.TEST_STATUS,
            object : SMTRunnerEventsAdapter() {
                override fun onTestStarted(test: SMTestProxy) {
                    if (samuraiLocator != null) {
                        test.setLocator(samuraiLocator)
                    }
                }

                override fun onSuiteStarted(suite: SMTestProxy) {
                    if (samuraiLocator != null) {
                        suite.setLocator(samuraiLocator)
                    }
                }

                override fun onTestFinished(test: SMTestProxy) {
                    updateTestResultCache(project, test)
                }

                override fun onTestFailed(test: SMTestProxy) {
                    updateTestResultCache(project, test)
                }
            }
        )

        // Dispose the message bus connection when the process terminates
        processHandler.addProcessListener(object : ProcessListener {
            override fun processTerminated(event: ProcessEvent) {
                connection.disconnect()
                restartAlarm.cancelAllRequests()
                // Trigger one final refresh for any remaining cached results
                if (!project.isDisposed) {
                    DaemonCodeAnalyzer.getInstance(project).restart("Samurai test run complete")
                }
            }
        })

        return console
    }

    private fun updateTestResultCache(project: Project, test: SMTestProxy) {
        // Don't cache skipped tests — when running a single leaf via -run pattern,
        // Go skips siblings. Caching them as FAIL would show wrong gutter icons.
        if (!test.isPassed && !test.isDefect) return

        val cache = SamuraiTestResultCache.getInstance(project)
        val testPath = buildTestPath(test) ?: return

        val result = if (test.isPassed) {
            SamuraiTestResultCache.TestResult.PASS
        } else {
            SamuraiTestResultCache.TestResult.FAIL
        }
        cache.setResult(testPath, result)

        // Debounce gutter icon refresh — schedule with delay, cancel any pending
        restartAlarm.cancelAllRequests()
        restartAlarm.addRequest({
            if (!project.isDisposed) {
                DaemonCodeAnalyzer.getInstance(project).restart("Samurai test result update")
            }
        }, DAEMON_RESTART_DELAY_MS)
    }

    private fun buildTestPath(test: SMTestProxy): String? {
        val parts = mutableListOf<String>()
        var current: SMTestProxy? = test

        while (current != null && current.parent != null) {
            val name = current.name
            if (name.isNotEmpty()) {
                // Normalize to match the format used by SamuraiPath.toRunPattern():
                // escape regex special chars and replace spaces with underscores
                parts.add(0, SamuraiPathResolver.normalizeForRunFlag(name))
            }
            current = current.parent as? SMTestProxy
        }

        return if (parts.isNotEmpty()) parts.joinToString("/") else null
    }
}
