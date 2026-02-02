package com.samurai.plugin

import com.goide.psi.*
import com.goide.stubs.index.GoFunctionIndex
import com.intellij.execution.Location
import com.intellij.execution.PsiLocation
import com.intellij.execution.testframework.sm.runner.SMTestLocator
import com.intellij.openapi.diagnostic.Logger
import com.intellij.openapi.project.Project
import com.intellij.psi.PsiElement
import com.intellij.psi.search.GlobalSearchScope
import com.intellij.psi.stubs.StubIndex
import com.intellij.psi.util.PsiTreeUtil

/**
 * Test locator for Samurai tests.
 *
 * Handles test URLs in the format: gotest://package#TestFunc/segment1/segment2
 * and navigates to the specific s.Test() call that matches the path.
 *
 * In the new API, Test() is the only scope method. Children are nested via an optional
 * builder argument: s.Test("parent", fn, func(s) { s.Test("child", fn) }).
 * To find a nested segment, we look for the builder (3rd arg) of the matching Test()
 * and recurse into it.
 *
 * Falls back to the provided GoTestLocator for non-Samurai subtests (e.g., t.Run()).
 */
class SamuraiTestLocator(
    private val fallback: SMTestLocator? = null
) : SMTestLocator {

    companion object {
        private val LOG = Logger.getInstance(SamuraiTestLocator::class.java)
        private val pathResolver = SamuraiPathResolver()
    }

    override fun getLocation(
        protocol: String,
        path: String,
        project: Project,
        scope: GlobalSearchScope
    ): List<Location<*>> {
        // Try Samurai-specific navigation first
        val result = getSamuraiLocation(protocol, path, project, scope)
        if (result.isNotEmpty()) return result

        // Fall back to GoTestLocator for non-Samurai subtests
        return fallback?.getLocation(protocol, path, project, scope) ?: emptyList()
    }

    private fun getSamuraiLocation(
        protocol: String,
        path: String,
        project: Project,
        scope: GlobalSearchScope
    ): List<Location<*>> {
        LOG.debug("SamuraiTestLocator.getLocation: protocol=$protocol, path=$path")

        if (protocol != "gotest") {
            return emptyList()
        }

        // Parse path: "package#TestFunc/segment1/segment2"
        val hashIndex = path.indexOf('#')
        if (hashIndex == -1) {
            return emptyList()
        }

        val testPath = path.substring(hashIndex + 1)

        // Split test path into segments
        val segments = testPath.split("/")
        if (segments.isEmpty()) {
            return emptyList()
        }

        val testFuncName = segments[0]
        val runSegments = if (segments.size > 1) segments.subList(1, segments.size) else emptyList()

        LOG.debug("testFuncName=$testFuncName, runSegments=$runSegments")

        // If no sub-segments, let the fallback handle top-level function navigation
        if (runSegments.isEmpty()) {
            return emptyList()
        }

        return findSamuraiCallLocation(project, scope, testFuncName, runSegments)
    }

    /**
     * Find the s.Test() call that matches the given path segments.
     */
    private fun findSamuraiCallLocation(
        project: Project,
        scope: GlobalSearchScope,
        testFuncName: String,
        runSegments: List<String>
    ): List<Location<*>> {
        val results = mutableListOf<Location<*>>()

        // Use project scope instead of the framework-provided scope,
        // which may be too narrow (module scope) and exclude test files.
        val searchScope = GlobalSearchScope.projectScope(project)

        // Use Go stub index to find the test function by name directly,
        // avoiding a full scan of all .go files in the project.
        val testFunctions = findTestFunctionsByName(project, searchScope, testFuncName)
        LOG.info("findSamuraiCallLocation: testFunctions=${testFunctions.size}, testFunc=$testFuncName, segments=$runSegments")

        for (testFunc in testFunctions) {
            LOG.debug("Found test function $testFuncName in ${testFunc.containingFile.name}")

            val target = findMatchingSamuraiCall(testFunc, runSegments)
            if (target != null) {
                LOG.debug("Found matching call: ${target.text.take(50)}...")
                results.add(PsiLocation.fromPsiElement(target))
            }
        }

        LOG.debug("findSamuraiCallLocation: results=${results.size}")
        return results
    }

    /**
     * Find Go test functions by name using the stub index for fast lookup.
     * Falls back to scanning all test files if the index returns no results.
     */
    private fun findTestFunctionsByName(
        project: Project,
        searchScope: GlobalSearchScope,
        testFuncName: String
    ): List<GoFunctionDeclaration> {
        // Try stub index lookup first — O(1) vs O(n) file scan
        try {
            val indexResults = StubIndex.getElements(
                GoFunctionIndex.KEY, testFuncName, project, searchScope, GoFunctionDeclaration::class.java
            )
            if (indexResults.isNotEmpty()) {
                return indexResults
                    .filter { it.containingFile.name.endsWith("_test.go") }
                    .toList()
            }
        } catch (e: Exception) {
            LOG.debug("Stub index lookup failed, falling back to file scan: ${e.message}")
        }

        // Fallback: scan all test files (original behavior)
        val allGoFiles = com.intellij.psi.search.FilenameIndex.getAllFilesByExt(project, "go", searchScope)
        val testFiles = allGoFiles.filter { it.name.endsWith("_test.go") }
        LOG.debug("Fallback file scan: allGoFiles=${allGoFiles.size}, testFiles=${testFiles.size}")

        return testFiles.mapNotNull { virtualFile ->
            val psiFile = com.intellij.psi.PsiManager.getInstance(project).findFile(virtualFile) as? GoFile
                ?: return@mapNotNull null
            psiFile.functions.find { it.name == testFuncName }
        }
    }

    /**
     * Find the s.Test() call within a test function matching the path segments.
     *
     * Segments are hierarchical: ["with_database", "create_user", "has_email"] means:
     * - Find s.Test("with database") at top level of samurai.Run's builder
     * - Within that Test's builder (3rd arg), find s.Test("create user")
     * - Within that Test's builder (3rd arg), find s.Test("has email")
     */
    private fun findMatchingSamuraiCall(
        testFunc: GoFunctionDeclaration,
        segments: List<String>
    ): PsiElement? {
        if (segments.isEmpty()) return testFunc.nameIdentifier

        // Step 1: Find samurai.Run(...) or samurai.RunWith(...) inside the test function
        val runCall = findSamuraiRunCall(testFunc)
        if (runCall == null) {
            LOG.debug("Could not find samurai.Run() call in ${testFunc.name}")
            return null
        }
        LOG.debug("Found samurai.Run() in ${testFunc.name}: ${runCall.text.take(80)}")

        // Step 2: Get the builder func literal (Run: arg[1], RunWith: arg[2])
        val builderIdx = pathResolver.builderArgIndex(runCall)
        val args = runCall.argumentList.expressionList
        if (args.size <= builderIdx) {
            LOG.debug("Run/RunWith call has ${args.size} args (need > $builderIdx)")
            return null
        }
        val builderFuncLit = args[builderIdx] as? GoFunctionLit
        if (builderFuncLit == null) {
            LOG.debug("Arg[$builderIdx] is not GoFunctionLit: ${args[builderIdx]::class.simpleName}")
            return null
        }

        // Step 3: Search recursively through nested Test() calls
        return searchInScope(builderFuncLit, segments, 0)
    }

    /**
     * Find the samurai.Run(...) or samurai.RunWith(...) call inside a test function.
     */
    private fun findSamuraiRunCall(testFunc: GoFunctionDeclaration): GoCallExpr? {
        val allCalls = PsiTreeUtil.findChildrenOfType(testFunc, GoCallExpr::class.java)
        return allCalls.firstOrNull { call ->
            pathResolver.isRootSamuraiRun(call)
        }
    }

    /**
     * Search for matching Test() calls within a func literal's scope.
     *
     * For each s.Test("name", fn, builder?) call that is a direct child:
     * - If name matches the target segment and it's the last segment: return it
     * - If name matches but not the last segment: recurse into the builder (3rd arg)
     */
    private fun searchInScope(
        funcLit: GoFunctionLit,
        segments: List<String>,
        depth: Int
    ): PsiElement? {
        if (depth >= segments.size) return null

        val targetSegment = normalizeSegmentForMatch(segments[depth])

        val block = funcLit.block ?: return null

        // Find all GoCallExpr that are direct children of this scope (not nested in closures)
        val directCalls = PsiTreeUtil.findChildrenOfType(block, GoCallExpr::class.java)
            .filter { isDirectChildOfScope(it, block) }

        LOG.debug("searchInScope: depth=$depth, target='$targetSegment', directCalls=${directCalls.size}")

        for (call in directCalls) {
            val methodName = getMethodName(call) ?: continue
            if (methodName != "Test") continue

            val name = extractFirstStringArg(call) ?: continue
            val normalizedName = normalizeSegmentForMatch(name)

            LOG.debug("  call: method=Test, name='$name', normalized='$normalizedName'")

            if (normalizedName != targetSegment) continue

            if (depth == segments.size - 1) {
                // Last segment — return the string literal for navigation
                val nameArg = call.argumentList.expressionList
                    .firstOrNull { it is GoStringLiteral }
                return nameArg ?: call
            }

            // Not last segment — look for builder (3rd arg, GoFunctionLit) and recurse
            val args = call.argumentList.expressionList
            val builderFuncLit = args.getOrNull(2) as? GoFunctionLit
            if (builderFuncLit != null) {
                return searchInScope(builderFuncLit, segments, depth + 1)
            }

            // No builder — can't go deeper
            return null
        }
        return null
    }

    /**
     * Check if a call is a direct child of the given scope block
     * (not nested inside another closure).
     */
    private fun isDirectChildOfScope(call: GoCallExpr, scopeBlock: GoBlock): Boolean {
        var current: PsiElement? = call.parent
        while (current != null && current != scopeBlock) {
            if (current is GoFunctionLit) return false // Inside a nested closure = different scope
            current = current.parent
        }
        return current == scopeBlock
    }

    /**
     * Get the method name from a call like s.Test.
     */
    private fun getMethodName(call: GoCallExpr): String? {
        val callee = call.expression as? GoReferenceExpression ?: return null
        if (callee.qualifier == null) return null
        return callee.identifier?.text
    }

    /**
     * Extract the first string argument from a call.
     */
    private fun extractFirstStringArg(callExpr: GoCallExpr): String? {
        val args = callExpr.argumentList.expressionList
        if (args.isEmpty()) return null
        return pathResolver.extractStringLiteral(args[0])
    }

    /**
     * Normalize a segment name for matching.
     * Go test replaces spaces with underscores in subtest names.
     */
    private fun normalizeSegmentForMatch(name: String): String {
        return SamuraiPathResolver.normalizeForRunFlag(name)
    }
}
