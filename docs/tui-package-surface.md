# TUI Package Exported Surface

Audit of the exported identifiers in the packages extracted from `internal/tui`
during the TUI decomposition refactor. Numbers below were produced by a
type-checked pass over the whole module (`golang.org/x/tools/go/packages` with
`Tests: true`), counting `types.Info.Uses` references per consuming package.

**Counting rules used throughout:**

- An *importer* is a package other than the declaring package. A package's own
  tests (same directory, including the `_test` external variant) do not count as
  importers — otherwise a test could keep dead surface alive forever.
- `exp` counts every exported identifier: types, funcs, methods, struct fields,
  consts and vars.

## Layering

`go list -deps ./internal/tui/...` confirms the graph is acyclic and every edge
points downward, with two intentional same-layer edges:

- `queue → statusbar` — only for `statusbar.TransientNotifyMsg`, the message the
  queue returns when a reorder is refused. `statusbar` imports `playback`,
  `styles` and `text` only, so the edge is one-way.
- `favorites → overlay` and `tags → overlay` — both need `overlay.Box`/`overlay.Row`
  to draw their pickers. `overlay` imports `keys`, `playback`, `styles`, `text`
  and never imports back.

`internal/tui/uitest` is the only `tui/*` package that imports `internal/tui`;
no non-test file in `internal/tui` references a `uitest` symbol, so the
dependency stays one-way.

## Per-package summary

| Package | exp | multi-consumer | single-consumer | zero-consumer | importers |
|---|---:|---:|---:|---:|---|
| `internal/desktop` | 6 | 0 | 6 | 0 | `internal/tui` |
| `tui/sorting` | 50 | 25 | 25 | 0 | `tui`, `library`, `queue`, `browse`, `favorites`, `tags`, `playlists`, `insights` |
| `tui/keys` | 151 | 41 | 108 | 2 | `tui` + 11 view packages |
| `tui/playback` | 57 | 38 | 18 | 1 | `tui`, `main`, `overlay`, `statusbar`, `queue`, `internal/app` |
| `tui/overlay` | 96 | 29 | 66 | 1 | `tui`, `metadata`, `settings`, `smartrules`, `favorites`, `uitest`, `tags` |
| `tui/statusbar` | 50 | 1 | 48 | 1 | `tui`, `queue`, `uitest` |
| `tui/settings` | 31 | 2 | 27 | 2 | `tui`, `uitest` |
| `tui/smartrules` | 13 | 0 | 12 | 1 | `tui` |
| `tui/metadata` | 48 | 0 | 46 | 2 | `tui` |
| `tui/queue` | 66 | 14 | 51 | 1 | `tui`, `internal/app` |
| `tui/browse` | 85 | 0 | 84 | 1 | `tui` |
| `tui/insights` | 134 | 0 | 129 | 5 | `tui` |
| `tui/mixes` | 17 | 0 | 16 | 1 | `tui` |
| `tui/uitest` | 72 | 0 | 35 | 37 | `main` |

## Why single-consumer packages stay exported

Most view packages have exactly one importer, `internal/tui`. That is the
architecture, not an accident: `internal/tui` holds the root `Model` and is the
only place that owns view state, routes keys and issues `tea.Cmd`s. A view
package exports what the root model must reach — its view struct, its state
fields, its `HandleAction`, its `Render` and its messages — and nothing else.
Unexporting that surface would mean folding the view back into `internal/tui`,
which is the opposite of the refactor.

Three packages have a different single consumer, worth naming explicitly:

- `internal/desktop` → `internal/tui` only. It wraps the macOS notification and
  "reveal in Finder" shells; the root model is the only caller.
- `tui/uitest` → `main` only. It is consumed by `main_tuitest.go`, which
  implements the hidden `muse tui-test` command. The exported surface is the
  harness API plus the JSON wire contract.
- `tui/queue` and `tui/playback` → imported directly by `internal/app` (and
  `tui/playback` by `main` as well) alongside `internal/tui`. `internal/tui`
  re-exports nothing on their behalf; `internal/app` and `main` name the leaf
  packages themselves.

## Why the zero-consumer exports stay exported

Every remaining exported identifier with no external consumer falls into one of
three categories. There are no unjustified zero-consumer exports left in the
packages listed above.

**1. Named in an exported signature (API).** The identifier is the type of an
exported parameter, result or struct field, so a caller cannot use the exported
API without naming it.

| Identifier | Reached through |
|---|---|
| `keys.HintMode` | parameter of `keys.RenderFooterHints` |
| `keys.RegistryKey` | key type of `keys.Registry.Bindings` |
| `playback.ModeIndicator` | element type of `playback.ModeIndicators`' result |
| `overlay.GoToKind` | `GoToOption.Kind` and `GoToPickedMsg.Kind` |
| `statusbar.NotifPriority` | parameter of `StatusBar.PushMsg` |
| `settings.Tab`, `settings.Action` | `SettingsOverlay.Tab` field and `SettingsOverlay.HandleKey` result |
| `smartrules.Action`, `metadata.Action` | `SmartRuleEditor.HandleKey` / `Editor.HandleKey` results |
| `metadata.Field` | type of the exported const `metadata.FieldDuration` |
| `insights.Panel` | `View.Panel` field and `PanelSpec.id` |
| `insights.PanelClass` | `PanelSpec.Class` field |
| `insights.PanelSpec` | result of `View.Spec` |
| `insights.Window` | element type of the exported var `insights.Windows` |
| `insights.DrillKind` | `Row.Drill` field |
| `uitest.UITestActionError` | `UITestScenarioResult.Error` |
| `uitest.UITestAssertion` | `UITestAction.Assert` |
| `uitest.UITestIssue`, `UITestIssueKind` | `UITestObservation.LayoutIssues` and `UITestIssue.Kind` |

**2. Interface satisfaction (IFACE).** `queue.View.SetSize`, `browse.BrowseView.SetSize`
and `mixes.MixesView.SetSize` are never called by name. They implement the
unexported `resizable` interface in `internal/tui/root_layout.go`, which
`Model.SetSize` ranges over to resize all eight primary views at once.
Unexporting any of them silently drops that view out of the resize loop.

**3. JSON wire contract (TAG).** The 33 zero-consumer struct fields in
`tui/uitest` all carry `json:` tags. They are the `muse tui-test` scenario input
format and the observation output format consumed by the JSON scenarios under
`docs/tui-testing/scenarios/`. `encoding/json` reaches them by reflection, so a
static reference count of zero is expected and means nothing. The same applies
to `tui.UITestSnapshot` in `internal/tui/uitest_snapshot.go`.

## Known remaining surface (out of scope here)

The packages that pre-date this refactor (`tui/library`, `tui/playlists`,
`tui/tags`, `tui/favorites`, `tui/palette`, `tui/text`, and `internal/tui`
itself) still carry exported identifiers with no external consumer and no
justification above. These were exported before the refactor started, so they
are reported rather than changed:

| Package | exported | unjustified zero-consumer |
|---|---:|---:|
| `tui/library` | 105 | 47 |
| `tui/playlists` | 118 | 53 |
| `tui/tags` | 136 | 43 |
| `tui/favorites` | 111 | 42 |
| `tui/palette` | 36 | 6 |
| `tui/text` | 21 | 4 |
| `internal/tui` | 127 | 26 |

The largest single cluster is `library.Panel` (47 of the `tui/library` figure).
`Panel` is the pre-refactor `library.View` renamed; the new `library.View`
wrapper absorbed every former caller, so `Panel`'s fields and cursor/filter
methods are now only used inside `tui/library`. Only `Panel().FocusedTrack()`
still has outside callers. Unexporting the rest is a safe follow-up but was not
part of this pass.
