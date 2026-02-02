package com.samurai.plugin

import com.intellij.execution.configurations.ConfigurationFactory
import com.intellij.execution.configurations.ConfigurationTypeBase
import com.intellij.execution.configurations.ConfigurationTypeUtil
import com.intellij.execution.configurations.RunConfiguration
import com.intellij.icons.AllIcons
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.NotNullLazyValue

/**
 * Configuration type for Samurai tests.
 *
 * Uses ConfigurationTypeBase (not SimpleConfigurationType) so we can expose
 * the factory for programmatic configuration creation from the gutter icon handler.
 */
class SamuraiConfigurationType : ConfigurationTypeBase(
    "SamuraiTestRunConfiguration",
    "Samurai Test",
    "Run Samurai BDD tests with source navigation",
    NotNullLazyValue.lazy { AllIcons.RunConfigurations.TestState.Run }
), DumbAware {

    val factory = object : ConfigurationFactory(this) {
        override fun createTemplateConfiguration(project: Project): RunConfiguration {
            return SamuraiRunConfiguration(project, "Samurai", this@SamuraiConfigurationType)
        }

        override fun getId(): String = "SamuraiTestConfiguration"
    }

    init {
        addFactory(factory)
    }

    companion object {
        fun getInstance(): SamuraiConfigurationType =
            ConfigurationTypeUtil.findConfigurationType(SamuraiConfigurationType::class.java)
    }
}
