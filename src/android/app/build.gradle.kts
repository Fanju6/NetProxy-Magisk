plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.kotlin.parcelize)
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
        versionCode = 27
        versionName = "8.0.0-alpha.2"
        buildConfigField("long", "ALPHA_EXPIRES_AT_MILLIS", "1786291200000L")
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
    implementation(libs.androidx.compose.ui.tooling.preview)
    implementation(libs.androidx.compose.material3)
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
    implementation(libs.json.schema.validator)
    implementation(libs.hiddenapibypass)
    testImplementation(libs.junit)
}
