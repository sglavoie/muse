# Setup and configuration

[Back to the overview](../README.md) · [User guide](user-guide.md)

## Requirements

### mpv — required

Playback engine. muse talks to it over a Unix IPC socket.

```
macOS:  brew install mpv
Debian: apt install mpv
Fedora: dnf install mpv
```

### A metadata backend — one of ExifTool or ffprobe is required

Track metadata (title, artist, album, album artist, genre, duration, track and
disc numbers) is read during library scans by an external tool. muse picks one at startup and
logs which; `muse check` reports it too. Startup only fails if neither is
usable.

Being on `$PATH` is not enough for either backend: muse runs a version probe
against each candidate and only accepts one that answers. The probe is bounded
(ten seconds each), so a wedged executable is treated as unusable instead of
hanging startup on a blank terminal. The bound is deliberately generous rather
than tight — a healthy probe answers in milliseconds, but a cold or heavily
loaded machine can take seconds just to start the process, and wrongly
rejecting a working tool is worse than a slow startup.

#### ExifTool — recommended (fastest)

The default backend. It reads a whole batch of files in one process, which
makes it about **2.4x faster than ffprobe** on a real library (measured over 300
mp3/flac files: 819 ms vs 1.94 s, roughly 2.7 ms vs 6.5 ms per file). ExifTool
requires Perl.

```
macOS:  brew install exiftool
Debian: apt install libimage-exiftool-perl
Fedora: dnf install perl-Image-ExifTool
```

If `exiftool -ver` fails after installing, the Perl runtime or a required
module is missing — on Debian/Ubuntu, `apt install libimage-exiftool-perl`
pulls in what it needs. muse treats that broken install the same as no install
and falls back to ffprobe, with a warning, rather than refusing to start.

#### ffprobe — required if ExifTool is absent

Part of FFmpeg, so no Perl runtime is involved. Used as the metadata backend
whenever ExifTool is missing or broken; scans are slower but complete, and
ffprobe reports durations and disc numbers for some formats more accurately
than ExifTool does. An ffprobe that resolves on `$PATH` but cannot actually run
(`ffprobe -version` fails) is rejected the same way a broken ExifTool is, so it
never becomes the backend and quietly scans every track into placeholder
metadata.

Independently of metadata, ffprobe also powers the background duration-audit
pass that corrects track durations metadata got wrong. Without ffprobe that
audit is simply skipped.

```
macOS:  brew install ffmpeg
Debian: apt install ffmpeg
Fedora: dnf install ffmpeg
```

### Album art — no extra dependency

Covers come from image files next to the music (`cover.jpg`, `folder.png`, …)
or from the picture embedded in each file, which muse reads with ExifTool.
Without ExifTool, the `ffmpeg` binary from the same FFmpeg package as ffprobe
extracts embedded pictures instead; with neither, only cover image files are
used. See [Album art](user-guide/playback.md#album-art) in the user guide.

### Notification backend — optional

Needed only for desktop banners (`notify_on_track_change` and
`muse now-playing`). Notifications are opt-in, so a missing backend never
blocks startup — `muse check` reports it as a warning.

**macOS** uses [alerter](https://github.com/vjeantet/alerter):

```
macOS:  brew install alerter
```

**Linux** uses `notify-send` from libnotify:

```
Debian: apt install libnotify-bin
Fedora: dnf install libnotify
Arch:   pacman -S libnotify
```

### Media controls — no extra dependency

On Linux, muse publishes itself as an MPRIS player on the D-Bus session bus
that every desktop session already runs, so media keys and media widgets work
with nothing to install; `playerctl` is handy for scripting it. A session
without a bus, such as over SSH, runs without media controls. On macOS,
media keys and Now Playing come with [muse.app](../macos/README.md).

## Install

### Homebrew

On macOS or Linux, the `sglavoie/tap` cask installs a prebuilt `muse` along
with mpv and ExifTool:

```sh
brew install --cask sglavoie/tap/muse-player
```

On macOS it installs the signed, notarized muse.app into `/Applications` and
links the `muse` inside it, so the command line and the app are one binary.
Upgrade with `brew upgrade` and remove with
`brew uninstall --cask muse-player`. The cask is `muse-player` because an
unrelated `muse` cask exists in Homebrew itself; the command is still `muse`.
Prebuilt archives are also attached to each
[GitHub release](https://github.com/sglavoie/muse/releases).

Installed muse 0.4.0 or earlier from the `sglavoie/tap/muse` cask? `brew
upgrade` mistakes it for Homebrew's own `muse` and skips it ("it is disabled
because it requires a signed download URL"). Switch to the new cask once:

```sh
brew uninstall --cask sglavoie/tap/muse
brew install --cask sglavoie/tap/muse-player
```

### go install

Building from source requires **Go 1.26+**.

```sh
go install github.com/sglavoie/muse@latest
```

`go install` puts the binary in `$(go env GOBIN)` when `GOBIN` is set, and in
`$(go env GOPATH)/bin` otherwise — usually `~/go/bin`. That directory is not on
`$PATH` by default. If `muse` comes back as "command not found", resolve the
install directory and add it:

```sh
MUSE_BIN="$(go env GOBIN)"                       # empty unless GOBIN is set
[ -n "$MUSE_BIN" ] || MUSE_BIN="$(go env GOPATH)/bin"

echo "$MUSE_BIN"                                 # where go install wrote muse
export PATH="$PATH:$MUSE_BIN"                    # add to ~/.bashrc or ~/.zshrc
```

To build from a checkout instead (a fork, or an unreleased revision):

```sh
git clone https://github.com/sglavoie/muse
cd muse
go install .
```

Then verify your configuration and prerequisites:

```sh
muse check
```

`muse check` validates the config file, checks that your music directories
exist and contain supported audio, and runs a live, time-bounded invocation of
mpv and of each metadata backend to catch broken installs. It also reports which backend
a scan would use. It prints `ok` and exits 0 when everything is in order, and
prints actionable errors to stderr and exits 1 otherwise. Advisory problems
(such as a missing notification backend, or falling back from ExifTool to
ffprobe) are printed as warnings without failing the check.

## Configuration

muse follows the XDG base directory spec, falling back to macOS conventions
when the XDG variables are unset:

| Purpose | Linux | macOS fallback |
| --- | --- | --- |
| Config | `$XDG_CONFIG_HOME/muse/config.toml` (default `~/.config/muse/config.toml`) | `~/.config/muse/config.toml` |
| Database | `$XDG_DATA_HOME/muse/muse.db` | `~/Library/Application Support/muse/muse.db` |
| Log | `$XDG_STATE_HOME/muse/muse.log` | `~/.local/state/muse/muse.log` |
| Sockets and PID lock | `$XDG_RUNTIME_DIR/muse/` | a user-specific directory under `$TMPDIR` |

Browsing context is saved in `browsing_session.json` beside `muse.db` on exit.
To reset the startup view, quit muse and remove that JSON file; the library,
queue, and playback session stay intact.

On first run muse creates the `muse` config directory with mode `0o700` and
writes a commented `config.toml` into it. The only setting you must change is
`music_directories`:

```toml
music_directories = ["/path/to/your/music"]
```

Everything else has a working default. See [`config.example.toml`](../config.example.toml)
for the full annotated set of options — excluded scan paths, default volume,
log level, mpv socket path and extra flags, seek and volume increments,
status-message and volume-feedback durations, gapless audio, EQ presets and
bands, stereo widening, volume normalization, theme and accent color, footer
hints, track-change notifications, following the now-playing track in the
Queue panel, and the recommendation-mix thresholds.

Open **Settings** (`ctrl+,`) and choose **Display** (`[` from Library) to
change the theme. Use `h`/`l` or the arrow keys on **Theme** to select **Auto**,
**Dark**, or **Light**. Auto follows the terminal background and is the default.
Both presets paint the full application canvas and provide contrasting text,
row highlights, and status colors.

On **Accent**, press `Enter` to open a grid of every accent: Blue (the
default), Teal, Green, Olive, Yellow, Sand, Orange, Brown, Red, Pink, Purple,
Grey and White, each dot drawn in its color. Move with `h`/`j`/`k`/`l` (or
`g`/`G` for the first and last cell) and the whole interface repaints with
the focused accent; `Enter` keeps it and `Esc` restores the one you started
with. On the Accent row itself, `h`/`l` also cycles accents without opening
the grid. Each accent has its own shade for Dark and Light, so it stays soft
on the dark canvas and readable on the light one; White becomes near-black ink
on Light. For a color of your own, choose **Custom** in the grid and type a
`#RGB` or `#RRGGBB` value; text accents adjust for readable contrast. Press
`r` on the Accent row, or submit an empty value, to restore Blue. With **Cover tint** on, a vivid cover's color takes over while
that track plays. `ctrl+s` saves and `Esc` cancels. While editing a color,
`Esc` discards that text edit first. For a quick switch, open the command
palette (`:`), search for **Toggle theme**, and press `Enter`. It switches
between Dark and Light, saves the choice, and keeps your accent. From Auto, it
switches to the opposite of the currently displayed theme.

The same options are available in `config.toml`:

```toml
theme = "light" # auto, dark, or light
primary_color = "teal"  # an accent name, or "#7c3aed"; empty uses blue
```

Until `music_directories` points at something readable, muse still starts — it
opens on an empty library with an overlay telling you which file to edit.
Directories that later go missing (an unmounted drive, a renamed folder) are
reported as a warning and skipped for that scan; the tracks under them stay in
the database rather than being marked missing.

If something goes wrong, the log file is the place to look. A panic in
background work — the scan pipeline, the mpv event reader, a watcher — is
recorded there with its stack and does not take the process down, and a panic
in the TUI leaves a record there too, since the terminal has already been
restored by the time you see it. muse also cleans up after a previous crash:
an mpv process orphaned by one is shut down at the next startup.

### Running against a throwaway library

Because every path in the table above comes from the XDG variables, pointing
them at a scratch directory gives you a fully isolated instance — its own
config, database, log, and socket — without touching your real setup. This is
the way to try a different `music_directories`, or to see what a new user sees:

```sh
mkdir -p /tmp/muse-sandbox/{config,data,state,run}
XDG_CONFIG_HOME=/tmp/muse-sandbox/config \
XDG_DATA_HOME=/tmp/muse-sandbox/data \
XDG_STATE_HOME=/tmp/muse-sandbox/state \
XDG_RUNTIME_DIR=/tmp/muse-sandbox/run \
  muse
```

muse writes a fresh `config.toml` under the sandbox config directory on that
first run; edit its `music_directories` to point at the test library. Two
things to keep in mind:

- The values must be absolute paths. Per the XDG spec, a relative value is
  ignored and muse falls back to the normal default — that is, your real
  config.
- `XDG_CONFIG_HOME` also redirects mpv's own config, so the sandbox instance
  will not pick up your personal mpv settings.

Setting `XDG_RUNTIME_DIR` matters when a real instance is already running: it
keeps the sandbox on a separate control socket and PID lock instead of talking
to the live process.

## Platform support

muse supports **macOS and Linux**. On macOS it also runs as a standalone
[macOS app](../macos/README.md) (macOS 14 or later, built from source).

**Windows is not supported.** This is not a packaging gap — the code depends on
Unix-only primitives:

- `syscall.Flock` for the single-instance lock (`internal/config/lockfile.go`).
- `syscall.SIGTERM` in the signal handler (`main_cli.go`).
- Unix-domain sockets for both the mpv IPC channel and the remote-control
  listener (`muse.ctrl`).

Porting would mean replacing all three with Windows equivalents. There are no
plans to do so.

