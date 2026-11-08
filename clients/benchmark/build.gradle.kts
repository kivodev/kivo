// :benchmark, the Android app on a device or emulator signed in to a server:
// Macrobenchmarks of its benchmarkRelease build, and BaselineProfileGenerator
// and PhoneProfileGenerator, which record the release build's baseline and
// startup profiles, the TV's and the phone's, from its nonMinifiedRelease
// build. Both builds are the release build, profileable, signed with the same
// key, so `adb install -r` puts either over the installed app and keeps its
// sign-in.
//
// Gradle's connected tasks (connected*AndroidTest, generateBaselineProfile)
// uninstall the app when they finish, which signs it out: use them only on an
// emulator. On a TV, from clients/, with S the TV's adb serial:
//
// Profiles, on Android 14 or later (BaselineProfileGenerator says why):
//   ./gradlew :android:app:assembleNonMinifiedRelease :benchmark:assembleNonMinifiedRelease
//   adb -s $S install -r android/app/build/outputs/apk/nonMinifiedRelease/app-nonMinifiedRelease.apk
//   adb -s $S install -r benchmark/build/outputs/apk/nonMinifiedRelease/benchmark-nonMinifiedRelease.apk
//   adb -s $S shell am instrument -w -e class com.mmcdole.kivo.benchmark.BaselineProfileGenerator \
//     [-e library <its name on the rail>] com.mmcdole.kivo.benchmark/androidx.test.runner.AndroidJUnitRunner
//   D=/sdcard/Android/media/com.mmcdole.kivo.benchmark
//   adb -s $S pull $D/BaselineProfileGenerator_browse-baseline-prof.txt android/app/src/release/generated/baselineProfiles/baseline-prof.txt
//   adb -s $S pull $D/BaselineProfileGenerator_startup-startup-prof.txt android/app/src/release/generated/baselineProfiles/startup-prof.txt
// then build and install the release build as below.
//
// The phone's journeys are added to the TV's, from PhoneProfileGenerator on a phone emulator signed in
// to a server, with the same two builds installed: run it as above with
//   -e class com.mmcdole.kivo.benchmark.PhoneProfileGenerator [-e library <its name on Libraries>]
// in the emulator's adb serial for $S, pull PhoneProfileGenerator_browse-baseline-prof.txt and PhoneProfileGenerator_startup-startup-prof.txt
// from $D to phone-baseline.txt and phone-startup.txt, and merge them into the TV's:
//   P=android/app/src/release/generated/baselineProfiles
//   scripts/merge-profile.py $P/baseline-prof.txt phone-baseline.txt
//   scripts/merge-profile.py $P/startup-prof.txt phone-startup.txt
// Recording the TV's profiles again replaces both files, so merge the phone's in again after.
//
// Benchmarks: install the build to measure, then its profile, which a
// sideloaded APK only gets when profileinstaller runs, then compile it:
//   ./gradlew :android:app:assembleBenchmarkRelease :benchmark:assembleBenchmarkRelease
//   adb -s $S install -r android/app/build/outputs/apk/benchmarkRelease/app-benchmarkRelease.apk
//   adb -s $S install -r benchmark/build/outputs/apk/benchmarkRelease/benchmark-benchmarkRelease.apk
//   adb -s $S shell am broadcast -a androidx.profileinstaller.action.INSTALL_PROFILE \
//     com.mmcdole.kivo/androidx.profileinstaller.ProfileInstallReceiver   (result=1: installed, 2: already)
//   adb -s $S shell cmd package compile -f -m speed-profile com.mmcdole.kivo
//   adb -s $S shell dumpsys package dexopt | grep -A3 '\[com.mmcdole.kivo\]'   (status=speed-profile)
//   adb -s $S shell am instrument -w -e class com.mmcdole.kivo.benchmark.BrowseBenchmark \
//     com.mmcdole.kivo.benchmark/androidx.test.runner.AndroidJUnitRunner
// TracksPanelBenchmark also needs `-e work <ID>`, a work with several audio and
// subtitle tracks, and resets compilation (CompilationMode.Partial), so below
// Android 14 it reinstalls the app and signs it out.
// The shipped build is the same with release in place of benchmarkRelease.
plugins {
    id("com.android.test")
    id("androidx.baselineprofile")
}

android {
    namespace = "com.mmcdole.kivo.benchmark"
    compileSdk = 37

    defaultConfig {
        minSdk = 28
        targetSdk = 37
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    targetProjectPath = ":android:app"
    experimentalProperties["android.experimental.self-instrumenting"] = true
}

kotlin {
    jvmToolchain(21)
}

// Generation runs on the devices adb has, never a managed device: the journeys need one signed in.
baselineProfile {
    useConnectedDevices = true
}

dependencies {
    implementation("androidx.benchmark:benchmark-macro-junit4:1.5.0")
    implementation("androidx.test.ext:junit:1.3.0")
    implementation("androidx.test.uiautomator:uiautomator:2.3.0")
}
