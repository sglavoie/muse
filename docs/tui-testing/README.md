# TUI Layout Test Harness

`muse tui-test` is a hidden, fixture-backed harness for exercising the real
Bubble Tea TUI (`internal/tui.Model`) in process. It drives the actual model —
resizes, key presses, text input, and a safe subset of palette commands — then
renders a normalized view of the terminal and reports layout problems as JSON.

It is intended for agents and CI scripts that need to verify TUI layout without
a real terminal. It is **hidden**: `muse tui-test` never appears in
`muse --help`.

## Minimum supported size

Muse supports normal TUI layout at `60x15` and larger. When either terminal
dimension is smaller, the root view renders a "terminal window is too small"
message instead of normal views or overlays. Committed scenarios that assert
normal layout should stay at or above `60x15`; below-minimum sizes are only for
testing that guard message.

## Safety guarantees

The harness never touches real state. Fixtures are built entirely from in-memory
fakes:

- No real config file is read or written.
- No database is opened; all rows come from in-memory fakes.
- No music files are read; no `mpv` is launched.
- No Finder, clipboard, or external process is invoked.

Direct `command` actions are restricted to a fixture-safe allowlist (internal
navigation and UI/state toggles). Any file, Finder, clipboard, external-process,
or config-saving command is rejected with a structured action error, so a
scenario cannot accidentally trigger a real side effect.

## Fixtures

`muse tui-test fixtures` lists the built-in fixtures:

| Fixture    | Purpose |
| ---------- | ------- |
| `empty`    | A model with no library/queue/playlist data — bare layout. |
| `rich`     | Deterministic data across every primary view: library, queue, playlists, favorites, tags, and recommendation (Mixes) data. The default. |
| `narrow`   | Long titles, artist/album/path names, and wide Unicode (CJK, diacritics, emoji) to stress truncation and width arithmetic. |
| `playback` | An active playback session: current track, queue, repeat/shuffle/volume/status metadata. |

## CLI

```
muse tui-test fixtures [--json]
muse tui-test run --scenario PATH [--fixture NAME] [--size WxH] [--json] [--observations] [--ansi] [--drain-timeout DUR]
muse tui-test session [--fixture NAME] [--size WxH] [--ansi] [--commands]
```

Exit codes:

- `0` — pass.
- `1` — a scenario action or assertion failed.
- `2` — invalid CLI input or invalid scenario input (bad flags, unreadable
  scenario, malformed scenario JSON, unknown fixture, malformed size).

### `fixtures`

Lists the available fixture names, one per line, or as `{"fixtures": [...]}` with
`--json`.

### `run`

Runs a scenario file and reports the result. Flags:

- `--scenario PATH` (required) — the scenario JSON file.
- `--fixture NAME` — override the scenario's `fixture`.
- `--size WxH` — override the scenario's initial `size`.
- `--json` — emit the full `UITestScenarioResult` as JSON instead of a summary.
- `--observations` — collect a per-action observation trail into the result.
- `--ansi` — include the raw ANSI-styled screen in observations.
- `--drain-timeout DUR` — per-command drain timeout (e.g. `50ms`).

Without `--json`, `run` prints a one-line `PASS`/`FAIL` summary to stdout.

### `session`

Runs an interactive newline-delimited JSON (NDJSON) loop: one JSON **request**
per stdin line, one JSON **response** per stdout line. Stdout stays pure NDJSON;
human diagnostics go to stderr. Request types:

- `{"type": "observe"}` — render and return the current observation.
- `{"type": "action", "action": {…}}` — apply one action (see below) and return
  the resulting observation, or `ok: false` with an `error`.
- `{"type": "reset"}` — rebuild a fresh fixture-backed driver and return its
  observation. The replaced driver is released first, so any pending async
  futures it still held are discarded; `pending_async` after a reset reflects
  only the new driver.
- `{"type": "quit"}` — acknowledge and exit.

Each response is `{"ok": bool, "type": "…", "observation": {…}, "error": "…"}`.

Flags:

- `--fixture NAME` — fixture to drive (defaults to `rich`).
- `--size WxH` — initial terminal size (defaults to `100x30`).
- `--ansi` — include the raw ANSI-styled screen in observations.
- `--commands` — include enabled palette command IDs as `available_commands`
  in observations. Omitted by default so NDJSON clients opt in explicitly.

## Scenario format

A scenario is a JSON object decoded strictly (unknown fields are rejected):

```json
{
  "name": "example",
  "fixture": "rich",
  "size": "100x30",
  "actions": [
    {"key": "2"},
    {"assert": {"view": "Playlists"}},
    {"assert": {"no_layout_issues": true}, "label": "playlists-fits"}
  ]
}
```

- `name` (required) — scenario name.
- `fixture` (optional) — fixture name; defaults to `rich`.
- `size` (optional) — initial terminal size `WxH`; defaults to `100x30`.
- `actions` (required) — at least one action, run in order. The run stops at the
  first failing action.

### Actions

Each action sets **exactly one** operation field, plus an optional `label` used
in error messages:

| Field | Meaning |
| ----- | ------- |
| `key` | Dispatch one key press. Supports printable runes (`"a"`, `"/"`, `"?"`), named keys (`"enter"`, `"esc"`, `"tab"`, `"space"`, arrows, `"home"`/`"end"`/`"pgup"`/`"pgdown"`), and modifier combos (`"ctrl+g"`, `"shift+tab"`). View switches: `1`–`7`; `]`/`[` cycle views. |
| `text` | Type a literal string, one printable key press per rune (e.g. into the command palette or a filter). |
| `resize` | Apply a new terminal size `WxH`. |
| `wait` | Drain pending async commands, optionally for a duration string (e.g. `"100ms"`). |
| `command` | Invoke an allowlisted palette command directly, matched by command ID or case-insensitive name (e.g. `"Go to Mixes"`). |
| `assert` | Evaluate one assertion (below). |

### Assertions

An `assert` object sets exactly one expectation:

| Field | Passes when |
| ----- | ----------- |
| `contains` | The plain screen contains the substring. |
| `not_contains` | The plain screen omits the substring. |
| `view` | The active **primary** view matches (case-insensitive): `Library`, `Playlists`, `Queue`, `Browse`, `Favorites`, `Tags`, `Mixes`. |
| `overlay` | The active overlay matches (case-insensitive), e.g. `help`, `command_palette`, `queue_summary`, `debug`, `now_playing`. There is no "no overlay" assertion; assert a `view` instead. |
| `status_contains` | The status line contains the substring. |
| `no_layout_issues` | The observation reports zero layout issues. Must be `true`. |

## Observation fields

Each observation is normalized JSON:

- `current_view` — active overlay name if one is showing, else the primary view.
- `primary_view` — the primary view name, ignoring overlays.
- `overlay` — the active overlay name (empty when none).
- `width`, `height` — the simulated terminal size.
- `plain_screen` — the rendered screen with ANSI escapes stripped.
- `ansi_screen` — the raw ANSI-styled screen (only with `--ansi`).
- `status` — the ANSI-stripped status bar.
- `layout_issues` — detected layout problems (see below); always present.
- `pending_async` — count of parked async command futures.
- `snapshot` — a compact digest of model state (view, overlay, cursor, focused
  row, queue length, etc.).
- `available_commands` — enabled palette command IDs (session mode only, opt-in
  via `--commands`; omitted otherwise).

## Layout issue kinds

| Kind | Meaning |
| ---- | ------- |
| `invalid_utf8` | Rendered content is not valid UTF-8. |
| `width_overflow` | A line is wider than the terminal (`actual` columns > `limit`). |
| `height_overflow` | More rendered lines than the terminal height (`actual` > `limit`). |
| `tab_character` | A tab appears in the output (breaks column-width arithmetic). |
| `blank_screen` | The screen is empty after stripping ANSI. |

## Committed scenarios

`docs/tui-testing/scenarios/` holds ready-to-run examples:

| Scenario | Fixture | Notes |
| -------- | ------- | ----- |
| `baseline-layout.json` | `rich` | Library renders and fits at 100×30. |
| `all-primary-views.json` | `rich` | Cycles all seven primary views; each fits. |
| `overlays.json` | `rich` | Help, queue-summary, debug, and favorite-target overlays fit at 100×30. |
| `command-palette-search.json` | `rich` | Opens the palette (at 120×50) and filters by typing. |
| `layout-regressions-rich.json` | `rich` | Guards command palette height, Queue columns, Mixes descriptions, and queue-summary short-height layout. |
| `layout-regressions-empty.json` | `empty` | Guards minimum-size empty-state and idle status-bar layout. |
| `metadata-editor-narrow.json` | `narrow` | Guards metadata-editor height with long values at the minimum supported size. |
| `resize-stress.json` | `rich` | Cycles many terminal sizes. |
| `narrow-long-text.json` | `narrow` | Long text and wide Unicode at the minimum supported size. |

The baseline, primary-view, overlay, command-palette, layout-regression, and
narrow/long-text scenarios are stable and gated in `go test`.
`resize-stress.json` is kept for manual bug discovery across a wider range of
supported terminal sizes.

### Example

```
$ muse tui-test run --scenario docs/tui-testing/scenarios/baseline-layout.json
PASS "baseline-layout" (fixture rich, 100x30, 3 actions)

$ muse tui-test run --scenario docs/tui-testing/scenarios/narrow-long-text.json --json
{ "ok": false, … "error": { "reason": "expected no layout issues, found 1: …" } }
```
