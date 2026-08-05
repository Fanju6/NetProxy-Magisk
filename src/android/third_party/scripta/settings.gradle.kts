@file:Suppress("UnstableApiUsage")

rootProject.name = "scripta"

enableFeaturePreview("TYPESAFE_PROJECT_ACCESSORS")

pluginManagement {
    repositories {
        google {
            mavenContent {
                includeGroupAndSubgroups("androidx")
                includeGroupAndSubgroups("com.android")
                includeGroupAndSubgroups("com.google")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositories {
        google {
            mavenContent {
                includeGroupAndSubgroups("androidx")
                includeGroupAndSubgroups("com.android")
                includeGroupAndSubgroups("com.google")
            }
        }
        mavenCentral()
    }
}

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}

// :editor  — the reusable, self-drawn virtualized code editor library (common + android + desktop)
// :sandbox — the demo app; KMP shared UI + one thin entry per platform (android app, desktop app)
include(":editor")
if (gradle.parent == null) {
    include(":sandbox:shared")
    include(":sandbox:android")
    include(":sandbox:desktop")
}
