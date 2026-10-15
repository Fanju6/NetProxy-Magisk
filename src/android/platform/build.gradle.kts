import java.util.Properties

plugins { java }

val local = Properties().apply {
    rootProject.file("local.properties").takeIf { it.isFile }?.inputStream()?.use { load(it) }
}
val sdk = providers.environmentVariable("ANDROID_HOME").orNull ?: local.getProperty("sdk.dir")
    ?: error("缺少 Android SDK 路径")
val androidJar = file("$sdk/platforms/android-37.0/android.jar")
dependencies {
    compileOnly(files(androidJar))
    testImplementation(files(androidJar))
    testImplementation(libs.junit)
}
tasks.compileJava { options.release.set(25) }

val dexDirectory = layout.buildDirectory.dir("dex")
tasks.register<JavaExec>("assembleDex") {
    dependsOn(tasks.jar, tasks.test)
    classpath = files(Class.forName("com.android.tools.r8.D8").protectionDomain.codeSource.location.toURI())
    mainClass.set("com.android.tools.r8.D8")
    inputs.file(tasks.jar.flatMap { it.archiveFile })
    inputs.file(androidJar)
    inputs.files(classpath).withPropertyName("dexCompiler").withPathSensitivity(PathSensitivity.NONE)
    outputs.dir(dexDirectory)
    doFirst {
        val output = dexDirectory.get().asFile.apply { mkdirs() }
        args = listOf("--release", "--min-api", "26", "--lib", androidJar.path,
            "--output", output.path, tasks.jar.get().archiveFile.get().asFile.path)
    }
}
