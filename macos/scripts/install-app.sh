#!/usr/bin/env sh
#
# Install macos/build/muse.app (built by `just mac-app Release`) into
# /Applications, replacing an earlier copy, then checks the installed
# signature and registers it with Launch Services so Finder and Spotlight find
# it. A running muse.app is left alone: replacing a bundle under a live process
# would end its muse session and stop the music, so quit it first.
#
# Environment:
#   MUSE_APP_INSTALL_DIR    destination directory (default /Applications)

set -eu

require_tool() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "required tool not found: $1" >&2
		exit 1
	fi
}

script_dir=$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)
macos_dir=$(dirname -- "$script_dir")

require_tool ditto
require_tool codesign

app="$macos_dir/build/muse.app"
if [ ! -x "$app/Contents/Resources/bin/muse" ]; then
	echo "$app has no bundled muse; build it with \`just mac-app Release\`" >&2
	exit 1
fi
codesign --verify --strict "$app"

install_dir="${MUSE_APP_INSTALL_DIR:-/Applications}"
dest="$install_dir/muse.app"
if pgrep -f "^$dest/Contents/MacOS/muse" >/dev/null 2>&1; then
	echo "$dest is running; quit it (menu bar icon > Quit muse) and rerun" >&2
	exit 1
fi

mkdir -p "$install_dir"
rm -rf "$dest"
ditto "$app" "$dest"
codesign --verify --strict --verbose=1 "$dest"
lsregister=/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister
if [ -x "$lsregister" ]; then
	"$lsregister" -f "$dest"
fi
echo "Installed $dest"
