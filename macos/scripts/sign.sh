#!/usr/bin/env sh
#
# Sign muse.app ad-hoc. An ad-hoc signature runs on this Mac only, and macOS
# treats each rebuild as a new app.
#
# Usage: sign.sh APP

set -eu

if [ "$#" -ne 1 ] || [ ! -d "$1/Contents" ]; then
	echo "usage: sign.sh APP" >&2
	exit 1
fi
app=$1

if ! command -v codesign >/dev/null 2>&1; then
	echo "required tool not found: codesign" >&2
	exit 1
fi

bundled="$app/Contents/Resources/bin/muse"
if [ -f "$bundled" ]; then
	codesign --force --sign - --identifier io.github.sglavoie.muse.cli "$bundled"
fi
codesign --force --sign - "$app"
codesign --verify --strict --verbose=1 "$app"
echo "Signed $app ad-hoc"
