package com.samurai.plugin

import com.goide.psi.*
import com.intellij.psi.PsiElement
import com.intellij.psi.util.PsiTreeUtil

/**
 * Resolves the full test path for a samurai s.Test() call.
 *
 * Given a PSI element representing s.Test("name", ...), walks up the closure chain
 * to build the complete path from the test function root.
 *
 * In the new API, Test() is the only scope method. Children are nested via an optional
 * builder argument: s.Test("parent", fn, func(s) { s.Test("child", fn) }).
 * The path is built by walking up parent Test() calls.
 *
 * Example: SamuraiPath("TestDatabase", ["with database", "create user", "has email"])
 *   -> toRunPattern(): "TestDatabase/with_database/create_user/has_email"
 */
class SamuraiPathResolver {

    companion object {
        // Characters that need escaping in Go test -run regex
        private val REGEX_SPECIAL = Regex("[.+*?^\${}()|\\[\\]\\\\]")

        /**
         * Normalize a test name for use in go test -run flag.
         * - Escape regex special characters
         * - Replace spaces with underscores (Go subtest naming convention)
         */
        fun normalizeForRunFlag(name: String): String {
            val escaped = REGEX_SPECIAL.replace(name) { "\\${it.value}" }
            return escaped.replace(" ", "_")
        }
    }

    /**
     * Data class representing a resolved samurai test path.
     */
    data class SamuraiPath(
        val testFunctionName: String,
        val segments: List<String>
    ) {
        /**
         * Returns the full path as a string suitable for -run flag.
         * Example: "TestDatabase/with_database/create_user/has_email"
         */
        fun toRunPattern(): String {
            return if (segments.isEmpty()) {
                testFunctionName
            } else {
                val normalizedSegments = segments.map { normalizeForRunFlag(it) }
                "$testFunctionName/${normalizedSegments.joinToString("/")}"
            }
        }

        /**
         * Returns a human-readable display name.
         * Examples:
         *   SamuraiPath("TestDatabase", [])                    -> "TestDatabase"
         *   SamuraiPath("TestDatabase", ["create user"])        -> "create user"
         *   SamuraiPath("TestDB", ["create user", "has email"]) -> "create user > has email"
         */
        fun toDisplayName(): String {
            return if (segments.isEmpty()) {
                testFunctionName
            } else {
                segments.joinToString(" > ")
            }
        }
    }

    /**
     * Returns true if this call expression is s.Test("...", ...).
     * These are the named calls that appear in the test tree and get gutter icons.
     */
    fun isSamuraiNamedCall(callExpr: GoCallExpr): Boolean {
        val callee = callExpr.expression ?: return false

        // Must be a reference expression (s.Test)
        if (callee !is GoReferenceExpression) return false

        val methodName = callee.identifier?.text ?: return false
        if (methodName != "Test") return false

        // Must have a qualifier (the receiver, e.g., "s")
        if (callee.qualifier == null) return false

        // Must have at least one argument (the name string)
        val args = callExpr.argumentList.expressionList
        if (args.isEmpty()) return false

        // First arg should be a string literal
        return args[0] is GoStringLiteral
    }

    /**
     * Extract the name string from s.Test("name", ...).
     */
    fun extractNameFromCall(callExpr: GoCallExpr): String? {
        val args = callExpr.argumentList.expressionList
        if (args.isEmpty()) return null
        val firstArg = args[0]
        return extractStringLiteral(firstArg)
    }

    /**
     * Resolves the full path for an s.Test() call expression.
     *
     * Algorithm: walk up the closure chain. At each level, if the parent call
     * is s.Test("name", ...), extract that name as a path segment. Continue
     * until we reach samurai.Run() or samurai.RunWith().
     */
    fun resolvePath(callExpr: GoCallExpr): SamuraiPath? {
        if (!isSamuraiNamedCall(callExpr)) return null

        val name = extractNameFromCall(callExpr) ?: return null
        val segments = mutableListOf(name)

        var current: PsiElement = callExpr

        while (true) {
            // Step 1: Find the containing GoFunctionLit (the closure we are inside)
            val funcLit = PsiTreeUtil.getParentOfType(current, GoFunctionLit::class.java) ?: break

            // Step 2: Walk up to the parent call
            val argList = funcLit.parent as? GoArgumentList ?: break
            val parentCall = argList.parent as? GoCallExpr ?: break

            // Step 3: Check what this parent call is
            if (isRootSamuraiRun(parentCall)) {
                // Reached samurai.Run() or samurai.RunWith() — find the test function name
                val testFunc = findContainingTestFunction(parentCall) ?: return null
                return SamuraiPath(testFunc.name ?: return null, segments)
            }

            if (isSamuraiNamedCall(parentCall)) {
                // Parent is s.Test("parentName", ...) — extract name and continue up
                val parentName = extractNameFromCall(parentCall)
                if (parentName != null) {
                    segments.add(0, parentName)
                }
                current = parentCall
                continue
            }

            // Some other call — keep walking up
            current = parentCall
        }

        // Fallback: if we walked out without finding samurai.Run,
        // still try to find the test function
        val testFunc = findContainingTestFunction(callExpr)
        if (testFunc != null) {
            return SamuraiPath(testFunc.name ?: return null, segments)
        }
        return null
    }

    /**
     * Checks if this is a root samurai.Run(t, func(*Scope), ...) or
     * samurai.RunWith(t, factory, func(*TestScope[V]), ...) call.
     *
     * Verifies the qualifier resolves to a samurai package import when qualified.
     * Unqualified calls (dot-imports) are matched by function name and argument shape.
     */
    fun isRootSamuraiRun(callExpr: GoCallExpr): Boolean {
        val callee = callExpr.expression as? GoReferenceExpression ?: return false
        val methodName = callee.identifier?.text ?: return false
        if (methodName != "Run" && methodName != "RunWith") return false

        // For qualified calls (e.g., samurai.Run), verify the qualifier is the samurai package
        val qualifier = callee.qualifier
        if (qualifier is GoReferenceExpression) {
            val goFile = callExpr.containingFile as? GoFile ?: return false
            val qualifierName = qualifier.text
            val matchingImport = goFile.imports.find { imp ->
                val alias = imp.alias
                if (alias != null) alias == qualifierName
                else imp.path?.substringAfterLast("/") == qualifierName
            }
            if (matchingImport == null || matchingImport.path?.endsWith("samurai") != true) return false
        }
        // qualifier == null means dot-import or unqualified — check args only

        val isRunWith = methodName == "RunWith"
        val args = callExpr.argumentList.expressionList

        if (isRunWith) {
            // RunWith(t, factory, func(*TestScope[V]), ...opts) — builder at index 2
            if (args.size < 3) return false
            return args[2] is GoFunctionLit
        }

        // Run(t, func(*Scope), ...opts) — builder at index 1
        if (args.size < 2) return false
        return args[1] is GoFunctionLit
    }

    /**
     * Returns the builder function literal argument index for a samurai Run/RunWith call.
     * Run: builder at index 1. RunWith: builder at index 2.
     */
    fun builderArgIndex(callExpr: GoCallExpr): Int {
        val calleeText = callExpr.expression?.text ?: ""
        val isRunWith = calleeText == "RunWith" || calleeText.endsWith(".RunWith")
        return if (isRunWith) 2 else 1
    }

    /**
     * Finds the containing test function (func TestXxx(t *testing.T)).
     */
    internal fun findContainingTestFunction(element: PsiElement): GoFunctionDeclaration? {
        var current: PsiElement? = element
        while (current != null) {
            if (current is GoFunctionDeclaration && isTestFunction(current)) {
                return current
            }
            current = current.parent
        }
        return null
    }

    /**
     * Checks if a function declaration is a Go test function: func TestXxx(t *testing.T).
     * Handles aliased testing imports (e.g., import t "testing").
     */
    fun isTestFunction(func: GoFunctionDeclaration): Boolean {
        val name = func.name ?: return false
        if (!name.startsWith("Test")) return false
        val params = func.signature?.parameters?.parameterDeclarationList ?: return false
        if (params.size != 1) return false
        return isTestingTParam(params[0])
    }

    /**
     * Checks if a parameter declaration has type *testing.T, handling aliased imports.
     */
    private fun isTestingTParam(param: GoParameterDeclaration): Boolean {
        val paramType = param.type ?: return false

        // Fast path: covers the common case
        if (paramType.text == "*testing.T") return true

        // For aliased imports, resolve the type reference
        val pointerType = paramType as? GoPointerType ?: return false
        val typeRef = PsiTreeUtil.findChildOfType(pointerType, GoTypeReferenceExpression::class.java)
            ?: return false
        if (typeRef.identifier?.text != "T") return false

        // Resolve the qualifier to check it's from the "testing" package
        val qualifier = typeRef.qualifier as? GoReferenceExpression ?: return false
        val goFile = param.containingFile as? GoFile ?: return false
        val qualifierName = qualifier.text
        val matchingImport = goFile.imports.find { imp ->
            val alias = imp.alias
            if (alias != null) alias == qualifierName
            else imp.path?.substringAfterLast("/") == qualifierName
        }
        return matchingImport?.path == "testing"
    }

    /**
     * Extracts the string value from a string literal expression.
     * Handles Go escape sequences in interpreted (double-quoted) strings.
     * Raw (backtick) strings are returned as-is.
     */
    internal fun extractStringLiteral(expr: GoExpression): String? {
        if (expr !is GoStringLiteral) return null
        val text = expr.text

        // Raw string literal — no escaping needed
        if (text.startsWith("`") && text.endsWith("`") && text.length >= 2) {
            return text.substring(1, text.length - 1)
        }

        // Interpreted string literal — unescape Go escape sequences
        if (text.startsWith("\"") && text.endsWith("\"") && text.length >= 2) {
            return unescapeGoString(text.substring(1, text.length - 1))
        }

        return null
    }

    private fun unescapeGoString(s: String): String {
        val sb = StringBuilder(s.length)
        var i = 0
        while (i < s.length) {
            if (s[i] == '\\' && i + 1 < s.length) {
                when (s[i + 1]) {
                    '\\' -> { sb.append('\\'); i += 2 }
                    '"'  -> { sb.append('"');  i += 2 }
                    'n'  -> { sb.append('\n'); i += 2 }
                    't'  -> { sb.append('\t'); i += 2 }
                    'r'  -> { sb.append('\r'); i += 2 }
                    'a'  -> { sb.append('\u0007'); i += 2 }
                    'b'  -> { sb.append('\b'); i += 2 }
                    'f'  -> { sb.append('\u000C'); i += 2 }
                    'v'  -> { sb.append('\u000B'); i += 2 }
                    else -> { sb.append(s[i]); i += 1 }
                }
            } else {
                sb.append(s[i]); i += 1
            }
        }
        return sb.toString()
    }
}
