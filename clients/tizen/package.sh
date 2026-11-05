#!/usr/bin/env bash
# Packages the Samsung TV app into build/kivo-tizen.wgt, unsigned. It makes
# server/web's packaged TV build (npm run build:packaged, dist-packaged) and
# copies it into www/, or copies the build at the path given instead:
#
#   bash package.sh                                  # build, then package
#   bash package.sh ../../server/web/dist-packaged   # package an existing build
#
# A TV only installs a package signed with a Samsung certificate; see README.md.
set -euo pipefail

if [ $# -gt 0 ]; then
  dist="$(cd "$1" && pwd)"
fi
cd "$(dirname "$0")"
if [ -z "${dist:-}" ]; then
  (cd ../../server/web && npm run build:packaged)
  dist=../../server/web/dist-packaged
fi
if [ ! -f "$dist/index.html" ]; then
  echo "package.sh: no index.html in $dist" >&2
  exit 1
fi

rm -rf www
mkdir www
cp -R "$dist"/. www/

# The store refuses a page that loads code from elsewhere or relaxes the
# CSP, so the page is checked before it's packaged: a CSP meta, no
# unsafe-inline or unsafe-eval, and no script or stylesheet from another
# origin.
page=www/index.html
if ! grep -q 'http-equiv="Content-Security-Policy"' "$page"; then
  echo "package.sh: $page has no Content-Security-Policy meta" >&2
  exit 1
fi
if grep -Eq "'unsafe-inline'|'unsafe-eval'" "$page"; then
  echo "package.sh: $page's CSP allows unsafe-inline or unsafe-eval" >&2
  exit 1
fi
if grep -Eiq '<(script|link)[^>]+(src|href)="(https?:)?//' "$page"; then
  echo "package.sh: $page loads a script or stylesheet from another origin" >&2
  exit 1
fi

# Tizen applies config.xml's policy as well as the page's, and its default
# would block the player's blob: media, so config.xml carries the same one.
meta="$(sed -n 's/.*http-equiv="Content-Security-Policy" content="\([^"]*\)".*/\1/p' "$page")"
widget="$(sed -n 's/.*<tizen:content-security-policy>\(.*\)<\/tizen:content-security-policy>.*/\1/p' config.xml)"
if [ "$meta" != "$widget" ]; then
  echo "package.sh: config.xml's content-security-policy isn't the page's" >&2
  exit 1
fi

mkdir -p build
rm -f build/kivo-tizen.wgt
zip -q -X -r build/kivo-tizen.wgt config.xml icon.png www -x '*.DS_Store' '*.map'
echo "build/kivo-tizen.wgt"
