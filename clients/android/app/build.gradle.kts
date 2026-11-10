// :android:app, the Android app: one APK for Android TV, Google TV, Fire TV
// and phones. The application and its one activity, which shows :android:tv's
// screens on a TV and :android:mobile's elsewhere; the version, signing,
// R8, the baseline profile and the open source licenses.
plugins {
    id("com.android.application")
    kotlin("plugin.compose")
    // Writes res/raw/aboutlibraries.json with each build: the libraries of every module in the app, with
    // their licenses, for Settings › About › Credits › Open source licenses.
    id("com.mikepenz.aboutlibraries.plugin.android")
    id("androidx.baselineprofile")
}

// The app's version, which release-please stamps, and its build number, which every request names in the app's
// version with it: -PbuildNumber when a build stamps one, else 0.
val appVersion = "0.3.0" // x-release-please-version
val buildNumber: Int = providers.gradleProperty("buildNumber").orNull?.toInt() ?: 0

// versionCode = (major * 10000 + minor * 100 + patch) * 1000 + buildNumber, so 0.3.0 build 12 is 300012. It rises
// with each version and each build of one, and stays under Play's 2100000000 while major is under 210.
val appVersionCode: Int = run {
    val (major, minor, patch) = appVersion.split(".").map(String::toInt)
    require(minor < 100 && patch < 100) { "version $appVersion: minor and patch must be under 100" }
    require(buildNumber in 0..999) { "buildNumber $buildNumber: must be 0 to 999" }
    (major * 10000 + minor * 100 + patch) * 1000 + buildNumber
}

// The upload key Google Play takes the app bundle with, when a build has one: from gradle.properties
// (kivo.upload.storeFile and the rest, in ~/.gradle/gradle.properties, never the project's) or the
// environment (KIVO_UPLOAD_STORE_FILE and the rest). Without it the release build is signed with the
// debug key. docs/android-play.md says how the key is made.
fun uploadSetting(property: String, variable: String): String? =
    providers.gradleProperty("kivo.upload.$property").orElse(providers.environmentVariable("KIVO_UPLOAD_$variable")).orNull
val uploadStoreFile = uploadSetting("storeFile", "STORE_FILE")

android {
    namespace = "com.mmcdole.kivo.android"
    compileSdk = 37

    defaultConfig {
        applicationId = "com.mmcdole.kivo"
        minSdk = 26
        targetSdk = 37
        versionCode = appVersionCode
        versionName = "$appVersion ($buildNumber)"
        // The version every request names: a semantic version, the build number as build metadata.
        buildConfigField("String", "KIVO_VERSION", "\"$appVersion+$buildNumber\"")
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        if (uploadStoreFile != null) {
            create("upload") {
                storeFile = file(uploadStoreFile)
                storePassword = uploadSetting("storePassword", "STORE_PASSWORD")
                keyAlias = uploadSetting("keyAlias", "KEY_ALIAS")
                keyPassword = uploadSetting("keyPassword", "KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        // The build that ships: shrunk and optimized by R8, with the baseline profile. Signed with
        // the upload key when there is one, for Google Play; else with the debug key the TVs'
        // installs already have, so it installs over them, a debug build too, without signing
        // anyone out. The baseline profile plugin adds two builds of it, both profileable and
        // signed the same: nonMinifiedRelease, which :benchmark's profile generators run against,
        // and benchmarkRelease, which the benchmarks measure.
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.getByName(if (uploadStoreFile != null) "upload" else "debug")
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

    lint {
        // Lints the modules the app is built from too.
        checkDependencies = true
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
    stabilityConfigurationFiles.add(layout.projectDirectory.file("../compose-stability.conf"))
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
        // Jellyfin's build of Media3's FFmpeg decoder; on a phone, Google Play services' Cast sender,
        // which the app uses only where Google Play services is.
        allowedLicensesMap = mapOf(
            "GPL-3.0-only" to listOf("org.jellyfin.media3"),
            "ASDKL" to listOf("com.google.android.gms"),
        )
    }
    library {
        duplicationMode = com.mikepenz.aboutlibraries.plugin.DuplicateMode.MERGE
    }
}

dependencies {
    implementation(project(":android:platform"))
    implementation(project(":android:tv"))
    implementation(project(":android:mobile"))
    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.activity:activity-compose:1.13.0")
    // The activity is a fragment activity, for the phone's Cast button's device list.
    implementation("androidx.fragment:fragment-ktx:1.8.9")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    // The phone's playback service and its media session.
    implementation("androidx.media3:media3-session:1.11.1")
    // Installs the baseline profile where no app store does, as on a sideloaded TV.
    implementation("androidx.profileinstaller:profileinstaller:1.4.1")

    baselineProfile(project(":benchmark"))

    testImplementation(kotlin("test-junit"))
    testImplementation("junit:junit:4.13.2")
}
