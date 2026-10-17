# Architecture and hosting

[Back to the overview](../README.md) · [Contributing](../CONTRIBUTING.md)

## Architecture

![muse high-level architecture](2026-03-17_high-level_lipgloss.svg)

_If the diagram does not render above, open
[`docs/2026-03-17_high-level_lipgloss.svg`](2026-03-17_high-level_lipgloss.svg)
directly._

muse is a single Go binary. `main_cli.go` dispatches between the TUI, the local
subcommands (`check`, `now-playing`), and the control-socket relay. The TUI
(`internal/tui`) is a Bubble Tea model that talks to narrow interfaces rather
than concrete types, so every dependency — player, database, config, notifier —
is injected and fakeable. `internal/player` owns the mpv subprocess and its
JSON IPC; `internal/library` owns scanning and metadata extraction (ExifTool or
ffprobe, chosen at startup);
`internal/db` owns the SQLite schema and its migrations; `internal/radio`,
`internal/recommendation`, and `internal/insights` own the scoring behind radio
mode, Mixes, and the Insights dashboard.

The Auto-DJ is split the same way. `internal/radio` is pure domain code:
settings and their presets, filters, the candidate partition, and the batch
plan, none of which know about a database or a terminal. `internal/db` owns the
transactions — a batch is written as hidden rows, then published together with
the state it advanced, so a crash can never leave a session and its queue
disagreeing, and every DJ-added row is marked as such. `internal/tui/autodj`
renders the panel and owns its keys but holds no dependencies beyond styling;
`internal/tui` does the planning off the update loop, publishes the result, and
rebuilds the queue through `internal/player` without interrupting the track
playing.

## Hosting muse

`muse --control PATH` runs the normal TUI and also serves a JSON control socket
at `PATH`, so a host app (such as a native macOS wrapper that runs muse in an
embedded terminal) can drive it and hear what it is doing. `muse.ctrl` keeps
working alongside it, so `muse next` and global hotkeys still reach the same
instance.

Hosting changes three things:

- **Notifications.** Track-change banners go to the host as `notify` events
  instead of through alerter or notify-send, and the "notifications are
  unavailable" startup warning is skipped. Whether to notify is still muse's
  decision: `notify_on_track_change` and the palette toggle apply as before.
- **Window title.** muse sets it to `▶ Title — Artist` while playing,
  `⏸ Title — Artist` while paused, and `muse` otherwise.
- **Exit status.** As in any session, status 3 means another muse already holds
  the instance lock, so a host can say so instead of reporting a crash.

The wire format is newline-delimited JSON, protocol version 1. A host writes
requests and muse answers each with a reply carrying the request's `id`:

```json
{"id":1,"op":"toggle-play"}
{"reply":1,"ok":true,"message":"toggle-play"}
```

The ops are `toggle-play`, `next`, `previous`, `volume-up`, `volume-down` and
`state`, whose reply carries a `state` object. Any other op is refused with
`ok:false`.

muse also writes events, each shaped `{"event":"<name>","<name>":{...}}`. A new
connection hears `hello` first, then the latest `state`:

```json
{"event":"hello","hello":{"pid":4242,"version":"v1.2.3","protocol":1}}
{"event":"state","state":{"playback":"playing","track":{"id":"<hash>","queueId":7,"title":"Song","artist":"Band","album":"Record","duration":183},"elapsed":12.5}}
{"event":"notify","notify":{"title":"Song","subtitle":"Band","body":"Record"}}
{"event":"quitting"}
```

- `state` is sent whenever the playback state (`playing`, `paused` or
  `stopped`) or the track changes, not on every tick; `elapsed` and `duration`
  are seconds, and `track` is absent when nothing is loaded.
- `quitting` means muse has begun shutting down and refuses new requests; the
  socket reaching EOF afterwards means shutdown finished.


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
without a terminal — see [`docs/tui-testing/README.md`](tui-testing/README.md)
for the fixtures, scenario format, and CLI.

Two suites sit outside `just test`, which runs only hermetic tests:

```sh
# Tests that need a real binary on PATH: mpv (playback continuity and gapless
# advance) and ExifTool. They skip themselves when the binary is absent.
go test -tags integration -race ./internal/player/ ./internal/library/

# Planner scale benchmarks at 1k / 10k / 100k tracks, with allocations.
go test -run '^$' -bench . -benchmem ./internal/radio
```

