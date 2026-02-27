package com.samurai.plugin

import com.goide.execution.GoBuildingRunConfiguration
import com.goide.psi.GoCallExpr
import com.goide.psi.GoFile
import com.goide.psi.GoReferenceExpression
import com.intellij.codeInsight.daemon.GutterIconNavigationHandler
import com.intellij.codeInsight.daemon.LineMarkerInfo
import com.intellij.codeInsight.daemon.LineMarkerProvider
import com.intellij.execution.Executor
import com.intellij.execution.RunManager
import com.intellij.openapi.diagnostic.Logger
import com.intellij.execution.executors.DefaultDebugExecutor
import com.intellij.execution.executors.DefaultRunExecutor
import com.intellij.execution.runners.ExecutionUtil
import com.intellij.icons.AllIcons
import com.intellij.ide.DataManager
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.editor.markup.GutterIconRenderer
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.popup.JBPopupFactory
import com.intellij.psi.PsiElement
import com.intellij.psi.util.PsiTreeUtil
import com.intellij.ui.awt.RelativePoint
import java.awt.event.MouseEvent
import javax.swing.Icon

/**
 * Provides gutter icons (run/debug buttons) next to samurai s.Test() calls.
 *
 * Icons reflect pass/fail status from the most recent test run:
 * - Before any run: green play button
 * - After pass: green checkmark
 * - After fail: red X
 */
class SamuraiRunLineMarkerProvider : LineMarkerProvider {

    companion object {
        private val LOG = Logger.getInstance(SamuraiRunLineMarkerProvider::class.java)
        private val pathResolver = SamuraiPathResolver()
    }

    override fun getLineMarkerInfo(element: PsiElement): LineMarkerInfo<*>? {
        // We anchor to the method name identifier (e.g. "Test") which is a unique
        // leaf element for each call. Previously we used getFirstLeaf(callExpr) which resolved
        // to the receiver "s" — shared across all s.Test() calls, causing GoLand
        // to keep only one marker.

        // element must be an identifier leaf inside a GoReferenceExpression (e.g. s.Test)
        val refExpr = element.parent as? GoReferenceExpression ?: return null
        val identifier = refExpr.identifier ?: return null
        if (element !== identifier) return null

        val methodName = identifier.text

        // The GoReferenceExpression must be the callee of a GoCallExpr
        val callExpr = refExpr.parent as? GoCallExpr ?: return null
        if (callExpr.expression !== refExpr) return null

        // Case 1: s.Test() calls
        if (methodName == "Test") {
            if (!pathResolver.isSamuraiNamedCall(callExpr)) return null

            val path = pathResolver.resolvePath(callExpr) ?: return null
            val icon = getIconForPath(element.project, path)

            return LineMarkerInfo(
                element,
                element.textRange,
                icon,
                { "Run '${path.toDisplayName()}'" },
                SamuraiRunNavigationHandler(path),
                GutterIconRenderer.Alignment.LEFT,
                { "Run samurai test" }
            )
        }

        // Case 2: samurai.Run() or samurai.RunWith() calls
        if (methodName == "Run" || methodName == "RunWith") {
            if (!pathResolver.isRootSamuraiRun(callExpr)) return null

            val testFunc = pathResolver.findContainingTestFunction(callExpr) ?: return null
            val funcName = testFunc.name ?: return null
            val path = SamuraiPathResolver.SamuraiPath(funcName, emptyList())

            val icon = getIconForPath(element.project, path)

            return LineMarkerInfo(
                element,
                element.textRange,
                icon,
                { "Run '${funcName}'" },
                SamuraiRunNavigationHandler(path),
                GutterIconRenderer.Alignment.LEFT,
                { "Run samurai test" }
            )
        }

        return null
    }

    private fun getIconForPath(project: Project, path: SamuraiPathResolver.SamuraiPath): Icon {
        val cache = SamuraiTestResultCache.getInstance(project)
        return when (cache.getResult(path.toRunPattern())) {
            SamuraiTestResultCache.TestResult.PASS -> AllIcons.RunConfigurations.TestState.Green2
            SamuraiTestResultCache.TestResult.FAIL -> AllIcons.RunConfigurations.TestState.Red2
            null -> AllIcons.RunConfigurations.TestState.Run
        }
    }

    /**
     * Navigation handler that shows run/debug popup when gutter icon is clicked.
     */
    private class SamuraiRunNavigationHandler(
        private val path: SamuraiPathResolver.SamuraiPath
    ) : GutterIconNavigationHandler<PsiElement> {

        override fun navigate(e: MouseEvent, elt: PsiElement) {
            val actionGroup = DefaultActionGroup().apply {
                add(object : AnAction(
                    "Run '${path.toDisplayName()}'",
                    "Run samurai test",
                    AllIcons.RunConfigurations.TestState.Run
                ) {
                    override fun actionPerformed(e: AnActionEvent) {
                        runOrDebug(elt, path, DefaultRunExecutor.getRunExecutorInstance())
                    }
                })
                add(object : AnAction(
                    "Debug '${path.toDisplayName()}'",
                    "Debug samurai test",
                    AllIcons.RunConfigurations.TestState.Run_run
                ) {
                    override fun actionPerformed(e: AnActionEvent) {
                        runOrDebug(elt, path, DefaultDebugExecutor.getDebugExecutorInstance())
                    }
                })
            }

            val popup = JBPopupFactory.getInstance()
                .createActionGroupPopup(
                    null,
                    actionGroup,
                    DataManager.getInstance().getDataContext(e.component),
                    JBPopupFactory.ActionSelectionAid.SPEEDSEARCH,
                    false
                )

            popup.show(RelativePoint(e))
        }

        /**
         * Create a SamuraiRunConfiguration (extends GoTestRunConfiguration) and run or debug it.
         * Uses GoLand's native Go test execution for proper environment, module support, etc.
         */
        private fun runOrDebug(
            element: PsiElement,
            path: SamuraiPathResolver.SamuraiPath,
            executor: Executor
        ) {
            val project = element.project

            val goFile = element.containingFile as? GoFile
            if (goFile == null) {
                showError(project, "Cannot find Go file for element")
                return
            }

            val packagePath = goFile.getImportPath(false)
            val directory = goFile.virtualFile?.parent?.path
            if (directory == null) {
                showError(project, "Cannot get directory path")
                return
            }

            val configurationType = SamuraiConfigurationType.getInstance()
            val runManager = RunManager.getInstance(project)
            val configName = "Samurai: ${path.toDisplayName()}"

            // Find or create the run configuration
            var settings = runManager.findConfigurationByName(configName)
            if (settings == null || settings.configuration !is SamuraiRunConfiguration) {
                if (settings != null) runManager.removeConfiguration(settings)
                settings = runManager.createConfiguration(configName, configurationType.factory)
                runManager.addConfiguration(settings)
            }

            val config = settings.configuration as SamuraiRunConfiguration
            config.kind = GoBuildingRunConfiguration.Kind.PACKAGE
            config.workingDirectory = directory
            config.pattern = "^${path.toRunPattern()}$"
            if (packagePath != null) {
                config.setPackage(packagePath)
            } else {
                // Fallback to directory-based execution
                config.kind = GoBuildingRunConfiguration.Kind.DIRECTORY
                config.directoryPath = directory
            }

            runManager.selectedConfiguration = settings

            try {
                ExecutionUtil.runConfiguration(settings, executor)
            } catch (e: Exception) {
                showError(project, "Failed to execute: ${e.message}")
            }
        }

        private fun showError(project: Project, message: String) {
            try {
                NotificationGroupManager.getInstance()
                    .getNotificationGroup("Samurai")
                    ?.createNotification("Samurai Error", message, NotificationType.ERROR)
                    ?.notify(project)
            } catch (e: Exception) {
                LOG.error("Samurai Error: $message", e)
            }
        }
    }
}
