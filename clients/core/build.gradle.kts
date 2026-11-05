import org.jetbrains.kotlin.gradle.plugin.mpp.apple.XCFramework
import org.jetbrains.kotlin.gradle.dsl.abi.ExperimentalAbiValidation

// :core, the Kotlin Multiplatform core the native apps share: one package per
// area over :api. The platforms implement its small interfaces (Device,
// PlayerEngine, BookAudio, CapabilityProbe, SecureStore, DownloadTransport). The
// download store is SQLDelight's, its schema in src/commonMain/sqldelight.
// The Apple apps get it as KivoCore.xcframework, :api exported with it,
// through SKIE (`./gradlew :core:assembleKivoCoreDebugXCFramework`, or
// Release; clients/apple/scripts/build-core.sh runs it for Xcode).
plugins {
    kotlin("multiplatform")
    kotlin("plugin.serialization")
    id("com.android.kotlin.multiplatform.library")
    id("co.touchlab.skie")
    id("app.cash.sqldelight")
}

val ktorVersion = "3.5.1"
val sqldelightVersion = "2.4.1"

/**
 * Writes the shared display tables (shared/display: one table per locale
 * and media.json, and languages/: each locale's language names and the
 * tag aliases) into a Kotlin object as JSON text, which display parses,
 * so the tables have one source and need no resource loading on any target.
 */
abstract class DisplayTables : DefaultTask() {
    @get:InputFiles
    @get:PathSensitive(PathSensitivity.NAME_ONLY)
    abstract val tables: ConfigurableFileCollection

    @get:InputFiles
    @get:PathSensitive(PathSensitivity.NAME_ONLY)
    abstract val languages: ConfigurableFileCollection

    @get:OutputDirectory
    abstract val output: DirectoryProperty

    @TaskAction
    fun write() {
        fun literal(json: String) = "\"" + json
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
            .replace("\$", "\\\$")
            .replace("\n", "\\n") + "\""
        val files = tables.files.sortedBy { it.name }
        val media = files.single { it.name == "media.json" }
        val locales = files.filter { it != media }
        val languageFiles = languages.files.sortedBy { it.name }
        val aliases = languageFiles.single { it.name == "aliases.json" }
        val out = output.get().file("com/mmcdole/kivo/core/display/DisplayJson.kt").asFile
        out.parentFile.mkdirs()
        out.writeText(
            buildString {
                appendLine("// Generated from shared/display by :core's displayTables task. Don't edit.")
                appendLine("package com.mmcdole.kivo.core.display")
                appendLine()
                appendLine("internal object DisplayJson {")
                appendLine("    val locales: Map<String, String> = mapOf(")
                for (file in locales) appendLine("        \"${file.nameWithoutExtension}\" to ${literal(file.readText())},")
                appendLine("    )")
                appendLine("    const val MEDIA: String = ${literal(media.readText())}")
                appendLine("    val languages: Map<String, String> = mapOf(")
                for (file in languageFiles - aliases) appendLine("        \"${file.nameWithoutExtension}\" to ${literal(file.readText())},")
                appendLine("    )")
                appendLine("    const val LANGUAGE_ALIASES: String = ${literal(aliases.readText())}")
                appendLine("}")
            },
        )
    }
}

val displayTables = tasks.register<DisplayTables>("displayTables") {
    tables.from(fileTree(rootDir.resolve("../shared/display")) { include("*.json") })
    languages.from(fileTree(rootDir.resolve("../shared/display/languages")) { include("*.json") })
    output.set(layout.buildDirectory.dir("generated/display"))
}

kotlin {
    jvmToolchain(21)
    // The public API the apps see, dumped to api/ (the JVM's and the
    // klibs', which is what reaches Swift) and checked by `check`:
    // `./gradlew :core:updateKotlinAbi` rewrites the dump when a change to
    // the API is meant.
    @OptIn(ExperimentalAbiValidation::class)
    abiValidation()
    jvm()
    android {
        namespace = "com.mmcdole.kivo.core"
        compileSdk = 37
        minSdk = 26
    }
    val kivoCore = XCFramework("KivoCore")
    listOf(iosArm64(), iosSimulatorArm64(), tvosArm64(), tvosSimulatorArm64()).forEach { target ->
        target.binaries.framework {
            baseName = "KivoCore"
            // Linked into the app, so there's no framework to embed and sign.
            isStatic = true
            export(project(":api"))
            binaryOption("bundleId", "com.mmcdole.kivo.core")
            kivoCore.add(this)
        }
    }

    sourceSets {
        commonMain {
            kotlin.srcDir(displayTables)
            dependencies {
                api(project(":api"))
                api("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.11.0")
                implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")
                implementation("io.ktor:ktor-client-websockets:$ktorVersion")
                api("androidx.datastore:datastore-preferences-core:1.2.1")
                implementation("app.cash.sqldelight:coroutines-extensions:$sqldelightVersion")
            }
        }
        androidMain.dependencies {
            implementation("io.ktor:ktor-client-okhttp:$ktorVersion")
        }
        appleMain.dependencies {
            implementation("io.ktor:ktor-client-darwin:$ktorVersion")
            implementation("app.cash.sqldelight:native-driver:$sqldelightVersion")
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
            implementation("io.ktor:ktor-client-mock:$ktorVersion")
        }
        // The tests drive virtual time with runCurrent and advanceTimeBy.
        matching { it.name.endsWith("Test") }.configureEach {
            languageSettings.optIn("kotlinx.coroutines.ExperimentalCoroutinesApi")
        }
        jvmTest.dependencies {
            implementation("app.cash.sqldelight:sqlite-driver:$sqldelightVersion")
            implementation("io.ktor:ktor-client-okhttp:$ktorVersion")
            implementation("io.ktor:ktor-server-cio:$ktorVersion")
            implementation("io.ktor:ktor-server-websockets:$ktorVersion")
        }
    }
}

skie {
    analytics {
        enabled.set(false)
    }
}

sqldelight {
    databases {
        create("DownloadDatabase") {
            packageName.set("com.mmcdole.kivo.core.downloads.db")
            // For ALTER TABLE … DROP COLUMN in a migration (SQLite 3.35: iOS 15).
            dialect("app.cash.sqldelight:sqlite-3-35-dialect:$sqldelightVersion")
        }
    }
}

// The download store is the core's own: its generated database, queries and
// rows are made internal, so they stay out of the apps' API.
tasks.withType<app.cash.sqldelight.gradle.SqlDelightTask>().configureEach {
    val generated = outputDirectory
    doLast {
        generated.get().asFileTree.matching { include("**/*.kt") }.forEach { file ->
            file.writeText(file.readText().replace(Regex("(?m)^public "), "internal "))
        }
    }
}
