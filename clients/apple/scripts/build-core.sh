#!/bin/bash
# Builds what the Swift code compiles against, with Gradle: KivoUI's tokens
# (KivoUI/Generated/Tokens.swift) and the Kotlin core as
# Frameworks/KivoCore.xcframework, with its libraries' licenses beside it
# (KivoCore-licenses.json), Debug or Release as Xcode's
# CONFIGURATION says (Debug when run by hand). Xcode runs it from the
# KivoCore aggregate target before every build; Gradle skips what's
# current, so it costs seconds unless the core changed.
set -euo pipefail

apple="$(cd "$(dirname "$0")/.." && pwd)"
clients="$(dirname "$apple")"
configuration="${CONFIGURATION:-Debug}"
case "$configuration" in
    Release) variant=Release ;;
    *) variant=Debug ;;
esac

# Xcode runs scripts without the login shell's environment: find JDK 21.
if [ -z "${JAVA_HOME:-}" ]; then
    if JAVA_HOME="$(/usr/libexec/java_home -v 21 2>/dev/null)"; then
        :
    elif [ -d /opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home ]; then
        JAVA_HOME=/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home
    else
        echo "error: JDK 21 not found; install it as FIRST-RUN.md says, or set JAVA_HOME" >&2
        exit 1
    fi
fi
export JAVA_HOME

cd "$clients"
./gradlew --console=plain :tokens:swiftTokens ":core:assembleKivoCore${variant}XCFramework" :core:exportLibraryDefinitions

lower="$(echo "$variant" | tr '[:upper:]' '[:lower:]')"
framework="$apple/Frameworks/KivoCore.xcframework"
mkdir -p "$apple/Frameworks"
changed="$(/usr/bin/rsync -a --delete --itemize-changes "$clients/core/build/XCFrameworks/$lower/KivoCore.xcframework/" "$framework/")"
# Xcode decides whether to process the xcframework again, and whether its
# module cache is current, by timestamps, and rsync carries Gradle's over,
# so Xcode can go on compiling against the old module after the core's API
# changes ("AppleServer has no member …"). So when anything changed, all
# of the framework takes the current time.
if [ -n "$changed" ]; then
    find "$framework" -exec touch {} +
fi

# The core's libraries and their licenses, for Open source licenses
# (Kivo/Settings/LicensesView.swift), which the app bundles.
cp "$clients/core/build/aboutlibraries/ios.json" "$apple/Frameworks/KivoCore-licenses.json"
