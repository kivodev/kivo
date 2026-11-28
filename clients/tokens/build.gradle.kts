// :tokens, the theme generators: shared/tokens.json to Compose, SwiftUI and the Roku app.
// :android:tv and :android:mobile run its main at build time, and the Xcode build runs swiftTokens
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

// The Roku app has no Gradle build, so its tokens are committed:
// `./gradlew :tokens:rokuTokens` after changing shared/tokens.json; a test
// fails while the file is stale.
tasks.register<JavaExec>("rokuTokens") {
    group = "build"
    description = "Writes the Roku app's tokens from shared/tokens.json."
    val input = rootDir.resolve("../shared/tokens.json")
    val output = rootDir.resolve("roku/source/generated/tokens.bs")
    inputs.file(input)
    outputs.file(output)
    classpath = sourceSets.main.get().runtimeClasspath
    mainClass.set("com.mmcdole.kivo.tokens.MainKt")
    args("roku", input.path, output.path)
}
