plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.1.0"
    id("org.jetbrains.intellij.platform") version "2.11.0"
}

group = "com.samurai"
version = providers.environmentVariable("PLUGIN_VERSION").getOrElse("0.2.0")

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

// GoLand 2025.3 requires Java 21
kotlin {
    jvmToolchain(21)
}

dependencies {
    intellijPlatform {
        // Target GoLand 2025.3 (latest)
        goland("2025.3")

        // The Go plugin is bundled with GoLand
        bundledPlugin("org.jetbrains.plugins.go")

        // Development tools
        pluginVerifier()
        zipSigner()
    }
}

intellijPlatform {
    pluginConfiguration {
        ideaVersion {
            sinceBuild = "253"
            untilBuild = "253.*"
        }
    }

    signing {
        certificateChain = providers.environmentVariable("CERTIFICATE_CHAIN")
        privateKey = providers.environmentVariable("PRIVATE_KEY")
        password = providers.environmentVariable("PRIVATE_KEY_PASSWORD")
    }

    publishing {
        token = providers.environmentVariable("PUBLISH_TOKEN")
    }

    pluginVerification {
        ides {
            recommended()
        }
    }
}

intellijPlatform {
    buildSearchableOptions = false
}

tasks {
    wrapper {
        gradleVersion = "9.3.1"
    }
}
