// :android:platform, what the Android app's screens share and draw nothing
// of: the core's Android implementations (the device, the Media3 engine with
// libass, the capability probe), the app's graph and signed-in session,
// discovery, the saved grids and settings, and the credits. The Compose compiler runs here
// only so the screens' modules see these classes' stability.
plugins {
    id("com.android.library")
    kotlin("plugin.compose")
}

/** Copies shared/credits (terms.json and the texts it names) into the app's assets, for the licenses page. */
abstract class CreditsAssets : DefaultTask() {
    @get:InputDirectory
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val credits: DirectoryProperty

    @get:OutputDirectory
    abstract val output: DirectoryProperty

    @TaskAction
    fun copy() {
        val into = output.get().dir("credits").asFile
        into.deleteRecursively()
        credits.get().asFile.copyRecursively(into)
    }
}

val creditsAssets = tasks.register<CreditsAssets>("creditsAssets") {
    credits.set(rootDir.resolve("../shared/credits"))
    output.set(layout.buildDirectory.dir("generated/credits"))
}

val media3Version = "1.11.1"
val coilVersion = "3.6.3"

android {
    namespace = "com.mmcdole.kivo.android.platform"
    compileSdk = 37

    defaultConfig {
        minSdk = 26
    }
}

kotlin {
    jvmToolchain(21)
}

androidComponents {
    onVariants { variant ->
        variant.sources.assets?.addGeneratedSourceDirectory(creditsAssets, CreditsAssets::output)
    }
}

composeCompiler {
    stabilityConfigurationFiles.add(layout.projectDirectory.file("../compose-stability.conf"))
}

dependencies {
    api(project(":core"))

    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.compose.runtime:runtime")
    implementation("androidx.core:core-ktx:1.18.0")

    api("androidx.media3:media3-exoplayer:$media3Version")
    implementation("androidx.media3:media3-exoplayer-hls:$media3Version")
    implementation("androidx.media3:media3-session:$media3Version")
    // A build of Media3's FFmpeg audio decoder (DTS, DTS-HD, TrueHD, AC-3, E-AC-3 and more).
    implementation("org.jellyfin.media3:media3-ffmpeg-decoder:1.9.0+1")
    // libass for Media3: styled ASS drawn over the video.
    api("io.github.peerless2012:ass-media:0.5.1")

    // Reads res/raw/aboutlibraries.json, which the app's build writes, for the open source licenses.
    implementation("com.mikepenz:aboutlibraries-core:15.3.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")

    api("io.coil-kt.coil3:coil:$coilVersion")
    implementation("io.coil-kt.coil3:coil-network-ktor3:$coilVersion")

    testImplementation(kotlin("test-junit"))
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
}
