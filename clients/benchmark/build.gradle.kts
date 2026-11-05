// :benchmark, Macrobenchmarks of the Android app's benchmark build on a
// device or emulator signed in to a server:
//   ./gradlew :benchmark:connectedBenchmarkAndroidTest \
//     -Pandroid.testInstrumentationRunnerArguments.work=<a work's ID>
// The work is a movie or episode with several audio and subtitle tracks.
plugins {
    id("com.android.test")
}

android {
    namespace = "com.mmcdole.kivo.benchmark"
    compileSdk = 37

    defaultConfig {
        minSdk = 28
        targetSdk = 37
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildTypes {
        // Matches the app's benchmark build, which it measures.
        create("benchmark") {
            isDebuggable = true
            signingConfig = signingConfigs.getByName("debug")
            matchingFallbacks += "release"
        }
    }

    targetProjectPath = ":android"
    experimentalProperties["android.experimental.self-instrumenting"] = true
}

kotlin {
    jvmToolchain(21)
}

androidComponents {
    beforeVariants(selector().all()) { it.enable = it.buildType == "benchmark" }
}

dependencies {
    implementation("androidx.benchmark:benchmark-macro-junit4:1.5.0")
    implementation("androidx.test.ext:junit:1.3.0")
    implementation("androidx.test.uiautomator:uiautomator:2.3.0")
}
