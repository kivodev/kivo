// :tokens, the theme generators: shared/tokens.json to Compose and SwiftUI.
// :android runs its main at build time, and the Xcode build runs swiftTokens
// (through clients/apple/scripts/build-core.sh), so the themes are never
// stale and nothing generated is committed.
plugins {
    kotlin("jvm")
}

kotlin {
    jvmToolchain(21)
}

dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")
    testImplementation(kotlin("test"))
}

tasks.register<JavaExec>("swiftTokens") {
    group = "build"
    description = "Writes KivoUI's SwiftUI tokens from shared/tokens.json."
    val input = rootDir.resolve("../shared/tokens.json")
    val output = rootDir.resolve("apple/KivoUI/Generated/Tokens.swift")
    inputs.file(input)
    outputs.file(output)
    classpath = sourceSets.main.get().runtimeClasspath
    mainClass.set("com.mmcdole.kivo.tokens.MainKt")
    args("swift", input.path, output.path)
}
