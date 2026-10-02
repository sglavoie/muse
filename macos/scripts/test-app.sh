#!/usr/bin/env sh
#
# Run the MuseAppTests XCTest suite: generate macos/muse.xcodeproj
# (generate-project.sh), then `xcodebuild test`. The tests are hosted in the
# app, so they build it too.
#
# Environment:
#   MUSE_MACOS_CACHE        cache root (default macos/.cache, gitignored)

set -eu

script_dir=$(CDPATH="" cd -- "$(dirname -- "$0")" && pwd)
macos_dir=$(dirname -- "$script_dir")

"$script_dir/generate-project.sh"

xcodebuild test \
	-project "$macos_dir/muse.xcodeproj" \
	-scheme muse \
	-destination "platform=macOS,arch=$(uname -m)" \
	-derivedDataPath "$macos_dir/DerivedData" \
	-quiet
