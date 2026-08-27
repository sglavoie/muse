# Changelog

All notable changes to muse are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
