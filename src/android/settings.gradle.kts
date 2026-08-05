pluginManagement {
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
        maven("https://jitpack.io")
    }
}

rootProject.name = "NetProxyManager"

// 复合构建不会自动继承主项目的 Android SDK 路径，供内置 Scripta 复用同一份配置。
val scriptaDirectory = file("third_party/scripta")
file("local.properties").takeIf(File::isFile)?.copyTo(
    target = scriptaDirectory.resolve("local.properties"),
    overwrite = true,
)

includeBuild(scriptaDirectory)
include(":app")
