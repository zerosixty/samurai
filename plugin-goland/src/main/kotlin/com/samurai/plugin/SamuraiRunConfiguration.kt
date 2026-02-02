package com.samurai.plugin

import com.goide.execution.testing.GoTestRunConfiguration
import com.goide.execution.testing.GoTestRunningState
import com.intellij.execution.configurations.ConfigurationType
import com.intellij.execution.runners.ExecutionEnvironment
import com.intellij.openapi.module.Module
import com.intellij.openapi.project.Project

/**
 * Run configuration for Samurai tests.
 *
 * Extends GoTestRunConfiguration to inherit all native Go test execution behavior
 * (environment setup, module support, build tags, etc.) but overrides newRunningState()
 * to provide a custom running state with Samurai-aware test result navigation.
 */
class SamuraiRunConfiguration(
    project: Project,
    name: String,
    configurationType: ConfigurationType
) : GoTestRunConfiguration(project, name, configurationType) {

    override fun newRunningState(
        env: ExecutionEnvironment,
        module: Module
    ): GoTestRunningState {
        return SamuraiRunningState(env, module, this)
    }
}
