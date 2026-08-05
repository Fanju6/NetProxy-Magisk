plugins {
    alias(libs.plugins.androidKotlinMultiplatformLibrary)
    alias(libs.plugins.composeCompiler)
    alias(libs.plugins.composeMultiplatform)
    alias(libs.plugins.kotlinMultiplatform)
}

kotlin {
    jvmToolchain(21)

    android {
        namespace = "top.yukonga.scripta.editor"
        compileSdk {
            version = release(37) {
                minorApiLevel = 0
            }
        }
        minSdk = 24
        withHostTest {}
    }

    jvm("desktop")

    sourceSets {
        commonMain.dependencies {
            api(libs.compose.foundation)
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
        }
    }
}
