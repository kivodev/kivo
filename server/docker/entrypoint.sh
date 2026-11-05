#!/bin/sh
# Runs Kivo as PUID:PGID with UMASK (the Unraid convention; 99:100 is
# Unraid's nobody:users), with the groups of any GPU device nodes so hardware
# transcoding works without running as root.
set -eu
umask "${UMASK:-022}"
mkdir -p /config /data /cache
if [ "$(id -u)" = 0 ]; then
  chown "${PUID}:${PGID}" /config /data /cache
  groups=""
  for dev in /dev/dri/* /dev/nvidia*; do
    [ -e "$dev" ] || continue
    g="$(stat -c %g "$dev")"
    case ",$groups," in *",$g,"*) ;; *) groups="${groups:+$groups,}$g" ;; esac
  done
  if [ -n "$groups" ]; then
    exec setpriv --reuid="${PUID}" --regid="${PGID}" --groups="$groups" "$@"
  fi
  exec setpriv --reuid="${PUID}" --regid="${PGID}" --clear-groups "$@"
fi
exec "$@"
