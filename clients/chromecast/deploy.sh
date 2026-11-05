#!/usr/bin/env bash
# Publishes the receiver's static files with rsync over SSH to the host that
# serves the receiver URL. There is no build: the tests run, then the page,
# its stylesheet, its logo and src/ are copied as they are.
#
#   KIVO_CAST_DEPLOY=user@host:path bash deploy.sh             # publish
#   KIVO_CAST_DEPLOY=user@host:path bash deploy.sh --dry-run   # show what would change
#
# Arguments after the script's name go to rsync. Files on the target that the
# receiver no longer has are deleted, except .htaccess and .well-known/ (the
# host's own configuration and certificate challenges). The target and its
# credentials come from the environment and your SSH setup, never this
# repository.
set -euo pipefail

target="${KIVO_CAST_DEPLOY:-}"
if [ -z "$target" ]; then
  echo "deploy.sh: set KIVO_CAST_DEPLOY to the rsync target, such as user@host:path" >&2
  exit 1
fi

cd "$(dirname "$0")"
npm test

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
cp index.html receiver.css lockup.svg "$stage"/
cp -R src "$stage"/src

rsync -rlvz --checksum --delete --chmod=D755,F644 \
  --exclude=.htaccess --exclude=.well-known/ \
  "$@" "$stage"/ "$target"
