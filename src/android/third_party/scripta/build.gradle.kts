// Root build script. Plugins are declared here with `apply false` so their versions
// resolve once for the whole build; each module applies the ones it needs.
plugins {
    alias(libs.plugins.androidApplication) apply false
    alias(libs.plugins.androidKotlinMultiplatformLibrary) apply false
    alias(libs.plugins.composeCompiler) apply false
    alias(libs.plugins.composeMultiplatform) apply false
    alias(libs.plugins.kotlinMultiplatform) apply false
}
