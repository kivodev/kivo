// :android:mobile, the Android app's phone screens: Material 3 over
// :android:platform.
plugins {
    id("com.android.library")
    kotlin("plugin.compose")
    // The navigation routes are serializable classes.
    kotlin("plugin.serialization")
}

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
        val file = output.get().file("com/mmcdole/kivo/android/mobile/theme/KivoTokens.kt").asFile
        args(tokens.get().asFile.path, file.path, "com.mmcdole.kivo.android.mobile.theme")
        super.exec()
    }
}

val tokensTool = configurations.create("tokensTool")
dependencies { tokensTool(project(":tokens")) }

val composeTokens = tasks.register<ComposeTokens>("composeTokens") {
    classpath = tokensTool
    mainClass.set("com.mmcdole.kivo.tokens.MainKt")
    tokens.set(rootDir.resolve("../shared/tokens.json"))
    output.set(layout.buildDirectory.dir("generated/tokens"))
}

android {
    namespace = "com.mmcdole.kivo.android.mobile"
    // The app merges this module's resources with the TV module's: the prefix keeps their names apart.
    resourcePrefix = "mobile_"
    compileSdk = 37

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }

    buildFeatures {
        compose = true
    }
}

kotlin {
    jvmToolchain(21)
}

composeCompiler {
    stabilityConfigurationFiles.add(layout.projectDirectory.file("../compose-stability.conf"))
}

androidComponents {
    onVariants { variant ->
        variant.sources.kotlin?.addGeneratedSourceDirectory(composeTokens, ComposeTokens::output)
    }
}

dependencies {
    implementation(project(":android:platform"))

    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material3:material3-adaptive-navigation-suite")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.navigation:navigation-compose:2.10.2")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:$lifecycleVersion")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:$lifecycleVersion")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")
    implementation("io.coil-kt.coil3:coil-compose:$coilVersion")
    // The player's video surface and subtitles.
    implementation("androidx.media3:media3-ui:1.11.1")
    // The sign-in code's link as a QR code.
    implementation("com.google.zxing:core:3.5.4")

    // The view models' tests: the core against Ktor's mock engine, on the JVM.
    testImplementation(kotlin("test-junit"))
    testImplementation("junit:junit:4.13.2")
    testImplementation("io.ktor:ktor-client-mock:3.5.1")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")

    // Navigation, the letter rail and the long-press sheet, on a device or emulator.
    androidTestImplementation(platform("androidx.compose:compose-bom:2026.09.00"))
    androidTestImplementation("androidx.compose.ui:ui-test-junit4")
    debugImplementation("androidx.compose.ui:ui-test-manifest")
    androidTestImplementation("androidx.test:runner:1.7.0")
    androidTestImplementation("androidx.test.ext:junit:1.3.0")
    // Compose's test rule idles through Espresso; this one finds Android 17's input manager.
    androidTestImplementation("androidx.test.espresso:espresso-core:3.7.0")
    androidTestImplementation(kotlin("test-junit"))
}
