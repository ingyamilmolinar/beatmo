#!/usr/bin/env bash
# Runs a command on a fresh Xvfb display. The auth file is ours: xvfb-run's own
# temp directory sometimes failed to clean up in CI and replaced the command's
# exit status with 5.
set -uo pipefail
auth="$(mktemp "${TMPDIR:-/tmp}/beatmo-xvfb.XXXXXX")"
trap 'rm -f "$auth"' EXIT
xvfb-run -a -f "$auth" -s "-screen 0 1920x1080x24" "$@"
