// :android:tv, the Android app's TV screens: Compose for TV over
// :android:platform, and the launcher's Watch Next row.
plugins {
    id("com.android.library")
    kotlin("plugin.compose")
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

android {
    namespace = "com.mmcdole.kivo.android.tv"
    compileSdk = 37

    defaultConfig {
        minSdk = 26
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
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
        variant.sources.assets?.addGeneratedSourceDirectory(creditsAssets, CreditsAssets::output)
    }
}

dependencies {
    implementation(project(":android:platform"))
    // Reads res/raw/aboutlibraries.json, which the app's build writes.
    implementation("com.mikepenz:aboutlibraries-core:15.3.0")

    implementation(platform("androidx.compose:compose-bom:2026.09.00"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.tv:tv-material:1.1.0")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:$lifecycleVersion")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:$lifecycleVersion")
    // Sections of a system trace, where a benchmark or Perfetto looks for them.
    implementation("androidx.tracing:tracing-ktx:1.3.0")

    implementation("androidx.media3:media3-ui:$media3Version")

    // Watch Next on the Android TV and Google TV home screen, refreshed in the background.
    implementation("androidx.tvprovider:tvprovider:1.1.0")
    implementation("androidx.work:work-runtime-ktx:2.12.0")

    implementation("io.coil-kt.coil3:coil-compose:$coilVersion")
    implementation("com.google.zxing:core:3.5.4")

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
