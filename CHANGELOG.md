# Changelog

All notable changes to muse are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Eight more audio formats in the library scanner: `.ac3`, `.au`, `.caf`,
  `.dts`, `.m4r`, `.mp4`, `.snd`, and `.w64`. Note that `.mp4` files are indexed
  and played as audio only; exclude them with `excluded_paths` if your music
  directory also holds video.
- ffprobe as a full metadata backend. muse picks a backend at startup —
  ExifTool when it is installed and working, ffprobe otherwise — so a machine
  with FFmpeg but no Perl can run muse. `muse check` prints which backend a
  scan would use, and the fallback is reported as a warning rather than a
  fatal error.
- The track-info overlay (`i`) reads through the active backend too, so it
  shows the raw tags ffprobe returns when ffprobe is the one scanning. On that
  backend it fills in rows ExifTool leaves blank for several containers —
  album artist, bitrate, channels, and sample rate — and reports disc numbers
  and Ogg/Opus durations that ExifTool does not return at all.
- A welcome overlay with setup instructions when no usable music directory is
  configured, so a first run on a machine without `~/Music` lands in a working
  (if empty) UI instead of an error.
- Panic diagnostics for background work. A panic in a background goroutine is
  now recovered and logged with its stack instead of killing the process, and
  a panic inside the TUI leaves a record in the log file rather than only on
  a terminal that has already been restored.
- Startup now reaps an mpv left running by a previous crash. Removing the
  stale IPC socket was never enough on its own: mpv runs with `--idle=yes` and
  survives its parent, so the orphan kept holding an audio device — and kept
  playing — invisibly.

### Changed

- ExifTool is no longer a hard startup requirement; it is the recommended
  backend because it scans a batch in one process (about 2.4x faster than
  ffprobe on a real library), but either tool is sufficient. Startup fails
  only when neither is available. A broken ExifTool install — present on
  `PATH` but failing `exiftool -ver`, typically a missing Perl module — is
  treated the same as an absent one and falls back to ffprobe.
- A missing, unreadable, or empty music directory is no longer fatal. muse
  starts, surfaces the problem as a warning with the configuration hint, and
  keeps the remaining directories in the scan.
- The macOS notification group identifier is now `muse.now-playing`.

### Fixed

- A music directory that has been unmounted, renamed, or deleted no longer
  causes every track under it to be marked missing. The scan skips the
  unreachable root and reports no deletions instead of treating an unwalked
  tree as an empty one.

## [0.1.0]

Initial public release. muse had been developed privately before this point, so
this entry describes the feature set as published rather than a diff against an
earlier release.

### Added

- Terminal music player for a local library: mpv over a Unix IPC socket for
  playback, a SQLite index built from ExifTool metadata, and a Bubble Tea TUI.
- Seven primary views — Library, Playlists, Queue, Browse, Favorites, Tags, and
  Mixes — plus an Insights dashboard, a command palette, and a searchable
  keybinding reference.
- Playlists: static plus smart playlists driven by an AND/OR rule editor.
- Playback features: gapless playback via mpv prefetch, a 10-band equalizer
  with seven built-in presets and user-defined custom presets, stereo widening,
  and EBU R128 volume normalization.
- Radio mode (Auto-DJ) that keeps the queue fed by scoring candidates against
  recent listening history, seedable from a mix or an Insights row.
- Recommendation mixes: Rediscover, Hidden Gems, On Repeat, and Deep Cuts, with
  configurable thresholds.
- Library management: an in-app metadata editor that writes to muse's database
  and never modifies your audio files, TSV metadata export/import for bulk
  cleanup, free-form tags with merge and undo, bulk and visual-line selection,
  and favorites for tracks, albums, and artists.
- Insights: listening sessions, momentum, discovery, and a data-health panel
  that surfaces colliding genre spellings, blank fields, and unverified
  durations.
- CLI subcommands: `muse check` for config and prerequisite validation,
  `muse now-playing`, and the remote-control commands `toggle-play`, `next`,
  `previous`, `volume-up`, and `volume-down`, relayed to a running instance
  over a Unix control socket.
- Desktop notifications on track change: `alerter` on macOS and `notify-send`
  on Linux, both optional and reported as warnings by `muse check` when absent.
- First-class Linux support alongside macOS, XDG-correct config, data, state,
  and runtime paths with macOS fallbacks, and an annotated
  [`config.example.toml`](config.example.toml).
- An optional ffprobe-backed background audit that corrects track durations
  metadata got wrong.
- `muse tui-test`, a hidden fixture-backed snapshot harness for the real Bubble
  Tea model, documented in [`docs/tui-testing/README.md`](docs/tui-testing/README.md).
- `muse --version` now falls back to the version embedded by
  `go install`, reporting the module version or the VCS revision instead of
  `dev` when no `-ldflags` value was stamped in.
- Project documentation: README, CONTRIBUTING, and an MIT license.

[Unreleased]: https://github.com/sglavoie/muse/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/sglavoie/muse/releases/tag/v0.1.0
