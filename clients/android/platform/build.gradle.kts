// :android:platform, what the Android app's screens share and draw nothing
// of: the core's Android implementations (the device, the Media3 engine with
// libass, the capability probe), the app's graph and signed-in session,
// discovery and the saved grids and settings. The Compose compiler runs here
// only so the screens' modules see these classes' stability.
plugins {
    id("com.android.library")
    kotlin("plugin.compose")
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

    api("io.coil-kt.coil3:coil:$coilVersion")
    implementation("io.coil-kt.coil3:coil-network-ktor3:$coilVersion")

    testImplementation(kotlin("test-junit"))
    testImplementation("junit:junit:4.13.2")
}
