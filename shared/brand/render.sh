#!/bin/sh
# Renders every raster icon from app-icon.svg, the one source for the app
# icon on each platform but the TVs, whose icons are drawn from the mark. Android draws its icons from vector drawables made
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

# The Apple TV app draws the same logos, and its icon is two layers, the
# mark at 72% of the icon's height over the background, as the system lifts
# them apart under focus; its Top Shelf images, shown when there's nothing
# to continue, are the lockup on the background.
tv="$root/clients/apple/KivoTV/Resources/Assets.xcassets"
for set in LaunchMark Logo TMDBLogo; do cp "$assets/$set.imageset/"*.svg "$tv/$set.imageset/"; done
brand="$tv/App Icon & Top Shelf Image.brandassets"
# <svg> <png> <width> <height> <the art's height>
on_canvas() {
	rsvg-convert -h "$5" "$1" | magick - -background none -gravity center -extent "$3x$4" -define png:exclude-chunks=date,time "$2"
}
# <png> <width> <height>
ground() {
	magick -size "$2x$3" xc:'#0d0e12' -alpha off -define png:exclude-chunks=date,time "$1"
}
icon="$brand/App Icon.imagestack"
store="$brand/App Icon - App Store.imagestack"
on_canvas mark.svg "$icon/Front.imagestacklayer/Content.imageset/front.png" 400 240 173
on_canvas mark.svg "$icon/Front.imagestacklayer/Content.imageset/front@2x.png" 800 480 346
on_canvas mark.svg "$store/Front.imagestacklayer/Content.imageset/front-store.png" 1280 768 553
ground "$icon/Back.imagestacklayer/Content.imageset/back.png" 400 240
ground "$icon/Back.imagestacklayer/Content.imageset/back@2x.png" 800 480
ground "$store/Back.imagestacklayer/Content.imageset/back-store.png" 1280 768
for shelf in "Top Shelf Image:shelf:1920" "Top Shelf Image Wide:shelf-wide:2320"; do
	set="${shelf%%:*}" rest="${shelf#*:}"
	name="${rest%%:*}" width="${rest#*:}"
	for scale in 1 2; do
		suffix=$([ "$scale" = 2 ] && echo @2x || true)
		out="$brand/$set.imageset/$name$suffix.png"
		on_canvas lockup.svg "$out" $((width * scale)) $((720 * scale)) $((122 * scale))
		magick "$out" -background '#0d0e12' -flatten -alpha off -define png:exclude-chunks=date,time "$out"
	done
done
