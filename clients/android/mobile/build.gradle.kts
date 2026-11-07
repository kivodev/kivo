// :android:mobile, the Android app's phone screens: Material 3 over
// :android:platform.
plugins {
    id("com.android.library")
    kotlin("plugin.compose")
}

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
    compileSdk = 37

    defaultConfig {
        minSdk = 26
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
}
