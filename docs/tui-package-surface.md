# TUI Package Exported Surface

Audit of the exported identifiers in the packages extracted from `internal/tui`
during the TUI decomposition refactor. Every number and every row below is
produced by a type-checked pass over the whole module
(`golang.org/x/tools/go/packages` with `Tests: true`), counting
`types.Info.Uses` references per consuming package. That pass is committed as
`TestExportedSurfaceInventoryCurrent` in `internal/tui/surface_inventory_test.go`
and re-runs on every `go test ./internal/tui`, so the audit fails rather than
rots when the code moves on. See [Per-identifier inventory](#per-identifier-inventory).

**Counting rules used throughout:**

- An *importer* is a package other than the declaring package. A package's own
  tests (same directory, including the `_test` external variant) do not count as
  importers — otherwise a test could keep dead surface alive forever.
- `exp` counts every exported identifier: types, funcs, methods, struct fields,
  consts and vars.

## Layering

Every package in the TUI graph carries a level, and the level fixes the
permitted direction of every import: **a package may import only packages at a
strictly lower level.** Nothing else is allowed — not a reference back up, and
not a reference sideways between two packages sharing a level.

The levels are not drawn by hand. A package that imports no other package in
the graph is L0; every other package sits one level above the highest package
it imports. An assignment like that exists precisely when the graph is acyclic,
which makes the rule above true by construction for the built graph and turns
the level column into a fact about the code rather than an intention about it.

Scope is `main`, `internal/app`, `internal/desktop`, `internal/tui` and every
`internal/tui/*` package. The rest of the module (`internal/library`,
`internal/db`, `internal/player`, `internal/config`, `internal/insights`,
`internal/recommendation`, `internal/metaexport`, `internal/radio`) sits
underneath all of it and never imports back — a checked property, not an
assumption. Three further rules are enforced with the levels:

- **Test files obey the levels too.** The toolchain rejects an import cycle
  between packages but accepts one introduced by an external test package
  (`package foo_test`), so `_test.go` imports are checked against the same
  ordering rather than exempted from it.
- **Only `tui/uitest` may import `internal/tui`.** `internal/tui` owns the root
  `Model` and imports the view packages; a view package reaching back would
  invert the architecture. `tui/uitest` is the documented exception — it drives
  the root model as a harness, and no non-test file in `internal/tui`
  references a `uitest` symbol.
- **The graph is closed.** No package outside the list below imports a package
  inside it, so the importer sets recorded in this document are complete.

This supersedes the level sketch in the refactor plan, which grouped packages by
intent before the code existed. Three imports the plan drew as connecting two
packages of equal rank — `queue → statusbar`, `favorites → overlay` and
`tags → overlay` — are ordinary descending imports under the assignment
measured from the code: `tui/queue`, `tui/favorites` and `tui/tags` are L4,
`tui/statusbar` and `tui/overlay` are L3.

The table below is generated from the direct imports `go list` reports, by
`TestPackageLayersMatchImportGraph` in
`internal/tui/styles/import_layering_test.go`. That test fails when a package
changes level, gains or loses an import, appears, disappears, or breaks one of
the rules above. `TestBuildNoImportCycles` in the same file is the standing
import-cycle check: `go list` refuses to report a graph containing a cycle, so
loading one is the proof for the built graph, and the test then walks the graph
again with test imports included, where the compiler alone is not proof. Both
live in the lowest package of the graph as an external test that imports
nothing from the module, so the guard cannot add an edge to the graph it
measures. Regenerate the table with:

```sh
MUSE_UPDATE_TUI_LAYERING=1 go test ./internal/tui/styles -run TestPackageLayersMatchImportGraph
```

<!-- BEGIN GENERATED: package layers -->

**33 packages, 147 direct imports between them, 8 levels (L0–L7).** Levels are read off the graph, not assigned by hand: a package that imports nothing else in the table is L0, and every other package sits one level above the highest package it imports.

| Level | Packages |
|---:|---|
| L7 | `main` |
| L6 | `internal/app`, `tui/uitest` |
| L5 | `internal/tui` |
| L4 | `tui/autodj`, `tui/browse`, `tui/favorites`, `tui/metadata`, `tui/palette`, `tui/queue`, `tui/settings`, `tui/smartrules`, `tui/tags` |
| L3 | `tui/albums`, `tui/genres`, `tui/insights`, `tui/mixes`, `tui/overlay`, `tui/playlists`, `tui/statusbar` |
| L2 | `tui/artists`, `tui/library`, `tui/playback` |
| L1 | `tui/chart`, `tui/keys`, `tui/sorting`, `tui/strip` |
| L0 | `internal/desktop`, `tui/buckets`, `tui/filter`, `tui/layout`, `tui/styles`, `tui/text` |

| Package | Level | Direct imports within the graph |
|---|---:|---|
| `main` | L7 | `internal/app` (L6), `tui/uitest` (L6), `internal/tui` (L5), `tui/playback` (L2) |
| `internal/app` | L6 | `internal/tui` (L5), `tui/queue` (L4), `tui/playback` (L2) |
| `tui/uitest` | L6 | `internal/tui` (L5), `tui/palette` (L4) |
| `internal/tui` | L5 | `tui/autodj` (L4), `tui/browse` (L4), `tui/favorites` (L4), `tui/metadata` (L4), `tui/palette` (L4), `tui/queue` (L4), `tui/settings` (L4), `tui/smartrules` (L4), `tui/tags` (L4), `tui/albums` (L3), `tui/genres` (L3), `tui/insights` (L3), `tui/mixes` (L3), `tui/overlay` (L3), `tui/playlists` (L3), `tui/statusbar` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/playback` (L2), `tui/chart` (L1), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `internal/desktop` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/autodj` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/browse` | L4 | `tui/albums` (L3), `tui/genres` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/filter` (L0), `tui/styles` (L0) |
| `tui/favorites` | L4 | `tui/overlay` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/metadata` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/palette` | L4 | `tui/overlay` (L3), `tui/filter` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/queue` | L4 | `tui/playlists` (L3), `tui/statusbar` (L3), `tui/playback` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/buckets` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/settings` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/smartrules` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/tags` | L4 | `tui/overlay` (L3), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/albums` | L3 | `tui/artists` (L2), `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/genres` | L3 | `tui/artists` (L2), `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/insights` | L3 | `tui/library` (L2), `tui/chart` (L1), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/filter` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/mixes` | L3 | `tui/library` (L2), `tui/keys` (L1), `tui/strip` (L1), `tui/styles` (L0), `tui/text` (L0) |
| `tui/overlay` | L3 | `tui/playback` (L2), `tui/keys` (L1), `tui/styles` (L0), `tui/text` (L0) |
| `tui/playlists` | L3 | `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/statusbar` | L3 | `tui/playback` (L2), `tui/styles` (L0), `tui/text` (L0) |
| `tui/artists` | L2 | `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/library` | L2 | `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/buckets` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/playback` | L2 | `tui/keys` (L1) |
| `tui/chart` | L1 | `tui/styles` (L0) |
| `tui/keys` | L1 | `tui/styles` (L0) |
| `tui/sorting` | L1 | `tui/buckets` (L0) |
| `tui/strip` | L1 | `tui/styles` (L0), `tui/text` (L0) |
| `internal/desktop` | L0 | — |
| `tui/buckets` | L0 | — |
| `tui/filter` | L0 | — |
| `tui/layout` | L0 | — |
| `tui/styles` | L0 | — |
| `tui/text` | L0 | — |

<!-- END GENERATED: package layers -->

## Per-package summary

| Package | exp | multi-consumer | single-consumer | zero-consumer | importers |
|---|---:|---:|---:|---:|---|
| `internal/desktop` | 10 | 0 | 10 | 0 | `internal/tui` |
| `tui/sorting` | 50 | 26 | 24 | 0 | `tui`, `library`, `queue`, `browse`, `favorites`, `tags`, `playlists`, `insights` |
| `tui/keys` | 163 | 43 | 119 | 1 | `tui` + 11 view packages |
| `tui/playback` | 62 | 41 | 20 | 1 | `tui`, `main`, `overlay`, `statusbar`, `queue`, `internal/app` |
| `tui/overlay` | 150 | 38 | 111 | 1 | `tui`, `metadata`, `autodj`, `settings`, `smartrules`, `favorites`, `uitest`, `tags`, `palette` |
| `tui/statusbar` | 69 | 1 | 65 | 3 | `tui`, `queue`, `uitest` |
| `tui/settings` | 34 | 2 | 30 | 2 | `tui`, `uitest` |
| `tui/smartrules` | 17 | 0 | 16 | 1 | `tui` |
| `tui/metadata` | 55 | 0 | 53 | 2 | `tui` |
| `tui/queue` | 80 | 18 | 62 | 0 | `tui`, `internal/app` |
| `tui/browse` | 90 | 0 | 89 | 1 | `tui` |
| `tui/insights` | 140 | 0 | 136 | 4 | `tui` |
| `tui/mixes` | 17 | 0 | 16 | 1 | `tui` |
| `tui/autodj` | 100 | 0 | 96 | 4 | `tui` |
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
| `autodj.Action` | result of `TunePanel.HandleKey` |
| `autodj.Tone` | `StatusLine.Tone` field |
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

## Per-identifier inventory

Every exported identifier of the fifteen packages in the summary above, with
the packages outside its own directory that reference it. The pre-existing
packages are not inventoried here; they are reported in the next section.
Each package's table lives in its own file under
[`tui-package-surface/`](tui-package-surface/), linked from the list at the end
of this section.

The tables are generated and verified by `TestExportedSurfaceInventoryCurrent`
(`internal/tui/surface_inventory_test.go`), which type-checks the whole module
with `golang.org/x/tools/go/packages` (`Tests: true`) and rebuilds every row
from `types.Info.Uses`. The test fails when a declaration is added, removed or
renamed, when an identifier gains or loses a consumer, and when the per-package
summary above no longer matches the code. Regenerate it with:

```sh
MUSE_UPDATE_SURFACE_INVENTORY=1 go test ./internal/tui -run TestExportedSurfaceInventoryCurrent
```

**Importers** lists every package outside the declaring directory that
references the identifier; `†` marks a consumer that only reaches it from
`_test.go` files. **Disposition** records why the identifier is exported:

| Disposition | Meaning |
|---|---|
| `multi` | Two or more importers. |
| `root` | Only `internal/tui` reaches it. The root `Model` owns view state, routes keys and issues every `tea.Cmd`; unexporting means folding the view back into `internal/tui`. |
| `app` | Only `internal/app` reaches it — the startup/shutdown layer that owns the player and the queue outside the TUI. |
| `harness` | Only `main` reaches it, through `main_tuitest.go` and the hidden `muse tui-test` command. |
| `peer` | Exactly one importer, and it is none of the three above; the Importers column names it (today always a sibling `tui/*` package). |
| `api` | No direct reference: it is named in the signature, field or const type of an identifier that *is* referenced, so a caller cannot use the package without naming it. |
| `iface` | No direct reference: it implements the named interface, which is what dispatches to it. Unexporting it drops the receiver out of that interface silently. |
| `json` | No direct reference: `encoding/json` reaches the field by reflection. These are the `muse tui-test` scenario input and observation output contract. |

A disposition suffixed `(tests only)` means every importer reaches the
identifier from `_test.go` files alone: it is exported for a test that lives
outside the declaring package, not for production code. Those identifiers are
the honest candidates for a later unexport-and-move-the-test pass; they are
recorded rather than changed here because moving a test is a behavioural change,
not an audit.

`api`, `iface` and `json` are the only justifications the test accepts for an
export with no importer at all. Anything else fails the test, so a zero-consumer
export must either earn a justification, be recorded in
`surfaceManualJustifications` with a reason, or be unexported.

<!-- BEGIN GENERATED: per-identifier inventory -->

**1109 exported identifiers across 15 packages**: 169 reached from two or more packages, 882 from exactly one, 58 from none (23 `api`, 2 `iface`, 33 `json`). 76 of the reached identifiers are named only by `_test.go` files outside their own package.

- [`internal/desktop`](tui-package-surface/internal-desktop.md)
- [`tui/sorting`](tui-package-surface/tui-sorting.md)
- [`tui/keys`](tui-package-surface/tui-keys.md)
- [`tui/playback`](tui-package-surface/tui-playback.md)
- [`tui/overlay`](tui-package-surface/tui-overlay.md)
- [`tui/statusbar`](tui-package-surface/tui-statusbar.md)
- [`tui/settings`](tui-package-surface/tui-settings.md)
- [`tui/smartrules`](tui-package-surface/tui-smartrules.md)
- [`tui/metadata`](tui-package-surface/tui-metadata.md)
- [`tui/queue`](tui-package-surface/tui-queue.md)
- [`tui/browse`](tui-package-surface/tui-browse.md)
- [`tui/insights`](tui-package-surface/tui-insights.md)
- [`tui/mixes`](tui-package-surface/tui-mixes.md)
- [`tui/autodj`](tui-package-surface/tui-autodj.md)
- [`tui/uitest`](tui-package-surface/tui-uitest.md)

<!-- END GENERATED: per-identifier inventory -->

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
