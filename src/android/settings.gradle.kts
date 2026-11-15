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

// Use the same pinned upstream source for all builds until Glass is published.
val miuixRevision = "d8efe1872c08c863a8b33e7044dd2a8b43038032"
val miuixOverride = providers.gradleProperty("netproxyMiuixSource").orNull
val miuixDirectory = file(miuixOverride ?: ".gradle/miuix/$miuixRevision")
fun miuixGit(vararg args: String) = providers.exec {
    commandLine("git", "-C", miuixDirectory.absolutePath, *args)
}.standardOutput.asText.get().trim()

if (miuixOverride == null && !miuixDirectory.resolve("miuix-glass/build.gradle.kts").isFile) {
    check(!gradle.startParameter.isOffline) { "MIUIX Glass is not cached. Build once online or provide the pinned checkout via -PnetproxyMiuixSource." }
    miuixDirectory.mkdirs()
    miuixGit("init", "--quiet")
    miuixGit("fetch", "--quiet", "--depth=1", "https://github.com/compose-miuix-ui/miuix.git", miuixRevision)
    miuixGit("checkout", "--quiet", "--detach", "FETCH_HEAD")
}
check(miuixDirectory.resolve("miuix-glass/build.gradle.kts").isFile && miuixGit("rev-parse", "HEAD") == miuixRevision) {
    "MIUIX Glass checkout must be at $miuixRevision: $miuixDirectory"
}
includeBuild(miuixDirectory) {
    dependencySubstitution {
        listOf("ui", "icons", "nav", "preference", "blur", "squircle", "material", "glass", "glass-icons").forEach { name ->
            substitute(module("top.yukonga.miuix.kmp:miuix-$name-android")).using(project(":miuix-$name"))
        }
    }
}
include(":app")
include(":platform")
