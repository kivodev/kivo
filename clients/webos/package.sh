#!/usr/bin/env bash
# Packages the LG TV test app into build/com.kivo.player_<version>_all.ipk.
# It makes server/web's packaged TV build without Samsung's webapis script
# (npm run build:packaged:webos, dist-packaged) and copies it into www/
# beside appinfo.json and the icons, or copies the build at the path given
# instead:
#
#   bash package.sh                                  # build, then package
#   bash package.sh ../../server/web/dist-packaged   # package an existing build
#
# It needs LG's CLI: npm install -g @webos-tools/cli.
set -euo pipefail

if [ $# -gt 0 ]; then
  dist="$(cd "$1" && pwd)"
fi
cd "$(dirname "$0")"
if [ -z "${dist:-}" ]; then
  (cd ../../server/web && npm run build:packaged:webos)
  dist=../../server/web/dist-packaged
fi
if [ ! -f "$dist/index.html" ]; then
  echo "package.sh: no index.html in $dist" >&2
  exit 1
fi

# webOS opens appinfo.json's main from the package's root, so the build and
# appinfo.json share a folder.
rm -rf www
mkdir www
cp -R "$dist"/. www/
cp appinfo.json icon.png largeIcon.png www/
# Samsung's build asks for its webapis script, which an LG TV doesn't have.
if grep -q 'WEBAPIS' www/index.html; then
  echo "package.sh: www/index.html is the Samsung build; build with npm run build:packaged:webos" >&2
  exit 1
fi
find www -name '*.map' -delete

mkdir -p build
rm -f build/*.ipk
# The build is minified already; ares-package's own pass is left out so the
# package is the build as tested.
ares-package --no-minify -o build www
