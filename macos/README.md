# muse.app

A standalone macOS app that runs `muse` in an embedded Ghostty terminal: its
own Dock icon, a menu-bar icon with the current track and Play/Pause, Next and
Previous, a global shortcut that shows or hides it, and native track-change
notifications. Closing the window hides it while the music keeps playing. It
spawns the ordinary `muse` binary on a pty; mpv, ExifTool and ffprobe still
come from your login shell's `PATH` (Homebrew). CI does not build it; the
release workflow does, and the Homebrew cask installs it (see the root
README's Install).

## Prerequisites

- macOS 14 or later, Apple silicon or Intel.
- Xcode (the full app, not only the Command Line Tools), with `xcodebuild`
  selected: `xcode-select -p` should name `Xcode.app`.
- Go and [`just`](https://github.com/casey/just), as for `muse` itself.
- `git`, `curl`, `shasum`, `tar`, `unzip`: present on a stock Mac.
- muse's own runtime dependencies (`brew install mpv exiftool`); see the root
  README's Requirements.

Nothing else is installed by hand. The first build downloads the Zig release
and XcodeGen version pinned in `GHOSTTY_VERSION` and `XCODEGEN_VERSION`, checks
their SHA-256, and caches them under `macos/.cache/` (gitignored). Building
GhosttyKit takes several minutes the first time and is skipped afterwards
while the pin and the patches are unchanged.

## Build and install

```bash
just mac-install        # Release build, bundled muse, signed, into /Applications
```

That is the whole install from a clean clone. It runs `just mac-app Release`,
which builds `muse` with `-X main.Version` (from `git describe`) and
`-X main.BuildTime` stamped in, builds the app, copies `muse` to
`muse.app/Contents/Resources/bin/muse`, and signs it (see below). Then
`scripts/install-app.sh` copies `macos/build/muse.app` to `/Applications`,
verifies the installed signature, and registers it with Launch Services so
Finder and Spotlight find it at once. It refuses while muse.app is running,
since replacing the bundle would end its session and stop the music: quit it
from the menu-bar icon first. `MUSE_APP_INSTALL_DIR` installs elsewhere.

The other recipes:

| Recipe | Does |
| --- | --- |
| `just mac-ghosttykit` | Build and vendor `Vendor/GhosttyKit.xcframework`, `Vendor/ghostty.h` and Ghostty's resources |
| `just mac-app` | Debug build into `macos/build/muse.app`, no bundled `muse` |
| `just mac-app Release` | Release build with the bundled, versioned `muse` |
| `just mac-test` | `xcodebuild test` over `MuseAppTests` (hosted in the app) |
| `just mac-install` | Release build, then install to `/Applications` |

`macos/muse.xcodeproj` is generated from `project.yml` on every build and is
gitignored: edit the spec, not the project. `MUSE_MACOS_CACHE` moves the cache
root.

### Which muse runs

The first executable of: the Preferences override (`museBinaryPath`), the
bundled `Contents/Resources/bin/muse`, the `go install` location (`$GOBIN`,
else the first `$GOPATH` entry's `bin`, else `~/go/bin`, as the login shell
reports them), then `PATH`. A Release build always bundles one, so an
installed app runs the `muse` it was built with, with one exception: a
`go install`ed muse whose hidden `muse build-info` reports a later `time=`
than the bundled copy's runs instead, since an older muse cannot read a
database the newer one has already migrated. A Debug build bundles none and
falls through to `just install`'s copy. Preferences names the binary running
and its version, and offers Choose… for the override.

An override that names no executable is an error rather than a quiet fall
back to some other `muse`.

### Environment

A Finder-launched app inherits almost no environment, so muse.app asks your
login shell, interactively (`$SHELL -l -i`), for `PATH` (where mpv, ExifTool
and ffprobe live), `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`,
`XDG_RUNTIME_DIR`, `LANG`, `LC_ALL` and every other exported `LC_*`, and
passes those to muse. The XDG directories matter: without them the app's
muse would open a different config and database than the terminal's, and take
a different instance lock, so two muses would run against two libraries. A
shell that cannot run interactively within five seconds is asked again as a
plain login shell.

Preferences lists `PATH` and the forwarded names, each showing the shell's
value in gray. Typing a value gives muse that instead, and clearing the field
returns to the shell's. They are stored together as the `museEnvironment`
dictionary default:

```bash
defaults write io.github.sglavoie.muse museEnvironment -dict XDG_DATA_HOME ~/music-db
defaults delete io.github.sglavoie.muse museEnvironment   # use the shell's for all
```

muse reads its environment when it starts, so an edit reaches the next muse:
use Relaunch muse after changing one.

### Signing and notarization

`scripts/sign.sh` signs inside out: the bundled `muse` first, with the
identifier `io.github.sglavoie.muse.cli`, then the app. By default it uses the
first Apple Development identity in your keychain (Xcode › Settings ›
Accounts creates one), picked by its SHA-1 so an expired certificate with the
same name is never chosen, else signs ad-hoc; either way the app runs on the
Mac that built it, and Gatekeeper (`spctl`) rejects it anywhere else.
`MUSE_CODESIGN_IDENTITY=-` forces ad-hoc.

The certificate matters for privacy prompts. A music library under
`~/Documents`, `~/Desktop` or `~/Downloads`, on an external drive or on a
network share is behind macOS's folder privacy (TCC), and the login-shell
probe may read startup files from those places too, so macOS asks whether
muse.app may access them; `Info.plist` carries a usage description for each
(`NSDocumentsFolderUsageDescription`, `NSDesktopFolderUsageDescription`,
`NSDownloadsFolderUsageDescription`, `NSRemovableVolumesUsageDescription`,
`NSNetworkVolumesUsageDescription`). `~/Music` needs no grant. A
certificate-signed app keeps those answers across rebuilds, because its
designated requirement names the certificate rather than the build; an ad-hoc
one is identified by its hash, so every `just mac-install` is a new app to
macOS and asks again. The first launch after switching from ad-hoc asks once
more. The answers live in System Settings › Privacy & Security › Files and
Folders.

To sign for distribution:

```bash
export MUSE_CODESIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)"
xcrun notarytool store-credentials muse-notary   # once; prompts for the Apple ID
export MUSE_NOTARY_PROFILE=muse-notary
just mac-app Release
```

With a Developer ID, both binaries are signed with the hardened runtime and a
secure timestamp, which notarization requires. With `MUSE_NOTARY_PROFILE` too,
the app is zipped, submitted with `notarytool --wait`, and the ticket stapled.
`MUSE_NOTARY_PROFILE` with an Apple Development or ad-hoc identity is refused.
No entitlements are needed: the app only spawns `muse`, which spawns `mpv`.

## Using the app

### Global shortcut

`Ctrl+Opt+Cmd+M` by default. It shows muse from any app and hides it when
muse is already in front; with no terminal (a launch error) it fronts the
error window. Change or clear it in Preferences (`Cmd+,`): click the recorder
and press the combination. A combination needs Ctrl, Opt, or Cmd unless the
key is a function key, and Preferences warns when another app or macOS itself
already owns it.

It is stored as the `museHotKey` default, lowercase and `+`-joined:

```bash
defaults write io.github.sglavoie.muse museHotKey 'ctrl+opt+cmd+m'
defaults write io.github.sglavoie.muse museHotKey ''    # off
defaults delete io.github.sglavoie.muse museHotKey      # back to the default
```

The app registers it with Carbon's `RegisterEventHotKey`, which needs no
Accessibility permission. A Karabiner rule that focused a terminal for muse
can go; `muse next` and friends in [`docs/karabiner.md`](../docs/karabiner.md)
keep working, since they reach the app's muse through `muse.ctrl`.

### Menu bar

Clicking the menu-bar icon shows or hides muse; right-click or Control-click
opens its menu. It starts with what is playing (`Title — Artist`, then the
album) or "Not playing", then Play/Pause, Next and Previous, then Show/Hide,
Relaunch muse, Preferences…, Launch at Login and Quit muse. The controls are
greyed out while the app has no connection to muse. The icon's tooltip names
the current track.

Preferences chooses whether muse appears in the Dock and menu bar or the menu
bar only (`museActivationPolicy`: `regular` or `accessory`), switched live.

### Notifications

Each track change posts one native banner from muse.app (title, artist, and
the album or "Now playing"); clicking it shows muse. None is posted while
muse's window is in front. macOS asks for permission on the first one. Two
switches apply: muse's own `notify_on_track_change` (and the palette toggle)
still decides whether a track change notifies at all, and the app's
Preferences toggle (`museNotifications`) can only silence it. alerter is never
used inside the app, so there are no double banners.

```bash
defaults write io.github.sglavoie.muse museNotifications -bool false
```

### Talking to muse

muse.app starts muse with `--control`, naming
`$TMPDIR/muse-app-<pid>.sock`, and keeps one connection to it for the
session: muse reports the playing track and pause state, and takes the menu's
Play/Pause, Next and Previous. The wire format is in the root README's
"Hosting muse". The plain-text `muse.ctrl` socket keeps working alongside it,
so `muse next` from any terminal drives the app's muse too. After Relaunch
muse the app reconnects without restarting.

### Window and quit semantics

- **Close** (`Cmd+W`, the red button): hides the window. muse and the music
  keep running; the shortcut, the menu-bar icon or the Dock icon bring it back
  to the same session.
- **Quit** (`Cmd+Q`, Quit muse in either menu): muse.app sends muse's
  process group the SIGHUP a closing terminal would, which muse treats as a
  clean shutdown, and waits up to 12 s for it to finish (its control socket
  reaching EOF), so the listen in progress is recorded and mpv is stopped. A
  muse still running after that is killed with its process group, mpv
  included.
- **muse exits cleanly** (`ctrl+c` in muse): muse.app quits, or, if
  Preferences says so, the window hides and the next show starts a fresh muse
  (`museExitBehavior`: `quit` or `relaunch`). A failed exit or a crash stays
  on screen with its status and a Relaunch button.
- **Already running**: muse allows one instance per runtime directory. If a
  muse is already running in a terminal, the app's muse exits with status 3
  and the window names the other one, with its PID, above a Relaunch button:
  quit the terminal muse, then press Relaunch.

### Terminal config

The terminal reads `~/.config/muse-app/ghostty.conf`, in Ghostty's own config
syntax, and not Ghostty.app's config. To share that one, write:

```text
config-file = ~/.config/ghostty/config
```

Preferences has Open and Reload for it. The app always overrides `command`,
`wait-after-command`, and `confirm-close-surface`, since it owns the process
muse runs in. Ghostty's standard themes ship inside the app, so
`theme = Catppuccin Mocha` works without Ghostty.app installed.

muse paints the terminal background with its theme (OSC 11) and the title bar
takes that color; the window title is `▶ Title — Artist` while playing.

### Menus and zoom

Edit has Copy, Paste, and Select All, and nothing else: muse owns every other
edit, undo included, through its own keys. View names the zoom keys (Actual
Size, Zoom In, Zoom Out), and Help › muse README opens the README on GitHub.
`Cmd+=` and `Cmd+-` zoom for the running session only; Preferences sets the
default font size (`museDefaultFontSize`, 6 to 72), which overrides the
config file's `font-size`.

## Layout

```text
project.yml                 XcodeGen spec
GHOSTTY_VERSION             pinned Ghostty tag, Zig version, Zig checksums
XCODEGEN_VERSION            pinned XcodeGen version and checksum
patches/ghostty/            patches applied to the pinned tag (see its README)
scripts/                    build-ghosttykit, generate-project, build-app,
                            test-app, sign, install-app, make-icons
MuseApp/App/                AppDelegate, menus, lifecycle
MuseApp/Ghostty/            runtime glue, config, surface view (ported, trimmed)
MuseApp/Shell/              window, status item, hotkey, preferences, muse
                            resolution, control client, notifications
MuseApp/Resources/          Info.plist, muse-launch, Assets.xcassets
MuseAppTests/               XCTest
NOTICE-ghostty              upstream MIT notices for the ported sources and themes
```

The icons (beamed eighth notes over a faint waveform) are drawn by
`scripts/make-icons.swift` and committed: rerun
`swift macos/scripts/make-icons.swift` after changing the artwork there, never
edit the PNGs by hand. `StatusIcon` is a template image, so macOS tints it for
light, dark, and highlighted menu bars.

## Port origin

Ported from tt's `macos/` tree (github.com/sglavoie/tt at
`1cff8752a2abe6e7d8acdf31d3a3fad22e57a12c`). tt's working tree was clean
there, so the signing edits that were uncommitted when this port was planned
(Apple Development signing by default, folder usage strings) are in that
commit; `scripts/sign.sh` and `scripts/install-app.sh` are unchanged in tt
since. The two trees are kept separate on purpose: fixes are ported by hand
between the repositories, so diff against tt's `macos/` when porting one.

Dropped from tt: Quick Capture and its shortcut, the Dock badge and arrival
notifications, drag and drop of file paths, and tt's `--notify`,
`capture`/`acknowledge`/`defer` control ops. Added beyond tt: the menu-bar
now-playing header and controls, and track-change notifications.

## Upgrading Ghostty

The embedding API in `ghostty.h` is not stable, so the app pins one tag and
the Swift under `MuseApp/Ghostty/` is a trimmed port of that tag's macOS app
(by way of tt's). Staying pinned is fine; upgrade deliberately, and preferably
after tt has, so its port can be diffed in:

1. **Bump the pin.** In `GHOSTTY_VERSION`, set `GHOSTTY_TAG`, set
   `ZIG_VERSION` to the tag's `build.zig.zon` `minimum_zig_version`, and copy
   both `ZIG_SHA256_*` values from `ziglang.org/download/index.json` for that
   release.
2. **Re-check the patches.** Read `patches/ghostty/README.md`: delete each patch
   the new tag already carries and refresh any that no longer apply.
3. **Rebuild.** `GHOSTTYKIT_FORCE=1 just mac-ghosttykit`, then `just mac-app`.
   Compile errors here are API churn; fix them in `MuseApp/Ghostty/`.
4. **Diff upstream against the port.** With the old and new tags checked out
   (the build leaves the new one in `macos/.cache/ghostty-<tag>/`):

   ```bash
   git -C macos/.cache/ghostty-NEW diff OLD NEW -- include/ghostty.h \
     macos/Sources/Ghostty/Surface\ View/SurfaceView_AppKit.swift \
     macos/Sources/Ghostty/Ghostty.App.swift \
     macos/Sources/Ghostty/Ghostty.Config.swift \
     macos/Sources/Ghostty/Ghostty.Input.swift
   ```

   The build clones one tag shallowly, so fetch the old tag first
   (`git -C macos/.cache/ghostty-NEW fetch --depth 1 origin tag OLD`). Carry
   every change to key, IME, mouse, clipboard, and runtime-callback handling
   into the matching file under `MuseApp/Ghostty/`; a new or renamed
   `ghostty_*` function or enum case in `ghostty.h` is where to look first.
5. **Test.** `just mac-test`, then `just mac-install`.
6. **Rerun the key checklist** in the installed app, side by side with muse in
   Ghostty.app on the same keys:
   - `Shift+Enter` replaces the queue (enqueue-replace) and is not Enter.
   - `Ctrl+,` opens Settings rather than reaching the app menu.
   - `Shift+Up`/`Shift+Down` change the sort direction, and
     `Shift+Left`/`Shift+Right` seek ±30 s.
   - `Ctrl+C` quits muse (and follows the exit preference), and is not
     swallowed as Copy.
   - `é`, `ç`, and dead-key `^` then `e` type correctly into the `/` filter.
   - Drag-select and `Cmd+C`, then `Cmd+V` into the filter, round-trip text.
   - The global shortcut toggles muse from another app; closing and reopening
     keeps the session and the music; `Cmd+Q` stops audio with no `mpv` left
     (`pgrep mpv` is empty).

## Manual checklist

After `just mac-install`, launch from Finder or Spotlight:

- The library renders. A library under `~/Music` scans without a prompt; one
  on an external drive (or under Documents, Desktop or Downloads) prompts once,
  scans after Allow, and does not prompt again after a reinstall.
- Play a track: the title bar, the menu-bar header and a native banner show
  it.
- Close the window: music continues. The shortcut brings the window back.
- The menu-bar Next and Pause work; `muse next` from a terminal also drives
  the app's muse.
- `Cmd+Q`: audio stops and `pgrep mpv` is empty. The listen appears in
  Insights on the next launch.
- With a terminal muse running, launching the app shows the "already running"
  error; after quitting the terminal muse, Relaunch works.
