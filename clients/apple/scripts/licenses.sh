#!/bin/bash
# Writes Licenses.plist into the app: each Swift package the build checked
# out (Nuke, the Google Cast SDK's package, libass-build) with its license,
# as LicensePlist reads them from the checkouts, for Open source licenses
# (Kivo/Settings/LicensesView.swift, KivoTV/Settings/LicensesView.swift).
# The Apple TV app links no Cast SDK, so its list leaves it out. Xcode runs
# it last in each app's build, before signing.
# LicensePlist's release is fetched once into build/tools and checked
# against its hash, and reads only the checkouts, with no network. A
# package without a license fails the build.
set -euo pipefail

apple="$(cd "$(dirname "$0")/.." && pwd)"
version=3.28.3
sha256=dd3ca6c08981a1e0c4980d5bc857bbd40f17b8b4c7ca89ab8923396fd910a040
tools="$apple/build/tools/license-plist-$version"
tool="$tools/license-plist"

if [ ! -x "$tool" ]; then
    mkdir -p "$tools"
    zip="$tools/portable_licenseplist.zip"
    curl -fsSL -o "$zip" "https://github.com/mono0926/LicensePlist/releases/download/$version/portable_licenseplist.zip"
    echo "$sha256  $zip" | shasum -a 256 -c -
    unzip -oq "$zip" -d "$tools"
    rm "$zip"
fi

# Xcode keeps the packages' checkouts beside Build/ in the derived data,
# for an archive as for a build.
derived="${BUILD_DIR%%/Build/*}"
out="$DERIVED_FILE_DIR/LicensePlist"
rm -rf "$out"
config=()
case "$PLATFORM_NAME" in
    appletv*)
        mkdir -p "$DERIVED_FILE_DIR"
        printf 'exclude:\n  - name: google-cast-ios-sdk\n' > "$DERIVED_FILE_DIR/LicensePlist.yml"
        config=(--config-path "$DERIVED_FILE_DIR/LicensePlist.yml")
        ;;
esac
"$tool" ${config[@]+"${config[@]}"} \
    --xcodeproj-path "$apple/Kivo.xcodeproj" \
    --sandbox-mode \
    --package-sources-path "$derived/SourcePackages" \
    --output-path "$out" \
    --prefix Licenses \
    --single-page \
    --add-version-numbers \
    --fail-if-missing-license \
    --suppress-opening-directory \
    --force \
    --silence-mode
mkdir -p "$TARGET_BUILD_DIR/$UNLOCALIZED_RESOURCES_FOLDER_PATH"
cp "$out/Licenses.plist" "$TARGET_BUILD_DIR/$UNLOCALIZED_RESOURCES_FOLDER_PATH/Licenses.plist"
