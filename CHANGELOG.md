# Changelog

All notable changes to muse are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Saving unchanged single-track metadata preserves the previous undo action.
  Metadata edits preserve fractional durations unless the duration is edited;
  editor undo also restores the precise original duration.
- Undoing a queue removal or clear reports database failures without changing
  the visible queue or playback, and permits retry within the original undo
  window. Bulk removal undo restores all rows in one transaction.

### Added

- View switching preserves each view's sort order and focus; the existing
  “Sort by default order” palette command resets the current view.
- Queue summaries update while open and mark unknown total durations with `+`.
- Bulk metadata editing supports explicit field clearing with `ctrl+d` and
  restoring the field to unchanged with `ctrl+r`, with a clear preview and undo.
- “Copy now-playing info” copies the playing or paused song independently of
  the focused row; track-copy commands handle missing artist/title values.

- Help search matches section headings and includes all controls in a matching section.
- Seek and volume palette commands show their configured step sizes.
- Successful library scans replace progress with a completion count retained in
  Recent Messages; canceled and incomplete scans retain their distinct feedback.

- Bulk metadata fields distinguish mixed values from empty values with a muted
  “Multiple values — unchanged” placeholder.
- Command-palette search accepts synonyms such as “refresh library”, “EQ”, and
  “tune radio”, with displayed-name matches ranked first.
- Metadata export commands show the unique exportable track count and selection
  scope; the palette can reveal the previous export backup.
- Direct “Sort by” palette commands offer the current view's supported fields
  and identify the selected field and direction.
- Up/Down in view filters recalls the last 20 distinct committed queries across
  views for the current session and restores unfinished input.
- Recent Messages shows scroll position and copies its full snapshot with `y`.

- Focused command-palette toggles show their current setting in the existing
  detail line, including the resolved Light/Dark theme and repeat mode.
- Single-track enqueue confirmations distinguish “Queued next” from
  “Added to queue” while retaining the undo hint.

- Disabled command-palette entries explain how to make the selected action
  available, without increasing the dialog height.
- Library and Queue no-match states show separate recovery keys for text and
  tag filters. Empty active tag filters stay empty, including sorted and
  History library views, until cleared.
- File-manager launch and exit errors appear as notifications and in recent
  messages, with the affected path. Linux uses “Open containing folder” in
  the palette instead of “Reveal in Finder”.

- A **Show recent messages** palette command retains the last 50 action results
  and errors for the current session, with wrapping and scrolling.
- Remaining queue duration in the Queue header, using scheduled playback rows
  for shuffle and sequential insertions; unknown durations are marked with `+`.
- Metadata re-exports preserve the previous file as `metadata-export.tsv.bak`.
  If the backup fails, the working export is left untouched.

- muse.app, a standalone macOS app for muse: `just mac-install` builds a
  Release app bundling a muse stamped with its version and build time, signs
  it inside out (an Apple Development identity by default, so privacy grants
  survive reinstalls; `MUSE_CODESIGN_IDENTITY=-` for ad-hoc; Developer ID plus
  `MUSE_NOTARY_PROFILE` to notarize) and installs it to `/Applications`. It
  has its own note icon and menu-bar template icon, asks for access to music
  libraries in Documents, Desktop, Downloads, on external drives and network
  shares, and is documented in `macos/README.md` and the README's "macOS app"
  section.

- A `macos/` skeleton for a standalone muse.app ported from tt: pinned
  GhosttyKit build (`just mac-ghosttykit`), an XcodeGen project, `just mac-app`
  and `just mac-test`. The app runs `muse` from the login shell's PATH in an
  embedded Ghostty terminal with full kitty keyboard support; an optional
  `~/.config/muse-app/ghostty.conf` layers over Ghostty's defaults.

- muse.app finds muse through a `museBinaryPath` override, its bundled copy,
  the `go install` location (preferred when built later than the bundled copy)
  or the login shell's PATH, and forwards the shell's PATH, XDG directories
  and locale so a Finder launch finds mpv and the same database. Closing the
  window hides it while music keeps playing; Quit drains muse so the last
  listen is recorded, and kills its process group (mpv included) if it is
  still running after 12 s. A muse already running in a terminal is named,
  with its PID, under a Relaunch banner.

- muse.app has a menu-bar item (left click shows or hides muse; right click
  opens Show/Relaunch/Preferences/Launch at Login/Quit), a global
  `ctrl+opt+cmd+m` shortcut that needs no Accessibility permission, and a
  Preferences window (⌘,): the shortcut recorder with conflict warnings, Dock
  and menu bar or menu bar only (switched live), Launch at Login, default
  zoom, track-change notifications on/off, the on-exit behavior, the muse
  binary override, environment overrides, and the app's `ghostty.conf`.

- muse.app talks to muse over `--control`: its menu-bar menu shows the
  current track (`Title — Artist`, album) and Play/Pause, Next and Previous,
  the menu-bar tooltip names the track, and each track change posts one
  native notification (clicking it shows muse) unless muse's window is in
  front. muse's `notify_on_track_change` still decides whether to notify; the
  app's toggle can only silence it. Relaunching muse reconnects the controls
  without restarting the app.

- `muse --control PATH` serves a JSON control socket for a host app: playback
  requests, `state` events on each track or pause change, and track-change
  banners as `notify` events instead of through alerter. Hosted sessions also
  set the window title to the playing track. See "Hosting muse" in the README.

- muse paints the terminal background with its theme once it knows the
  terminal's own background, and restores it on quit.

- A hidden `muse build-info` prints `version=`, `revision=` and `time=` for
  launchers that need to compare builds; `just release` now stamps the build
  time.

- muse exits with status 3 when another instance is already running, so a
  launcher can tell that apart from any other failure. The message is
  unchanged.

- A **Toggle theme** command in the command palette switches Dark and Light,
  saving the choice while preserving the custom primary color.

- Selectable Light and Dark themes in Display settings, with automatic terminal
  detection, a customizable primary color, live preview, and saved preferences.
  Both palettes now paint the full app canvas, improve text contrast, and derive
  focus and selection highlights from the primary color.

- A steerable Auto-DJ. Radio mode is now a session you can tune while it plays,
  through a panel on `ctrl+a`:
  - **Tune** — a preset (Familiar 20%, Balanced 40%, Explore 80%, Custom for
    anything in between), the discovery target in ten-point steps, an anchor
    mode (`Anchor` keeps scoring against the track the session started from,
    `Follow` re-anchors on every track change), and include/exclude filters on
    tags, genres, and artists. Applying to a running session rebuilds the queue
    ahead of the track you are listening to without interrupting it.
  - **Preview** — the batch the current draft would add, the reason each track
    was picked, what was ruled out, and any shortage ("only 1 of 3 familiar
    tracks were available; discovery tracks filled the rest"). Read-only:
    looking at a draft changes nothing that is playing or queued.
  - **Recipes** — named settings, saved and reloaded from the database. A recipe
    stores settings only, never a queue or a seed: saving one changes nothing
    that is playing, and loading one edits the draft, which you then start. A
    recipe whose filters name a tag that has since been deleted is reported as
    unusable rather than silently widened.
- Auto-DJ rows are now marked as the DJ's own in the database. A re-tune
  replaces only the rows the DJ added: tracks you queued yourself are left
  alone, and so is the one playing. A session also survives a restart — muse
  restores the settings, the seed, and the discovery balance, and keeps filling
  from where it left off. Ownership is recorded when a row is written and is
  never inferred from its source label, so queue rows left over from an earlier
  version's radio mode are treated as manual entries after the upgrade: a
  re-tune plays around them instead of discarding them.
- `Start Auto-DJ from selection…` in the command palette, seeding a session from
  whatever the active view has selected, alongside the existing `A` starts from
  a mix or an Insights row.
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

- Clearing a text filter preserves the focused row, including duplicate queue
  entries, or restores the pre-search position when there are no matches.
- Empty Recently Added and History views explain their own state and offer
  relevant next actions.
- Queue summaries use the same remaining-time calculation as the Queue header.

- `just fmt`, `just fmt-check` and CI's formatting check skip gitignored Go
  files, so Go sources Zig fetches into `macos/.cache` never fail them.

- SIGHUP now shuts muse down cleanly, as SIGINT and SIGTERM do: closing the
  terminal saves the in-flight listen and flushes the database instead of
  killing muse outright, and mpv is no longer relaunched while muse exits.
- The command palette and Settings hold still. The palette keeps the width of
  the full command list instead of shrinking around whatever the filter leaves,
  and both overlays are anchored to the top of the screen rather than centered,
  so typing a query or switching tabs changes only how far down the box reaches.
- Radio refills are planned off the update loop. Picking the next batch used to
  score the library inline, which froze the interface on a large collection for
  as long as it took — including the keypress that turns radio off. Planning now
  runs as a background command whose result is checked against the state it was
  planned from and discarded if anything moved, so a refill can no longer publish
  a stale batch or delay a gesture.
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
- `muse volume-up` / `volume-down` help and the README describe the step as
  `volume_increment` (default 5%) rather than a fixed 5%.
- `config.example.toml` and the first-run config document the `eq_bands` range
  and band frequencies, the valid `eq_preset` keys, and the
  `stereo_wide_amount` range.
- Track lists are headed by what is open — "Mixes — <mix>", "Insights —
  <drill>", "Playlists — <playlist>" — matching Tags, instead of "Library" or
  "Tracks".
- Footer and help labels agree with what the keys do: Playlists and Tags both
  read `c create · r rename`, the selection footer says `a remove` where `a`
  removes (Playlists, Favorites), every Settings tab words and orders its hints
  the same way, and the Equalizer reads `s save preset`, which tells it apart
  from `ctrl+s`. Playlist create and rename inputs show the same `+ ` and `r `
  prompts as Tags.
- Mixes and Insights no longer bind `v` and `a`, which did nothing there.
- Dropped two redundant database indexes (migration 026), slightly reducing
  write work when refreshing recommendations and editing playlists.
- CI runs again on pull requests and pushes to `main`: a Linux build, vet,
  format and `go mod tidy` check, race-enabled tests, and the pinned
  golangci-lint.
- `just clean` also removes `coverage.out` and `coverage.html`.

### Fixed

- Esc clears a committed text filter before navigating back in Browse,
  Favorites, Tags, Mixes, and Insights, matching Library and Queue. The palette's
  Clear view filter command works wherever a committed filter is present, and
  Mixes shows the filtered badge too.
- Pausing playback keeps the queue position visible in the status bar.
- The persistent file-watcher warning names `ctrl+r` as the rescan shortcut.
- An open mix's footer advertises `e` to enqueue a track; help now correctly
  describes Enter as opening a mix.

- On-screen hints now name keys that do what they say. The empty Library card
  offers `ctrl+r` rescan instead of `s` (Stop) and a nonexistent import; the
  empty Playlists card offers `c` new instead of `n` (Next) and a nonexistent
  M3U import; the empty Favorites card says `F`, not `f`; and the file-watcher
  warning points at `ctrl+r` rather than `Shift+R` (reset position). The Mixes
  help no longer describes `x` as bulk selection — there it dismisses a
  track from future mixes — and footers no longer offer `/ filter`
  in Playlists, `enter filter-by` in Tags, or `F choose fav` in Favorites.

- Desktop notifications no longer leave a zombie process behind on every track
  change for the rest of the session.

- Notification sounds are written to a private per-user cache directory instead
  of a shared, predictable `/tmp/muse-sounds`.

- Items deleted just before quitting no longer linger: startup purges rows a
  previous run soft-deleted, so re-creating a tag with a deleted tag's name no
  longer fails with a raw "UNIQUE constraint failed", and deleted Auto-DJ
  recipes no longer accumulate forever.

- `muse check` reports a missing mpv and a missing metadata backend in the same
  run instead of stopping at mpv.

- Dropping a large folder into the library while muse runs no longer sends
  every file to a single ExifTool invocation; the watcher batches like a full
  scan does.

- Rescans no longer slow down with queue size: the per-track queue update now
  uses an index on `queue.track_hash`.

- Theme canvas colors now survive nested text-style resets in settings and
  primary views, preventing dark terminal backgrounds from leaking into Light.

- A music directory that has been unmounted, renamed, or deleted no longer
  causes every track under it to be marked missing. The scan skips the
  unreachable root and reports no deletions for it instead of treating an
  unwalked tree as an empty one. That suppression is scoped to the skipped
  directory: files deleted from the music directories that were read are still
  reconciled in the same scan, so one unmounted drive no longer freezes
  missing-file detection for the rest of the library.
- Metadata-backend detection can no longer hang startup or `muse check`: both
  version probes now carry their own deadline, so an ExifTool or ffprobe that
  resolves on `PATH` but never returns is treated as unusable instead of
  blocking on a blank terminal. The deadline is ten seconds per probe rather
  than a tight bound, because a cold or heavily loaded machine can take seconds
  just to start a process — a working ExifTool being rejected for answering
  slowly is worse than a slower startup, since it silently downgrades the
  backend or reports that none is installed. The prerequisite check now sizes
  its own budget from that deadline, so a slow ExifTool can no longer eat the
  time reserved for probing the ffprobe fallback.
- A broken ffprobe is no longer accepted as the metadata backend. ffprobe used
  to qualify on `PATH` presence alone, so an installation that could not
  actually execute — missing shared libraries, a stale symlink into an
  uninstalled FFmpeg — became the backend and scanned every track into
  filename-derived placeholder metadata with no error reported anywhere.
- A recovered panic in a background worker no longer passes for a successful
  result. A metadata worker that died mid-batch used to leave tracks with an
  empty path and no metadata in the library; each file is now recovered
  individually and falls back to filename-derived metadata. A hash worker that
  died could leave the directory walk blocked forever on a channel nobody was
  reading; the last worker to go now drains it. A panic in the library
  pipeline itself reported a clean finish while leaving the scan and watch
  streams open; it now closes them and surfaces the failure as a scan error
  and a degraded watcher status.
- A truncated scan no longer looks like a finished one. The hash stage and the
  duration audit closed their output channels identically whether every file
  had been processed or a worker had died partway, so callers read the close as
  completion; both now hand back an outcome that says which happened. The
  practical consequence was the worst case of all: a manual rescan cut short by
  a hash worker rebuilt a library index missing the files that were never
  hashed, then marked those still-present files missing. It now reports the
  truncation and leaves the deletion reconciliation alone. Panics in the
  watcher-forwarder and diff-walk goroutines are likewise recorded as pipeline
  failures — published as a scan error and a degraded watcher status, with the
  rest of the pipeline cancelled — instead of quietly closing streams behind an
  already-healthy status.
- A configuration where only some music directories are unusable no longer
  claims that no directories were found. Validation now reports which roots are
  usable, so the welcome overlay says some directories could not be read (and
  that the rest are being scanned) instead of announcing an empty library while
  the valid roots are indexing.
- A panic during shutdown no longer skips the rest of the teardown. A single
  recovery around the whole sequence meant a panicking callback ended shutdown
  at that point while still reporting a clean exit, leaving the database open,
  the lockfile held, the mpv socket on disk and the log unflushed. Each teardown
  step now recovers on its own, so every later step still runs and the log names
  the step that failed.
- Prerequisite warnings now reach the TUI. The ffprobe fallback notice and the
  unavailable-notification notice were logged at startup and then discarded, so
  a user launching straight into the player saw a slower scan or silent track
  notifications with no explanation on screen; they now appear in the status bar
  alongside the configuration warnings. `muse check` is unchanged.

- The volume can no longer go above 100%. mpv's own limit is 130%, so repeated
  volume-up presses at "100%" made playback louder than the level shown.
- One unplugged drive or missing directory in `music_directories` no longer
  turns off live library watching for every other directory; only the
  unreachable one is skipped, with a warning.
- Numeric tag values are indexed as written: a tag of `1000000` no longer shows
  as `1e+06`, and `0.10` stays `0.10`.
- The default library order ignores case, so artists, albums, and titles
  starting with a lowercase letter no longer sort after "Z".
- Importing a metadata export no longer reports untouched values as edits when
  the tag itself has surrounding quotes (such as `"Heroes"`) or stray
  whitespace.
- "Unknown Artist" no longer counts as a favorite artist: it is left out of
  Hidden Gems' top artists and the never-played mix's artist affinity, and an
  untagged radio seed no longer boosts every other untagged track.
- Smart playlist "is" / "is not" rules ignore case like "contains" does, and a
  genre rule matches one genre of a multi-genre track ("genre is Rock" now
  matches a track tagged "Rock; Pop").
- The startup duration check no longer re-probes and re-warns about the same
  unreadable files on every launch; they are checked again once the file
  changes.
- Track-change and `muse now-playing` notifications appear for titles or
  artists that start with "-" (such as "-M-"), which `notify-send` and
  `alerter` used to reject as options.
- Saving settings no longer replaces a symlinked `config.toml` (for example one
  managed by stow or chezmoi) with a regular file; the link's target is
  updated instead.
- Saving settings no longer rounds a fractional seek offset (such as 2.5) down
  to a whole second.
- An `[[eq_custom_presets]]` entry keyed `custom` is rejected with a warning
  instead of loading and then vanishing on the next settings save.
- `muse check` and `muse now-playing` print each config warning once instead
  of twice, and out-of-range decimal values are reported exactly as written
  (60.04 no longer shows as "60.0 is out of range (1-60)").
- `muse now-playing` prints why it could not reach mpv (`error: query mpv: …`)
  instead of only reporting that muse is not running.
- `muse --help` lists `-h`/`--help` and `--version` under Flags, which used to
  be empty, and prints to stdout.
- A failed copy to the clipboard names the tool and its error message instead
  of just "exit status 1", and a missing file manager when opening or
  revealing a folder is logged instead of failing silently.
- Selecting rows in Playlists or Tags switches the footer to its selection
  hints and shows the selection count, as every other view already did.
- `E` in Mixes enqueues the open mix instead of doing nothing once a mix is
  open, `/` on the mix list explains that you open a mix to filter it, and `s`
  stops playback on the mix list; it only cycles the sort field inside a mix.
- The empty Tags card suggests `c` to create a tag instead of `t`, which has
  no track to act on there.
- The footer keeps `? help` at narrow widths and drops the hints before it
  instead.
- The help overlay no longer lists the seek keys as ±5s/±30s, since both
  offsets are configurable, and `g`/`G` and `home`/`end` jump to its top and
  bottom.
- Counts of one read "1 track", "1 play", and "1 session" in Mixes and
  Insights, and the Data Health messages read "1 track carries no genre tag"
  instead of "1 tracks carry".
- The command palette only shows its "Recent & frequent" heading once a
  command has been used.

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
