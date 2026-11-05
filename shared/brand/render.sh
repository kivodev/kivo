#!/bin/sh
# Renders every raster icon from app-icon.svg, the one source for the app
# icon on each platform but the TV. Android draws its icons from vector drawables made
# from the same mark (clients/android/src/main/res/drawable). Needs
# rsvg-convert (librsvg) and ImageMagick: brew install librsvg imagemagick.
set -eu
cd "$(dirname "$0")"
root=../..

render() {
	rsvg-convert -w "$1" app-icon.svg -o "$2"
}

render 512 "$root/server/web/public/icon-512.png"
render 192 "$root/server/web/public/icon-192.png"
render 180 "$root/server/web/public/apple-touch-icon.png"
cp app-icon.svg "$root/server/web/public/icon.svg"
# The Windows tray and installer icon.
magick "$root/server/web/public/icon-512.png" -define icon:auto-resize=256,64,48,32,24,16 "$root/server/cmd/kivo/kivo.ico"
# The web client's logo, served beside the icons (Logo.tsx).
for f in lockup lockup-light mark mark-light; do cp "$f.svg" "$root/server/web/public/$f.svg"; done
# TV launchers put the icon on a card of their own, often wider than it is
# tall, so the TV's icon is the mark alone on a clear ground (tv-icon.svg).
rsvg-convert -w 512 tv-icon.svg -o "$root/clients/tizen/icon.png"

# iOS draws its logos from vectors in the asset catalog: the sign-in lockup
# for each appearance, and the launch screen's mark at 144 points, centred
# on the mark's artwork (x 10.5 to 54.5).
assets="$root/clients/apple/Kivo/Resources/Assets.xcassets"
cp lockup-light.svg "$assets/Logo.imageset/Logo-light.svg"
cp lockup.svg "$assets/Logo.imageset/Logo-dark.svg"
sed 's|viewBox="0 0 64 64"|width="144" height="144" viewBox="0.5 0 64 64"|' mark.svg >"$assets/LaunchMark.imageset/LaunchMark.svg"

# The App Store refuses an app icon with an alpha channel.
ios="$root/clients/apple/Kivo/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon.png"
render 1024 "$ios"
magick "$ios" -alpha off -define png:exclude-chunks=date,time "$ios"
