# muse.app

A standalone macOS app that will run `muse` in an embedded Ghostty terminal,
with its own Dock icon. For now it is a skeleton: it initializes libghostty and
opens an empty window. None of it runs in CI.

Ported from tt's `macos/` tree (github.com/sglavoie/tt at
`1cff8752a2abe6e7d8acdf31d3a3fad22e57a12c`; its working tree was clean, so the
earlier uncommitted signing edits were already in that commit). Fixes are
ported by hand between the two repositories.

## Prerequisites

- macOS 14 or later.
- Xcode (the full app), with `xcodebuild` selected.
- [`just`](https://github.com/casey/just), `git`, `curl`, `shasum`, `tar`, `unzip`.

The first build downloads the Zig release and XcodeGen version pinned in
`GHOSTTY_VERSION` and `XCODEGEN_VERSION`, checks their SHA-256, and caches them
under `macos/.cache/` (gitignored). Building GhosttyKit takes several minutes
the first time and is skipped afterwards while the pin and the patches are
unchanged.

## Recipes

| Recipe | Does |
| --- | --- |
| `just mac-ghosttykit` | Build and vendor `Vendor/GhosttyKit.xcframework`, `Vendor/ghostty.h` and Ghostty's resources |
| `just mac-app` | Build `macos/build/muse.app` (Debug), ad-hoc signed |
| `just mac-app Release` | The same, Release, bundling `just build`'s muse at `Contents/Resources/bin/muse` |
| `just mac-test` | Run the `MuseAppTests` XCTest suite |

`project.yml` is the XcodeGen spec; the generated `muse.xcodeproj` is
gitignored. `MUSE_MACOS_CACHE` moves the cache root.
