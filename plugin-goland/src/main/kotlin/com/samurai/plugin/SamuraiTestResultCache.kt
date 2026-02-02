package com.samurai.plugin

import com.intellij.openapi.components.Service
import com.intellij.openapi.project.Project
import java.util.concurrent.ConcurrentHashMap

/**
 * Session-scoped cache mapping test paths to pass/fail status.
 * Used by the line marker provider to show status icons in the gutter.
 */
@Service(Service.Level.PROJECT)
class SamuraiTestResultCache {

    enum class TestResult { PASS, FAIL }

    private val results = ConcurrentHashMap<String, TestResult>()

    fun setResult(testPath: String, result: TestResult) {
        results[testPath] = result
    }

    fun getResult(testPath: String): TestResult? {
        return results[testPath]
    }

    fun clear() {
        results.clear()
    }

    companion object {
        fun getInstance(project: Project): SamuraiTestResultCache {
            return project.getService(SamuraiTestResultCache::class.java)
        }
    }
}
