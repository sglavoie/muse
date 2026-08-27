# muse

muse is a terminal music player for a local music library. It drives
[mpv](https://mpv.io) over a Unix IPC socket for playback, keeps a SQLite index
of your files built from [ExifTool](https://exiftool.org) metadata, and renders
everything through a [Bubble Tea](https://github.com/charmbracelet/bubbletea)
TUI: seven primary views, a command palette, an in-app metadata editor, a
listening-insights dashboard, and a radio mode that keeps picking the next
track for you. It is keyboard-driven throughout — press `?` at any time for the
full keybinding reference.

<!-- TODO: replace with a real screenshot or asciinema cast captured against a
     live library. Placeholder until the author records one. -->

> **Screenshot / cast:** _not yet captured._

## Features

**Seven primary views**, switched with `1`–`7` (plus Insights on `8`):

| View | What it does |
| --- | --- |
| **Library** (`1`) | The whole collection, with sub-modes for All / Recently Added / History. Filter with prefixes (`a:` artist, `t:` title, `l:` album, `g:` genre). |
| **Playlists** (`2`) | Static playlists plus smart playlists driven by a rule editor (AND/OR over field/operator/value rules). |
| **Queue** (`3`) | The live play queue: reorder, remove, yank/cut/paste rows, save the queue as a playlist. |
| **Browse** (`4`) | Hierarchical browsing by Artists ↔ Albums ↔ Genres, drilling down to tracks. |
| **Favorites** (`5`) | Favorited tracks, albums, and artists, grouped by category. |
| **Tags** (`6`) | Free-form user tags with create / rename / delete / merge, undo, and a per-track tag picker. |
| **Mixes** (`7`) | Generated recommendation mixes — Rediscover, Hidden Gems, On Repeat, Deep Cuts — tuned by thresholds in `config.toml`. |
| **Insights** (`8`) | Listening statistics: sessions, momentum, discovery, and a data-health panel that surfaces colliding genre spellings, blank fields, and unverified durations. |

**Playback**

- **Radio mode / Auto-DJ** (`A`) — muse keeps the queue fed by scoring
  candidate tracks against what you have been listening to, avoiding recent
  history. It can be seeded from a mix or from an Insights row.
- **Gapless playback** via mpv prefetch (`gapless_audio`).
- **10-band equalizer** with built-in presets (Flat, Bass Boost, Treble Boost,
  Rock, Jazz, Classical, Pop) and user-defined custom presets, plus stereo
  widening and EBU R128 volume normalization.
- **Desktop notifications** on track change, and a `muse now-playing` subcommand
  you can bind to a global hotkey.
- **Remote control** of a running instance from another terminal:
  `muse toggle-play`, `muse next`, `muse previous`, `muse volume-up`,
  `muse volume-down`.

**Library management**

- **Metadata editor** (`M`) — fix title, artist, album, and genre on the
  focused track. Edits are stored in muse's database, so your audio files are
  never modified. `i` shows the raw ExifTool tags for the file.
- **Metadata export / import** — round-trip the visible view's rows through a
  TSV file in the data directory, for bulk cleanup outside the TUI.
- Bulk selection (`x` / `X`) and visual line selection (`v`) in every list view.
- Undo (`u`) for destructive operations.

**Supported audio formats**: `.aac`, `.aif`, `.aifc`, `.aiff`, `.ape`, `.flac`,
`.m4a`, `.m4b`, `.mka`, `.mp2`, `.mp3`, `.mpa`, `.oga`, `.ogg`, `.opus`,
`.wav`, `.wma`, `.wv`.

## Requirements

### mpv — required

Playback engine. muse talks to it over a Unix IPC socket.

```
macOS:  brew install mpv
Debian: apt install mpv
Fedora: dnf install mpv
```

### ExifTool — required

Reads track metadata (title, artist, album, genre, duration, track and disc
numbers) during library scans, and backs the track-info overlay. ExifTool
requires Perl.

```
macOS:  brew install exiftool
Debian: apt install libimage-exiftool-perl
Fedora: dnf install perl-Image-ExifTool
```

If `exiftool -ver` fails after installing, the Perl runtime or a required
module is missing — on Debian/Ubuntu, `apt install libimage-exiftool-perl`
pulls in what it needs.

### ffprobe — optional

Part of FFmpeg. When present, muse runs a background duration-audit pass that
verifies and corrects track durations that metadata got wrong. Without it the
audit is simply skipped; everything else works.

```
macOS:  brew install ffmpeg
Debian: apt install ffmpeg
Fedora: dnf install ffmpeg
```

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

## Install

muse is distributed through `go install` only. It requires **Go 1.26+**.

```sh
go install github.com/sglavoie/muse@latest
```

`go install` puts the binary in `$(go env GOPATH)/bin` — usually `~/go/bin`,
which is not on `$PATH` by default. If `muse` comes back as "command not found",
add it:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"   # add to ~/.bashrc or ~/.zshrc
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
exist and contain supported audio, and runs a live invocation of mpv and
ExifTool to catch broken installs. It prints `ok` and exits 0 when everything
is in order, and prints actionable errors to stderr and exits 1 otherwise.
Advisory problems (such as a missing notification backend) are printed as
warnings without failing the check.

## Configuration

muse follows the XDG base directory spec, falling back to macOS conventions
when the XDG variables are unset:

| Purpose | Linux | macOS fallback |
| --- | --- | --- |
| Config | `$XDG_CONFIG_HOME/muse/config.toml` (default `~/.config/muse/config.toml`) | `~/.config/muse/config.toml` |
| Database | `$XDG_DATA_HOME/muse/muse.db` | `~/Library/Application Support/muse/muse.db` |
| Log | `$XDG_STATE_HOME/muse/muse.log` | `~/.local/state/muse/muse.log` |
| Sockets and PID lock | `$XDG_RUNTIME_DIR/muse/` | a user-specific directory under `$TMPDIR` |

On first run muse creates the `muse` config directory with mode `0o700` and
writes a commented `config.toml` into it. The only setting you must change is
`music_directories`:

```toml
music_directories = ["/path/to/your/music"]
```

Everything else has a working default. See [`config.example.toml`](config.example.toml)
for the full annotated set of options — excluded scan paths, default volume,
log level, mpv socket path and extra flags, seek and volume increments,
gapless audio, EQ presets and bands, stereo widening, volume normalization,
footer hints, track-change notifications, and the recommendation-mix
thresholds.

## Usage

Run `muse` with no arguments to launch the TUI. The subcommands are:

| Command | Description |
| --- | --- |
| `muse check` | Validate config and prerequisites, then exit |
| `muse now-playing` | Notify the current track via a desktop notification |
| `muse toggle-play` | Toggle play/pause on the running instance |
| `muse next` | Skip to the next track |
| `muse previous` | Skip to the previous track |
| `muse volume-up` | Increase volume by 5% |
| `muse volume-down` | Decrease volume by 5% |

The five playback commands are relayed to an already-running muse over a Unix
control socket, so they are convenient to bind to global hotkeys — see
[`docs/karabiner.md`](docs/karabiner.md) for a macOS Karabiner-Elements setup.

Global flags: `muse --version`, `muse --help`.

### Core keybindings

Press `?` inside muse for the complete, searchable list — the table below is
just enough to get moving.

| Key | Action |
| --- | --- |
| `?` | Show help |
| `ctrl+c` | Quit |
| `space` | Play / pause |
| `s` / `n` / `p` | Stop / next track / previous track |
| `left` / `right` | Seek ±5s (`shift` for ±30s) |
| `=` / `-` | Volume up / down |
| `1`–`8` | Switch to Library / Playlists / Queue / Browse / Favorites / Tags / Mixes / Insights |
| `tab` / `shift+tab` | Next / previous view |
| `j` `k` / `down` `up` | Move cursor |
| `ctrl+d` / `ctrl+u` | Half-page down / up |
| `g` / `G` | Go to top / bottom |
| `/` | Filter the current view |
| `enter` | Enqueue / open the focused row |
| `x` / `X` / `v` | Toggle bulk selection / clear selection / visual line mode |
| `r` / `z` | Cycle repeat mode / toggle shuffle |
| `A` | Toggle radio mode (Auto-DJ) |
| `F` | Favorite the focused track, album, or artist |
| `t` / `ctrl+t` | Manage tags / filter views by tag |
| `M` / `i` | Edit metadata / show track info |
| `ctrl+r` | Rescan library |
| `ctrl+s` | Cycle sort field (`shift+up` / `shift+down` for direction) |
| `:` | Command palette |
| `ctrl+,` | Settings |
| `u` | Undo |

muse renders normal layout at terminal sizes of **60x15 and larger**; below
that it shows a "terminal window is too small" message instead.

## Platform support

muse supports **macOS and Linux**.

**Windows is not supported.** This is not a packaging gap — the code depends on
Unix-only primitives:

- `syscall.Flock` for the single-instance lock (`internal/config/lockfile.go`).
- `syscall.SIGTERM` in the signal handler (`main_cli.go`).
- Unix-domain sockets for both the mpv IPC channel and the remote-control
  listener (`muse.ctrl`).

Porting would mean replacing all three with Windows equivalents. There are no
plans to do so.

## Architecture

![muse high-level architecture](docs/2026-03-17_high-level_lipgloss.svg)

_If the diagram does not render above, open
[`docs/2026-03-17_high-level_lipgloss.svg`](docs/2026-03-17_high-level_lipgloss.svg)
directly._

muse is a single Go binary. `main_cli.go` dispatches between the TUI, the local
subcommands (`check`, `now-playing`), and the control-socket relay. The TUI
(`internal/tui`) is a Bubble Tea model that talks to narrow interfaces rather
than concrete types, so every dependency — player, database, config, notifier —
is injected and fakeable. `internal/player` owns the mpv subprocess and its
JSON IPC; `internal/library` owns scanning and ExifTool metadata;
`internal/db` owns the SQLite schema and its migrations; `internal/radio`,
`internal/recommendation`, and `internal/insights` own the scoring behind radio
mode, Mixes, and the Insights dashboard.

## Development

muse uses [`just`](https://github.com/casey/just) as its task runner:

```sh
just              # list all recipes
just build        # build ./muse
just install      # go install .
just run          # go run .
just test         # go test -race -count=1 ./...
just test-pkg internal/tui
just lint         # go vet + golangci-lint (pinned version)
just fmt          # gofmt -w .
just fmt-check    # fail if anything needs formatting
just cover        # coverage profile + HTML report, prints the total
just tidy         # go mod tidy
just clean        # remove build artifacts
```

Install the pre-commit hook (runs `fmt-check`, `go vet`, and the linter) with:

```sh
just hooks
```

`just hooks` pins golangci-lint to the same version the `lint` recipe uses; if
you installed the hook before that pin existed, re-run `just hooks` to pick it
up.

muse has a bespoke snapshot harness for testing the real Bubble Tea model
without a terminal — see [`docs/tui-testing/README.md`](docs/tui-testing/README.md)
for the fixtures, scenario format, and CLI.

## License

MIT. See [`LICENSE`](LICENSE).
