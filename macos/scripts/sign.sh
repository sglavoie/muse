#!/usr/bin/env sh
#
# Sign muse.app inside out: the bundled muse, then the app. By default with
# this Mac's first Apple Development identity, else ad-hoc; either runs on this
# Mac only. A certificate gives the app a designated requirement that survives
# a rebuild, so privacy grants (Documents, Desktop, Downloads, external drives,
# notifications) are asked for once rather than after every install; an ad-hoc
# signature is its cdhash, which macOS treats as a new app each build. With a
# Developer ID the signature carries the hardened runtime and a secure
# timestamp, which notarization requires, and a notarytool keychain profile
# then notarizes and staples the app.
#
# Usage: sign.sh APP
#
# Environment:
#   MUSE_CODESIGN_IDENTITY  codesign identity, e.g.
#                           "Developer ID Application: Name (TEAMID)";
#                           unset picks the first Apple Development
#                           identity, else ad-hoc; "-" forces ad-hoc
#   MUSE_NOTARY_PROFILE     `xcrun notarytool store-credentials` profile name;
#                           requires a Developer ID identity

set -eu

require_tool() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required tool not found: $1" >&2
		exit 1
	fi
}

if [ "$#" -ne 1 ] || [ ! -d "$1/Contents" ]; then
	echo "usage: sign.sh APP" >&2
	exit 1
fi
app=$1

require_tool codesign

notary_profile="${MUSE_NOTARY_PROFILE:-}"
if [ -n "${MUSE_CODESIGN_IDENTITY:-}" ]; then
	identity=$MUSE_CODESIGN_IDENTITY
	development=
else
	# The SHA-1 rather than the name, which an expired twin may share.
	identity=$(security find-identity -v -p codesigning 2>/dev/null |
		sed -n 's/^ *[0-9][0-9]*) \([0-9A-F]\{40\}\) "Apple Development: .*/\1/p' | head -n 1)
	identity=${identity:--}
	development=$identity
fi

if [ "$identity" = - ] || [ -n "$development" ]; then
	if [ -n "$notary_profile" ]; then
		echo "MUSE_NOTARY_PROFILE needs a Developer ID MUSE_CODESIGN_IDENTITY" >&2
		exit 1
	fi
	set -- --force --sign "$identity"
else
	set -- --force --sign "$identity" --options runtime --timestamp
fi

bundled="$app/Contents/Resources/bin/muse"
if [ -f "$bundled" ]; then
	codesign "$@" --identifier io.github.sglavoie.muse.cli "$bundled"
fi
codesign "$@" "$app"
codesign --verify --strict --verbose=1 "$app"

if [ "$identity" = - ]; then
	echo "Signed $app ad-hoc"
	exit 0
fi
echo "Signed $app as $identity"

if [ -z "$notary_profile" ]; then
	exit 0
fi

require_tool xcrun
require_tool ditto
archive=$(mktemp -d)/muse.zip
trap 'rm -rf "$(dirname -- "$archive")"' EXIT HUP INT TERM
ditto -c -k --keepParent "$app" "$archive"
echo "Submitting $app for notarization..."
xcrun notarytool submit "$archive" --keychain-profile "$notary_profile" --wait
xcrun stapler staple "$app"
xcrun stapler validate "$app"
echo "Notarized and stapled $app"
