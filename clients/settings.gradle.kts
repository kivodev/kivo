// The native apps. :api is the Kotlin client generated from
// server/internal/api/openapi.yaml by `make generate` in server/; :core is
// the Kotlin Multiplatform core the apps share; :android is the Android app;
// :benchmark measures the Android app on a device; :tokens writes the
// Compose theme from shared/tokens.json.
rootProject.name = "kivo"

dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
    }
}

pluginManagement {
    repositories {
        gradlePluginPortal()
        google()
        mavenCentral()
    }
}

include(":api", ":core", ":android", ":benchmark", ":tokens")
