plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.kotlin.parcelize)
}

// 使用仓库提交数作为管理器 versionCode，避免发布时手动维护两套编号。
fun gitCommitCount(): Int {
    val repositoryRoot = rootProject.rootDir.parentFile.parentFile
    return runCatching {
        val process = ProcessBuilder("git", "rev-list", "--count", "HEAD")
            .directory(repositoryRoot)
            .redirectErrorStream(true)
            .start()
        val output = process.inputStream.bufferedReader().use { it.readText().trim() }
        val exitCode = process.waitFor()
        output.toIntOrNull()?.takeIf { exitCode == 0 && it > 0 } ?: 1
    }.getOrDefault(1)
}

android {
    namespace = "com.fanjv.netproxy"
    compileSdk {
        version = release(37)
    }

    defaultConfig {
        applicationId = "com.fanjv.netproxy"
        minSdk = 31
        targetSdk = 37
        versionCode = gitCommitCount()
        versionName = "8.0.0-beta.3"
        buildConfigField("long", "ALPHA_EXPIRES_AT_MILLIS", "1787155200000L")
        ndk {
            abiFilters += "arm64-v8a"
        }
    }

    buildTypes {
        release {
            optimization.enable = true
        }
    }
    buildFeatures {
        compose = true
        buildConfig = true
    }

    experimentalProperties["android.experimental.r8.dex-startup-optimization"] = true

    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }

    lint {
        abortOnError = true
        checkReleaseBuilds = false
    }

    packaging {
        dex {
            useLegacyPackaging = true
        }
        jniLibs {
            useLegacyPackaging = true
            excludes += "lib/*/libandroidx.graphics.path.so"
        }
        resources {
            excludes += "META-INF/**"
            excludes += "kotlin/**"
            excludes += "**.bin"
            excludes += "**/DebugProbesKt.bin"
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.compose.ui)
    implementation(libs.androidx.compose.ui.graphics)
    implementation(libs.androidx.compose.material.icons.extended)

    // libsu
    implementation(libs.libsu.core)
    implementation(libs.libsu.io)

    // Miuix
    implementation(libs.miuix.ui)
    implementation(libs.miuix.icons)
    implementation(libs.miuix.navigation3.ui)
    implementation(libs.miuix.preference)
    implementation(libs.miuix.blur)
    implementation(libs.miuix.squircle)
    implementation(libs.scripta.editor)
    implementation(libs.androidx.navigation3.runtime)
    implementation(libs.androidx.navigationevent.compose)
    implementation(libs.androidx.lifecycle.viewmodel.navigation3)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.hiddenapibypass)
    testImplementation(libs.junit)
}
