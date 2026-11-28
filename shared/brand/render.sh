#!/bin/sh
# Renders every raster icon from app-icon.svg, the one source for the app
# icon on each platform but the TV. Android draws its icons from vector drawables made
# from the same mark (clients/android/app/src/main/res/drawable). Needs
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
# TMDB's logo, for the credit in Settings › About. It's TMDB's, kept as
# they publish it. Android draws it from a vector drawable of the same path
# (clients/android/tv/src/main/res/drawable/tmdb_logo.xml).
cp tmdb.svg "$root/server/web/public/tmdb.svg"
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
cp tmdb.svg "$assets/TMDBLogo.imageset/TMDBLogo.svg"

# The App Store refuses an app icon with an alpha channel.
ios="$root/clients/apple/Kivo/Resources/Assets.xcassets/AppIcon.appiconset/AppIcon.png"
render 1024 "$ios"
magick "$ios" -alpha off -define png:exclude-chunks=date,time "$ios"

# Roku's channel poster and splash are full frames: the TV icon's mark, or
# the lockup, centred on the dark theme's ground at the size Roku asks for.
roku="$root/clients/roku/images"
mkdir -p "$roku"
frame() { # width height source x y inner-width inner-height output
	inner=$(sed "s|<svg xmlns=\"http://www.w3.org/2000/svg\"|<svg x=\"$4\" y=\"$5\" width=\"$6\" height=\"$7\"|" "$3")
	printf '<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s"><rect width="100%%" height="100%%" fill="#0d0e12"/>%s</svg>' "$1" "$2" "$inner" |
		rsvg-convert -o "$8"
}
frame 540 405 tv-icon.svg 120 52 300 300 "$roku/icon-fhd.png"
frame 290 218 tv-icon.svg 64 28 162 162 "$roku/icon-hd.png"
frame 1920 1080 lockup.svg 601 440 718 200 "$roku/splash-fhd.png"
