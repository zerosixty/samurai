package com.samurai.plugin

import com.goide.execution.GoBuildingRunConfiguration
import com.goide.psi.GoCallExpr
import com.goide.psi.GoFile
import com.goide.psi.GoFunctionDeclaration
import com.intellij.execution.actions.ConfigurationContext
import com.intellij.execution.actions.LazyRunConfigurationProducer
import com.intellij.execution.configurations.ConfigurationFactory
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.util.Ref
import com.intellij.psi.PsiElement
import com.intellij.psi.util.PsiTreeUtil

/**
 * Intercepts GoLand's native "Run Test" actions (gutter icon on func TestXxx,
 * right-click Run, package-level runs) and produces a SamuraiRunConfiguration
 * instead of the standard GoTestRunConfiguration when the test uses samurai.Run() or samurai.RunWith().
 *
 * This ensures our SamuraiTestLocator is active for test result navigation.
 */
class SamuraiRunConfigurationProducer : LazyRunConfigurationProducer<SamuraiRunConfiguration>() {

    companion object {
        private val LOG = Logger.getInstance(SamuraiRunConfigurationProducer::class.java)
        private val pathResolver = SamuraiPathResolver()
    }

    override fun getConfigurationFactory(): ConfigurationFactory {
        return SamuraiConfigurationType.getInstance().factory
    }

    override fun setupConfigurationFromContext(
        configuration: SamuraiRunConfiguration,
        context: ConfigurationContext,
        sourceElement: Ref<PsiElement>
    ): Boolean {
        val location = context.psiLocation ?: return false
        val goFile = location.containingFile as? GoFile ?: return false

        if (!goFile.name.endsWith("_test.go")) return false

        // Case 1: Cursor is on a samurai s.Test() call
        val callExpr = PsiTreeUtil.getParentOfType(location, GoCallExpr::class.java)
        if (callExpr != null && pathResolver.isSamuraiNamedCall(callExpr)) {
            val path = pathResolver.resolvePath(callExpr)
            if (path != null) {
                configureForPath(configuration, goFile, path)
                LOG.debug("setupConfigurationFromContext: samurai call -> ${path.toRunPattern()}")
                return true
            }
        }

        // Case 2: Cursor is on/inside a func TestXxx that contains samurai.Run()/RunWith()
        val testFunc = findContainingOrAdjacentTestFunc(location)
        if (testFunc != null && containsSamuraiRun(testFunc)) {
            configureForTestFunc(configuration, goFile, testFunc)
            LOG.debug("setupConfigurationFromContext: test func -> ${testFunc.name}")
            return true
        }

        // Case 3: Package-level run — check if file has any samurai.Run()/RunWith() calls
        if (testFunc == null && fileContainsSamuraiTests(goFile)) {
            configureForPackage(configuration, goFile)
            LOG.debug("setupConfigurationFromContext: package -> ${goFile.packageName}")
            return true
        }

        return false
    }

    override fun isConfigurationFromContext(
        configuration: SamuraiRunConfiguration,
        context: ConfigurationContext
    ): Boolean {
        val location = context.psiLocation ?: return false
        val goFile = location.containingFile as? GoFile ?: return false

        if (!goFile.name.endsWith("_test.go")) return false

        // Check if this configuration matches the current context
        val callExpr = PsiTreeUtil.getParentOfType(location, GoCallExpr::class.java)
        if (callExpr != null && pathResolver.isSamuraiNamedCall(callExpr)) {
            val path = pathResolver.resolvePath(callExpr) ?: return false
            return configuration.pattern == "^${path.toRunPattern()}$"
        }

        val testFunc = findContainingOrAdjacentTestFunc(location)
        if (testFunc != null && containsSamuraiRun(testFunc)) {
            return configuration.pattern == "^${testFunc.name}$"
        }

        return false
    }

    private fun configureRun(
        configuration: SamuraiRunConfiguration,
        goFile: GoFile,
        name: String,
        pattern: String
    ) {
        val directory = goFile.virtualFile?.parent?.path ?: return
        val packagePath = goFile.getImportPath(false)

        configuration.name = name
        configuration.kind = GoBuildingRunConfiguration.Kind.PACKAGE
        configuration.workingDirectory = directory
        configuration.pattern = pattern
        if (packagePath != null) {
            configuration.setPackage(packagePath)
        } else {
            configuration.kind = GoBuildingRunConfiguration.Kind.DIRECTORY
            configuration.directoryPath = directory
        }
    }

    private fun configureForPath(
        configuration: SamuraiRunConfiguration,
        goFile: GoFile,
        path: SamuraiPathResolver.SamuraiPath
    ) = configureRun(configuration, goFile, "Samurai: ${path.toDisplayName()}", "^${path.toRunPattern()}$")

    private fun configureForTestFunc(
        configuration: SamuraiRunConfiguration,
        goFile: GoFile,
        testFunc: GoFunctionDeclaration
    ) = configureRun(configuration, goFile, "Samurai: ${testFunc.name}", "^${testFunc.name}$")

    private fun configureForPackage(
        configuration: SamuraiRunConfiguration,
        goFile: GoFile
    ) = configureRun(configuration, goFile, "Samurai: ${goFile.packageName ?: "tests"}", "")

    /**
     * Find the test function containing the cursor, or the one whose name
     * identifier the cursor is on (for gutter icon clicks).
     */
    private fun findContainingOrAdjacentTestFunc(element: PsiElement): GoFunctionDeclaration? {
        // Direct parent traversal — cursor is inside the function
        var current: PsiElement? = element
        while (current != null) {
            if (current is GoFunctionDeclaration && isTestFunction(current)) {
                return current
            }
            current = current.parent
        }
        return null
    }

    private fun isTestFunction(func: GoFunctionDeclaration): Boolean {
        return pathResolver.isTestFunction(func)
    }

    /**
     * Check if a test function contains a samurai.Run()/RunWith() call.
     */
    private fun containsSamuraiRun(testFunc: GoFunctionDeclaration): Boolean {
        val allCalls = PsiTreeUtil.findChildrenOfType(testFunc, GoCallExpr::class.java)
        return allCalls.any { pathResolver.isRootSamuraiRun(it) }
    }

    /**
     * Check if a Go test file contains any samurai.Run()/RunWith() calls.
     */
    private fun fileContainsSamuraiTests(goFile: GoFile): Boolean {
        return goFile.functions
            .filter { isTestFunction(it) }
            .any { containsSamuraiRun(it) }
    }
}
