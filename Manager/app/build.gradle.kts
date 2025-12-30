@file:Suppress("UnstableApiUsage")

import com.android.build.gradle.tasks.PackageAndroidArtifact
import com.android.build.gradle.internal.api.BaseVariantOutputImpl
import java.io.FileInputStream
import java.util.Properties

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.jetbrains.kotlin.android)
}

val keystorePropertiesFile: File = rootProject.file("keystore.properties")
val keystoreProperties = if (keystorePropertiesFile.exists() && keystorePropertiesFile.isFile) {
    Properties().apply {
        load(FileInputStream(keystorePropertiesFile))
    }
} else null

android {
    namespace = "www.netproxy.web.ui"
    compileSdk = 36

    signingConfigs {
        if (keystoreProperties != null) {
            create("release") {
                keyAlias = keystoreProperties["keyAlias"] as String
                keyPassword = keystoreProperties["keyPassword"] as String
                storeFile = file(keystoreProperties["storeFile"] as String)
                storePassword = keystoreProperties["storePassword"] as String
            }
        }
    }

    defaultConfig {
        applicationId = "www.netproxy.web.ui"
        minSdk = 26
        targetSdk = 36
        versionCode = 1
        versionName = "1.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            val releaseSig = signingConfigs.findByName("release")
            signingConfig = if (releaseSig != null) releaseSig else {
                println("use debug signing config")
                signingConfigs["debug"]
            }
        }
    }

    applicationVariants.all {
        outputs.forEach {
            val output = it as BaseVariantOutputImpl
            output.outputFileName = "NetProxy-${versionName}-${versionCode}-$name.apk"
        }
        kotlin.sourceSets {
            getByName(name) {
                kotlin.srcDir("build/generated/ksp/$name/kotlin")
            }
        }
    }
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
    // https://stackoverflow.com/a/77745844
    tasks.withType<PackageAndroidArtifact> {
        doFirst { appMetadata.asFile.orNull?.writeText("") }
    }
    androidResources {
        generateLocaleConfig = true
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_21
        targetCompatibility = JavaVersion.VERSION_21
    }
    kotlin {
        jvmToolchain {
            languageVersion.set(JavaLanguageVersion.of(21))
        }
    }
    buildFeatures {
        aidl = false
        buildConfig = true
        viewBinding = true
    }
    packaging {
        resources {
            excludes += "**"
        }
    }
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.ktx)
    implementation(libs.androidx.appcompat)
    implementation(libs.androidx.swiperefreshlayout)
    implementation(libs.androidx.recyclerview)
    implementation(libs.androidx.webkit)
    implementation(libs.material)

    implementation(libs.com.github.topjohnwu.libsu.core)
    implementation(libs.com.github.topjohnwu.libsu.service)
    implementation(libs.com.github.topjohnwu.libsu.io)

    implementation(libs.dev.rikka.rikkax.parcelablelist)
}
