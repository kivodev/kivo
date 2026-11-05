// :api, the generated client: models and one suspend call
// per operation, over Ktor and kotlinx.serialization. Its sources are written
// by `make generate` in server/ and never edited; this build file, the
// generator's ignore list, EventStreamMessage.kt and ProblemException.kt are
// hand-written.
plugins {
    kotlin("multiplatform")
    kotlin("plugin.serialization")
}

val ktorVersion = "3.5.1"

kotlin {
    jvmToolchain(21)
    jvm()
    iosArm64()
    iosSimulatorArm64()
    tvosArm64()
    tvosSimulatorArm64()

    sourceSets {
        commonMain.dependencies {
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.11.0")
            implementation("org.jetbrains.kotlinx:kotlinx-serialization-core:1.11.0")
            api("io.ktor:ktor-client-core:$ktorVersion")
            api("io.ktor:ktor-client-content-negotiation:$ktorVersion")
            api("io.ktor:ktor-serialization-kotlinx-json:$ktorVersion")
            api("org.jetbrains.kotlinx:kotlinx-datetime:0.8.0")
        }
        commonTest.dependencies {
            implementation(kotlin("test"))
            implementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
            implementation("io.ktor:ktor-client-mock:$ktorVersion")
        }
    }
}
