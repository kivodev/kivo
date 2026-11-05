// Plugin versions for every module, pinned once here.
plugins {
    kotlin("multiplatform") version "2.4.0" apply false
    kotlin("plugin.serialization") version "2.4.0" apply false
    kotlin("plugin.compose") version "2.4.0" apply false
    kotlin("jvm") version "2.4.0" apply false
    id("com.android.kotlin.multiplatform.library") version "9.4.1" apply false
    id("com.android.application") version "9.4.1" apply false
    id("com.android.test") version "9.4.1" apply false
    id("co.touchlab.skie") version "0.10.15" apply false
    id("app.cash.sqldelight") version "2.4.1" apply false
}
