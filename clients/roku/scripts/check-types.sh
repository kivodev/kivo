#!/bin/sh
# Type-checks the app against the generated API interfaces with
# BrighterScript 1 (BSC1, by default the bsc1 dev dependency), which checks
# interface types where the 0.x compiler that builds the app doesn't. The
# app must have no errors, and typecheck/bad.bs must fail on its misspelt
# field, so the check is known to read the types.
set -eu
cd "$(dirname "$0")/.."
bsc1=${BSC1:-node_modules/.bin/bsc1}

out=$("$bsc1" --project bsconfig.typecheck.json 2>&1) || { printf '%s\n' "$out"; echo "check:types: the app has type errors"; exit 1; }

if bad=$("$bsc1" --project bsconfig.typecheck-bad.json 2>&1); then
    echo "check:types: typecheck/bad.bs compiled; the check no longer reads the types"; exit 1
fi
printf '%s\n' "$bad" | sed 's/\x1b\[[0-9;]*m//g' | grep -q "typecheck/bad.bs:6:17 - error cannot-find-name: Cannot find name 'titel'" ||
    { printf '%s\n' "$bad"; echo "check:types: typecheck/bad.bs failed, but not on its misspelt field"; exit 1; }
echo "check:types: ok"
