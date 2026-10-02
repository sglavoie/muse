#!/usr/bin/env sh
#
# Build muse.app: generate macos/muse.xcodeproj (generate-project.sh), build it,
# copy the product to macos/build/muse.app, bundle muse, and sign (sign.sh).
#
# Usage: build-app.sh [Debug|Release]   (default Debug)
#
# Environment:
#   MUSE_BINARY             muse to bundle at Contents/Resources/bin/muse;
#                           required for Release (`just mac-app Release` builds
#                           it with the version and build time stamped in),
#                           optional for Debug
#   MUSE_MACOS_CACHE        cache root (default macos/.cache, gitignored)

set -eu

require_tool() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required tool not found: $1" >&2
		exit 1
	fi
}

script_dir=$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)
macos_dir=$(dirname -- "$script_dir")

configuration="${1:-Debug}"
case "$configuration" in
Debug | Release) ;;
*)
	echo "configuration must be Debug or Release, not $configuration" >&2
	exit 1
	;;
esac

muse_binary="${MUSE_BINARY:-}"
if [ "$configuration" = Release ] && [ -z "$muse_binary" ]; then
	echo "a Release build bundles muse: set MUSE_BINARY, or run \`just mac-app Release\`" >&2
	exit 1
fi
if [ -n "$muse_binary" ] && [ ! -x "$muse_binary" ]; then
	echo "MUSE_BINARY is not an executable file: $muse_binary" >&2
	exit 1
fi

"$script_dir/generate-project.sh"
require_tool ditto

# --- Build -----------------------------------------------------------------------

derived_data="$macos_dir/DerivedData"
xcodebuild build \
	-project "$macos_dir/muse.xcodeproj" \
	-scheme muse \
	-configuration "$configuration" \
	-destination "platform=macOS,arch=$(uname -m)" \
	-derivedDataPath "$derived_data" \
	-quiet

product="$derived_data/Build/Products/$configuration/muse.app"
if [ ! -f "$product/Contents/Info.plist" ]; then
	echo "xcodebuild reported success but $product is missing" >&2
	exit 1
fi

out="$macos_dir/build/muse.app"
mkdir -p "$macos_dir/build"
rm -rf "$out"
ditto "$product" "$out"

# --- Bundle muse and sign --------------------------------------------------------
#
# Copying muse in breaks the seal xcodebuild wrote, so the bundle is always
# re-signed afterwards, inside out.

if [ -n "$muse_binary" ]; then
	mkdir -p "$out/Contents/Resources/bin"
	cp "$muse_binary" "$out/Contents/Resources/bin/muse"
	chmod 755 "$out/Contents/Resources/bin/muse"
fi
"$script_dir/sign.sh" "$out"

echo "Built $out ($configuration)"
