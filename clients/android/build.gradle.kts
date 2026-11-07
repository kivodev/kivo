// :android, the Android app: one APK for Android TV, Google TV and Fire
// TV. The UI and the platform glue (the Media3 engine, the capability
// probe, libass) over :core.
plugins {
    id("com.android.application")
    kotlin("plugin.compose")
    // Writes res/raw/aboutlibraries.json with each build: the app's libraries and :core's, with their
    // licenses, for Settings › About › Credits › Open source licenses.
    id("com.mikepenz.aboutlibraries.plugin.android")
    id("androidx.baselineprofile")
}

val media3Version = "1.11.1"
val lifecycleVersion = "2.11.0"
val coilVersion = "3.6.3"

/** Runs :tokens, writing the Compose tokens from shared/tokens.json. */
abstract class ComposeTokens : JavaExec() {
    @get:InputFile
    @get:PathSensitive(PathSensitivity.NAME_ONLY)
    abstract val tokens: RegularFileProperty

    @get:OutputDirectory
    abstract val output: DirectoryProperty

    override fun exec() {
        val file = output.get().file("com/mmcdole/kivo/android/theme/KivoTokens.kt").asFile
        args(tokens.get().asFile.path, file.path, "com.mmcdole.kivo.android.theme")
        super.exec()
    }
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

val tokensTool = configurations.create("tokensTool")
dependencies { tokensTool(project(":tokens")) }

val composeTokens = tasks.register<ComposeTokens>("composeTokens") {
    classpath = tokensTool
    mainClass.set("com.mmcdole.kivo.tokens.MainKt")
    tokens.set(rootDir.resolve("../shared/tokens.json"))
    output.set(layout.buildDirectory.dir("generated/tokens"))
}

// The build number, which every request names in the app's version: -PbuildNumber when a release stamps one, else
// the commit count, so each build of a later commit is told apart.
val buildNumber: Int = providers.gradleProperty("buildNumber").orNull?.toInt()
    ?: providers.exec {
        commandLine("git", "rev-list", "--count", "HEAD")
        isIgnoreExitValue = true
    }.standardOutput.asText.get().trim().toIntOrNull()
    ?: 1

android {
    namespace = "com.mmcdole.kivo.android"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.mmcdole.kivo"
        minSdk = 26
        targetSdk = 37
        versionCode = buildNumber
        versionName = "0.4.0 ($buildNumber)" // x-release-please-version
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        // The build that ships: shrunk and optimized by R8, with the baseline profile, and signed
        // with the debug key the TVs' installs already have, so it installs over them, a debug
        // build too, without signing anyone out. The baseline profile plugin adds two builds of
        // it, both profileable and signed the same: nonMinifiedRelease, which :benchmark's
        // BaselineProfileGenerator runs against, and benchmarkRelease, which the benchmarks measure.
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.getByName("debug")
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        // The FFmpeg extension, libass and the app's own code: native libraries stay compressed.
        jniLibs.useLegacyPackaging = false
    }
}

kotlin {
    jvmToolchain(21)
}

// The release build's profiles, from :benchmark's BaselineProfileGenerator, are kept in
// src/release/generated/baselineProfiles: baseline-prof.txt for the code compiled ahead of time
// and startup-prof.txt, by which R8 puts the code startup runs first in the first dex file.
// src/main/baseline-prof.txt, written by hand, is merged in beside them.
baselineProfile {
    dexLayoutOptimization = true
}

composeCompiler {
    stabilityConfigurationFiles.add(layout.projectDirectory.file("compose-stability.conf"))
}

androidComponents {
    onVariants { variant ->
        variant.sources.kotlin?.addGeneratedSourceDirectory(composeTokens, ComposeTokens::output)
        variant.sources.assets?.addGeneratedSourceDirectory(creditsAssets, CreditsAssets::output)
    }
}

aboutLibraries {
    export {
        // Only what the page shows.
        excludeFields.addAll("developers", "funding", "scm", "description", "organization", "website")
    }
    license {
        // A library under a license not named here fails the build.
        strictMode = com.mikepenz.aboutlibraries.plugin.StrictMode.FAIL
        allowedLicenses.addAll("Apache-2.0", "MIT", "BSD-2-Clause", "BSD-3-Clause", "ISC")
        // Jellyfin's build of Media3's FFmpeg decoder.
        allowedLicensesMap = mapOf("GPL-3.0-only" to listOf("org.jellyfin.media3"))
    }
    library {
        duplicationMode = com.mikepenz.aboutlibraries.plugin.DuplicateMode.MERGE
    }
}

dependencies {
    implementation(project(":core"))
    // Reads res/raw/aboutlibraries.json.
    implementation("com.mikepenz:aboutlibraries-core:15.3.0")

    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.tv:tv-material:1.1.0")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:$lifecycleVersion")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:$lifecycleVersion")
    // Installs the baseline profile where no app store does, as on a sideloaded TV.
    implementation("androidx.profileinstaller:profileinstaller:1.4.1")
    // Sections of a system trace, where a benchmark or Perfetto looks for them.
    implementation("androidx.tracing:tracing-ktx:1.3.0")

    implementation("androidx.media3:media3-exoplayer:$media3Version")
    implementation("androidx.media3:media3-exoplayer-hls:$media3Version")
    implementation("androidx.media3:media3-ui:$media3Version")
    implementation("androidx.media3:media3-session:$media3Version")
    // A build of Media3's FFmpeg audio decoder (DTS, DTS-HD, TrueHD, AC-3, E-AC-3 and more).
    implementation("org.jellyfin.media3:media3-ffmpeg-decoder:1.9.0+1")
    // libass for Media3: styled ASS drawn over the video.
    implementation("io.github.peerless2012:ass-media:0.5.1")

    // Watch Next on the Android TV and Google TV home screen, refreshed in the background.
    implementation("androidx.tvprovider:tvprovider:1.1.0")
    implementation("androidx.work:work-runtime-ktx:2.12.0")

    implementation("io.coil-kt.coil3:coil-compose:$coilVersion")
    implementation("io.coil-kt.coil3:coil-network-ktor3:$coilVersion")
    implementation("com.google.zxing:core:3.5.4")

    baselineProfile(project(":benchmark"))

    testImplementation(kotlin("test-junit"))
    testImplementation("junit:junit:4.13.2")

    // The player's remote tests: Compose UI tests on a device or emulator, against a fake engine and Ktor's mock engine.
    androidTestImplementation(platform("androidx.compose:compose-bom:2026.09.00"))
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    androidTestImplementation("io.ktor:ktor-client-mock:3.5.1")
    // The test's clock, which the screen and the core's player waits share.
    androidTestImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
    androidTestImplementation(kotlin("test-junit"))
}
