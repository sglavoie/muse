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

**33 packages, 136 direct imports between them, 8 levels (L0–L7).** Levels are read off the graph, not assigned by hand: a package that imports nothing else in the table is L0, and every other package sits one level above the highest package it imports.

| Level | Packages |
|---:|---|
| L7 | `main` |
| L6 | `internal/app`, `tui/uitest` |
| L5 | `internal/tui` |
| L4 | `tui/browse`, `tui/favorites`, `tui/metadata`, `tui/palette`, `tui/queue`, `tui/settings`, `tui/smartrules`, `tui/tags` |
| L3 | `tui/albums`, `tui/genres`, `tui/insights`, `tui/mixes`, `tui/overlay`, `tui/playlists`, `tui/statusbar` |
| L2 | `tui/artists`, `tui/library`, `tui/playback` |
| L1 | `tui/autodj`, `tui/chart`, `tui/keys`, `tui/sorting`, `tui/strip` |
| L0 | `internal/desktop`, `tui/buckets`, `tui/filter`, `tui/layout`, `tui/styles`, `tui/text` |

| Package | Level | Direct imports within the graph |
|---|---:|---|
| `main` | L7 | `internal/app` (L6), `tui/uitest` (L6), `internal/tui` (L5), `tui/playback` (L2) |
| `internal/app` | L6 | `internal/tui` (L5), `tui/queue` (L4), `tui/playback` (L2) |
| `tui/uitest` | L6 | `internal/tui` (L5), `tui/palette` (L4) |
| `internal/tui` | L5 | `tui/browse` (L4), `tui/favorites` (L4), `tui/metadata` (L4), `tui/palette` (L4), `tui/queue` (L4), `tui/settings` (L4), `tui/smartrules` (L4), `tui/tags` (L4), `tui/albums` (L3), `tui/genres` (L3), `tui/insights` (L3), `tui/mixes` (L3), `tui/overlay` (L3), `tui/playlists` (L3), `tui/statusbar` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/playback` (L2), `tui/autodj` (L1), `tui/keys` (L1), `tui/sorting` (L1), `internal/desktop` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/browse` | L4 | `tui/albums` (L3), `tui/genres` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/filter` (L0), `tui/styles` (L0) |
| `tui/favorites` | L4 | `tui/overlay` (L3), `tui/artists` (L2), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/metadata` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/palette` | L4 | `tui/overlay` (L3), `tui/filter` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/queue` | L4 | `tui/playlists` (L3), `tui/statusbar` (L3), `tui/playback` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/buckets` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/settings` | L4 | `tui/overlay` (L3), `tui/styles` (L0), `tui/text` (L0) |
| `tui/smartrules` | L4 | `tui/overlay` (L3), `tui/styles` (L0) |
| `tui/tags` | L4 | `tui/overlay` (L3), `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/albums` | L3 | `tui/artists` (L2), `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/genres` | L3 | `tui/artists` (L2), `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/insights` | L3 | `tui/library` (L2), `tui/chart` (L1), `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/styles` (L0), `tui/text` (L0) |
| `tui/mixes` | L3 | `tui/library` (L2), `tui/keys` (L1), `tui/styles` (L0), `tui/text` (L0) |
| `tui/overlay` | L3 | `tui/playback` (L2), `tui/keys` (L1), `tui/styles` (L0), `tui/text` (L0) |
| `tui/playlists` | L3 | `tui/library` (L2), `tui/keys` (L1), `tui/sorting` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/statusbar` | L3 | `tui/playback` (L2), `tui/styles` (L0), `tui/text` (L0) |
| `tui/artists` | L2 | `tui/strip` (L1), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/library` | L2 | `tui/keys` (L1), `tui/sorting` (L1), `tui/strip` (L1), `tui/buckets` (L0), `tui/filter` (L0), `tui/layout` (L0), `tui/styles` (L0), `tui/text` (L0) |
| `tui/playback` | L2 | `tui/keys` (L1) |
| `tui/autodj` | L1 | `tui/styles` (L0), `tui/text` (L0) |
| `tui/chart` | L1 | `tui/styles` (L0) |
| `tui/keys` | L1 | `tui/styles` (L0) |
| `tui/sorting` | L1 | `tui/buckets` (L0) |
| `tui/strip` | L1 | `tui/styles` (L0) |
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
| `internal/desktop` | 6 | 0 | 6 | 0 | `internal/tui` |
| `tui/sorting` | 50 | 25 | 25 | 0 | `tui`, `library`, `queue`, `browse`, `favorites`, `tags`, `playlists`, `insights` |
| `tui/keys` | 152 | 41 | 109 | 2 | `tui` + 11 view packages |
| `tui/playback` | 61 | 39 | 21 | 1 | `tui`, `main`, `overlay`, `statusbar`, `queue`, `internal/app` |
| `tui/overlay` | 97 | 29 | 67 | 1 | `tui`, `metadata`, `settings`, `smartrules`, `favorites`, `uitest`, `tags`, `palette` |
| `tui/statusbar` | 50 | 1 | 48 | 1 | `tui`, `queue`, `uitest` |
| `tui/settings` | 34 | 2 | 30 | 2 | `tui`, `uitest` |
| `tui/smartrules` | 13 | 0 | 12 | 1 | `tui` |
| `tui/metadata` | 48 | 0 | 46 | 2 | `tui` |
| `tui/queue` | 76 | 16 | 59 | 1 | `tui`, `internal/app` |
| `tui/browse` | 85 | 0 | 84 | 1 | `tui` |
| `tui/insights` | 134 | 0 | 129 | 5 | `tui` |
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

The table is generated and verified by `TestExportedSurfaceInventoryCurrent`
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

**995 exported identifiers across 15 packages**: 153 reached from two or more packages, 783 from exactly one, 59 from none (23 `api`, 3 `iface`, 33 `json`). 77 of the reached identifiers are named only by `_test.go` files outside their own package.

| Package | Identifier | Kind | Importers | Disposition |
|---|---|---|---|---|
| `internal/desktop` | `ClipboardResultMsg` | type | `internal/tui` | root |
| `internal/desktop` | `ClipboardResultMsg.Err` | field | `internal/tui` | root |
| `internal/desktop` | `ClipboardResultMsg.Text` | field | `internal/tui` | root |
| `internal/desktop` | `CopyToClipboard` | func | `internal/tui` | root |
| `internal/desktop` | `OpenDir` | func | `internal/tui` | root |
| `internal/desktop` | `RevealFile` | func | `internal/tui` | root |
| `tui/sorting` | `AdjustScroll` | func | `internal/tui`, `tui/insights`, `tui/library` | multi |
| `tui/sorting` | `AlbumAlbumSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `AlbumTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `ArtistAlbumSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `ArtistArtistSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `ArtistTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `ClampCursorToCount` | func | `tui/browse`, `tui/favorites`, `tui/tags` | multi |
| `tui/sorting` | `CompareNumericZeroLast` | func | `tui/queue` | peer |
| `tui/sorting` | `CompareStringsUnknownLast` | func | `tui/queue` | peer |
| `tui/sorting` | `CycleSortField` | func | `internal/tui`, `tui/library` | multi |
| `tui/sorting` | `FavListSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `FavTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `FindPlaylistPosition` | func | `internal/tui` | root |
| `tui/sorting` | `FindStringPosition` | func | `internal/tui` | root |
| `tui/sorting` | `FindTrackHashPosition` | func | `internal/tui` | root |
| `tui/sorting` | `GenreArtistSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `GenreGenreSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `GenreTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `HistorySortFields` | var | `internal/tui` | root |
| `tui/sorting` | `LibrarySortFields` | var | `internal/tui` | root |
| `tui/sorting` | `PlaylistListSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `PlaylistTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `QueueSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `RecentlyAddedSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `SortAlbum` | const | `internal/tui†`, `tui/queue` | multi |
| `tui/sorting` | `SortArtist` | const | `internal/tui†`, `tui/queue` | multi |
| `tui/sorting` | `SortAsc` | const | `internal/tui`, `tui/library`, `tui/queue†` | multi |
| `tui/sorting` | `SortDateAdded` | const | `internal/tui`, `tui/library` | multi |
| `tui/sorting` | `SortDefault` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortDesc` | const | `internal/tui`, `tui/library` | multi |
| `tui/sorting` | `SortDir` | type | `internal/tui`, `tui/library`, `tui/queue` | multi |
| `tui/sorting` | `SortDirIndicator` | func | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortDuration` | const | `internal/tui†`, `tui/queue` | multi |
| `tui/sorting` | `SortField` | type | `internal/tui`, `tui/library`, `tui/queue` | multi |
| `tui/sorting` | `SortFieldHasBuckets` | func | `internal/tui`, `tui/library`, `tui/queue` | multi |
| `tui/sorting` | `SortFieldLabel` | func | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortLastPlayed` | const | `internal/tui`, `tui/library` | multi |
| `tui/sorting` | `SortName` | const | `internal/tui†` | root (tests only) |
| `tui/sorting` | `SortPlaylistTrackIndices` | func | `tui/playlists`, `tui/tags` | multi |
| `tui/sorting` | `SortPlaylists` | func | `tui/playlists` | peer |
| `tui/sorting` | `SortSource` | const | `internal/tui†`, `tui/queue` | multi |
| `tui/sorting` | `SortState` | type | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortState.Dir` | field | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortState.Field` | field | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/library`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/sorting` | `SortStrings` | func | `tui/browse`, `tui/favorites` | multi |
| `tui/sorting` | `SortTitle` | const | `internal/tui†`, `tui/queue` | multi |
| `tui/sorting` | `SortTrackCount` | const | `internal/tui†` | root (tests only) |
| `tui/sorting` | `SortTracks` | func | `internal/tui†`, `tui/browse`, `tui/favorites`, `tui/library` | multi |
| `tui/sorting` | `TagsTrackSortFields` | var | `internal/tui` | root |
| `tui/sorting` | `TrackBucket` | func | `internal/tui`, `tui/library` | multi |
| `tui/keys` | `Action` | type | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/overlay†`, `tui/playback`, `tui/playlists`, `tui/queue`, `tui/statusbar†`, `tui/tags` | multi |
| `tui/keys` | `ActionAlbumBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionAlbumOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionArtistBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionArtistOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionAutoDJTune` | const | `internal/tui` | root |
| `tui/keys` | `ActionBrowseModeNext` | const | `internal/tui` | root |
| `tui/keys` | `ActionBrowseModePrev` | const | `internal/tui` | root |
| `tui/keys` | `ActionBulkAction` | const | `internal/tui` | root |
| `tui/keys` | `ActionBulkSelectionToggle` | const | `internal/tui`, `tui/library`, `tui/playlists`, `tui/queue` | multi |
| `tui/keys` | `ActionClearQueue` | const | `internal/tui`, `tui/queue` | multi |
| `tui/keys` | `ActionClearSelection` | const | `internal/tui` | root |
| `tui/keys` | `ActionCommandPalette` | const | `internal/tui` | root |
| `tui/keys` | `ActionCursorDown` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionCursorUp` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionCut` | const | `internal/tui` | root |
| `tui/keys` | `ActionDebug` | const | `internal/tui` | root |
| `tui/keys` | `ActionDequeue` | const | `internal/tui`, `tui/queue` | multi |
| `tui/keys` | `ActionEditMetadata` | const | `internal/tui` | root |
| `tui/keys` | `ActionFavoritesBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionFavoritesOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionFilterBar` | const | `internal/tui` | root |
| `tui/keys` | `ActionFocusNowPlaying` | const | `internal/tui` | root |
| `tui/keys` | `ActionFullPageDown` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionFullPageUp` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionGenreBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionGenreOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionGoTo` | const | `internal/tui` | root |
| `tui/keys` | `ActionGoToBottom` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionGoToTop` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionHalfPageDown` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionHalfPageUp` | const | `internal/tui`, `tui/browse`, `tui/favorites`, `tui/insights`, `tui/library`, `tui/mixes`, `tui/playlists`, `tui/queue`, `tui/tags` | multi |
| `tui/keys` | `ActionHelp` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsAntiRut` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsEnqueueAll` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsJumpBrowse` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsMergeGenre` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsPanelNext` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsPanelPrev` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsRadioSeed` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsSavePlaylist` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsWindowNext` | const | `internal/tui` | root |
| `tui/keys` | `ActionInsightsWindowPrev` | const | `internal/tui` | root |
| `tui/keys` | `ActionLibraryEnqueue` | const | `internal/tui` | root |
| `tui/keys` | `ActionLibraryEnqueueReplace` | const | `internal/tui` | root |
| `tui/keys` | `ActionLibraryFilterClear` | const | `internal/tui` | root |
| `tui/keys` | `ActionLibraryModeNext` | const | `internal/tui` | root |
| `tui/keys` | `ActionLibraryModePrev` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixDismiss` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixEnqueueAll` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixRadioSeed` | const | `internal/tui` | root |
| `tui/keys` | `ActionMixUndismiss` | const | `internal/tui` | root |
| `tui/keys` | `ActionModesLegend` | const | `internal/tui` | root |
| `tui/keys` | `ActionMoveTrackDown` | const | `internal/tui`, `tui/queue` | multi |
| `tui/keys` | `ActionMoveTrackUp` | const | `internal/tui`, `tui/queue` | multi |
| `tui/keys` | `ActionNext` | const | `internal/tui`, `tui/playback` | multi |
| `tui/keys` | `ActionNextView` | const | `internal/tui` | root |
| `tui/keys` | `ActionNone` | const | `internal/tui`, `tui/overlay†`, `tui/playback` | multi |
| `tui/keys` | `ActionNowPlayingInfo` | const | `internal/tui` | root |
| `tui/keys` | `ActionPaste` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlayNext` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlayPause` | const | `internal/tui`, `tui/playback` | multi |
| `tui/keys` | `ActionPlaylistBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlaylistCreate` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlaylistDelete` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlaylistEnqueueAll` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlaylistOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionPlaylistRename` | const | `internal/tui` | root |
| `tui/keys` | `ActionPrevView` | const | `internal/tui` | root |
| `tui/keys` | `ActionPreview` | const | `internal/tui` | root |
| `tui/keys` | `ActionPrevious` | const | `internal/tui`, `tui/playback` | multi |
| `tui/keys` | `ActionQueueSummary` | const | `internal/tui` | root |
| `tui/keys` | `ActionQuit` | const | `internal/tui` | root |
| `tui/keys` | `ActionRadioToggle` | const | `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/keys` | `ActionRepeatCycle` | const | `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/keys` | `ActionRescan` | const | `internal/tui` | root |
| `tui/keys` | `ActionResetPosition` | const | `internal/tui` | root |
| `tui/keys` | `ActionSaveMixAsPlaylist` | const | `internal/tui` | root |
| `tui/keys` | `ActionSaveQueueAsPlaylist` | const | `internal/tui` | root |
| `tui/keys` | `ActionSeekBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionSeekBackLong` | const | `internal/tui` | root |
| `tui/keys` | `ActionSeekForward` | const | `internal/tui` | root |
| `tui/keys` | `ActionSeekForwardLong` | const | `internal/tui` | root |
| `tui/keys` | `ActionSelect` | const | `internal/tui`, `tui/queue` | multi |
| `tui/keys` | `ActionSettings` | const | `internal/tui` | root |
| `tui/keys` | `ActionShuffleToggle` | const | `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/keys` | `ActionSmartPlaylistCreate` | const | `internal/tui` | root |
| `tui/keys` | `ActionSmartPlaylistEdit` | const | `internal/tui` | root |
| `tui/keys` | `ActionSortAsc` | const | `internal/tui` | root |
| `tui/keys` | `ActionSortCycle` | const | `internal/tui` | root |
| `tui/keys` | `ActionSortDesc` | const | `internal/tui` | root |
| `tui/keys` | `ActionStop` | const | `internal/tui` | root |
| `tui/keys` | `ActionStopAfterCurrent` | const | `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/keys` | `ActionSwitchAlbums` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchArtists` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchBrowse` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchFavorites` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchGenres` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchInsights` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchLibrary` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchMixes` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchPlaylists` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchQueue` | const | `internal/tui` | root |
| `tui/keys` | `ActionSwitchTags` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagBack` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagCreate` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagDelete` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagEnqueueAll` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagFilter` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagMerge` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagOpen` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagPicker` | const | `internal/tui` | root |
| `tui/keys` | `ActionTagRename` | const | `internal/tui` | root |
| `tui/keys` | `ActionToggleFavorite` | const | `internal/tui` | root |
| `tui/keys` | `ActionTrackInfo` | const | `internal/tui` | root |
| `tui/keys` | `ActionUndo` | const | `internal/tui` | root |
| `tui/keys` | `ActionVisualMode` | const | `internal/tui` | root |
| `tui/keys` | `ActionVolumeDown` | const | `internal/tui`, `tui/playback` | multi |
| `tui/keys` | `ActionVolumeUp` | const | `internal/tui`, `tui/playback` | multi |
| `tui/keys` | `ActionYank` | const | `internal/tui` | root |
| `tui/keys` | `Context` | type | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextAlbums` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextArtists` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextBrowse` | const | `internal/tui`, `tui/overlay†` | multi |
| `tui/keys` | `ContextFavorites` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextGenres` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextGlobal` | const | `internal/tui`, `tui/overlay`, `tui/statusbar†` | multi |
| `tui/keys` | `ContextInsights` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextLibrary` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextMixes` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextModals` | const | `tui/overlay` | peer |
| `tui/keys` | `ContextPlaylists` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextQueue` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `ContextTags` | const | `internal/tui`, `tui/overlay` | multi |
| `tui/keys` | `HintMode` | type | — | api |
| `tui/keys` | `HintModeDefault` | const | `internal/tui` | root |
| `tui/keys` | `HintModeSelection` | const | `internal/tui` | root |
| `tui/keys` | `HintModeVisual` | const | `internal/tui` | root |
| `tui/keys` | `NewRegistry` | func | `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/keys` | `Registry` | type | `internal/tui` | root |
| `tui/keys` | `Registry.Bindings` | field | `internal/tui†`, `tui/overlay†` | multi (tests only) |
| `tui/keys` | `Registry.KeysFor` | method | `internal/tui` | root |
| `tui/keys` | `Registry.Lookup` | method | `internal/tui†`, `tui/overlay†`, `tui/statusbar†` | multi (tests only) |
| `tui/keys` | `Registry.Resolve` | method | `internal/tui` | root |
| `tui/keys` | `RegistryKey` | type | — | api |
| `tui/keys` | `RegistryKey.Ctx` | field | `tui/overlay†` | peer (tests only) |
| `tui/keys` | `RegistryKey.Key` | field | `tui/overlay†` | peer (tests only) |
| `tui/keys` | `RenderFooterHints` | func | `internal/tui` | root |
| `tui/playback` | `MissingTrackMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `MissingTrackMsg.Path` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `MissingTrackSelectedMsg` | type | `internal/tui`, `tui/queue` | multi |
| `tui/playback` | `MissingTrackSelectedMsg.Hash` | field | `internal/tui†`, `tui/queue` | multi |
| `tui/playback` | `MissingTrackSelectedMsg.Index` | field | `internal/tui`, `tui/queue` | multi |
| `tui/playback` | `MissingTrackSelectedMsg.Path` | field | `internal/tui`, `tui/queue` | multi |
| `tui/playback` | `ModeIndexRadio` | const | `internal/tui` | root |
| `tui/playback` | `ModeIndexRepeat` | const | `internal/tui` | root |
| `tui/playback` | `ModeIndexShuffle` | const | `internal/tui` | root |
| `tui/playback` | `ModeIndexStopAfter` | const | `internal/tui` | root |
| `tui/playback` | `ModeIndicator` | type | — | api |
| `tui/playback` | `ModeIndicator.Active` | field | `tui/overlay`, `tui/statusbar` | multi |
| `tui/playback` | `ModeIndicator.Glyph` | field | `internal/tui`, `tui/overlay`, `tui/statusbar` | multi |
| `tui/playback` | `ModeIndicator.Key` | field | `tui/overlay`, `tui/statusbar†` | multi |
| `tui/playback` | `ModeIndicator.Name` | field | `tui/overlay`, `tui/statusbar†` | multi |
| `tui/playback` | `ModeIndicator.State` | field | `internal/tui`, `tui/overlay`, `tui/statusbar†` | multi |
| `tui/playback` | `ModeIndicators` | func | `internal/tui`, `tui/overlay`, `tui/statusbar` | multi |
| `tui/playback` | `ModeState` | func | `internal/tui` | root |
| `tui/playback` | `PauseChangedMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `PauseChangedMsg.Paused` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `PlayFromIndexResultMsg` | type | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Err` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Hash` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Index` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Path` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Ran` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Replay` | field | `internal/tui` | root |
| `tui/playback` | `PlayFromIndexResultMsg.Request` | field | `internal/tui` | root |
| `tui/playback` | `PlayQueueIndexMsg` | type | `internal/tui`, `tui/queue` | multi |
| `tui/playback` | `PlayQueueIndexMsg.Index` | field | `internal/tui`, `tui/queue` | multi |
| `tui/playback` | `PlayRecordedMsg` | type | `internal/tui` | root |
| `tui/playback` | `PlayRecordedMsg.Counted` | field | `internal/tui` | root |
| `tui/playback` | `PlayRecordedMsg.Hash` | field | `internal/tui` | root |
| `tui/playback` | `PlayRecordedMsg.PlayedAt` | field | `internal/tui` | root |
| `tui/playback` | `PlaybackEndedMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `PlayerDisconnectedMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `PlayerStatusMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `PlayerStatusMsg.Text` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `PositionSyncInterval` | const | `internal/tui` | root |
| `tui/playback` | `RemoteAction` | func | `internal/tui` | root |
| `tui/playback` | `RemoteActionMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `RemoteActionMsg.ActionName` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `RepeatAll` | const | `internal/app`, `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/playback` | `RepeatMode` | type | `internal/app`, `internal/tui`, `tui/overlay`, `tui/statusbar` | multi |
| `tui/playback` | `RepeatMode.DisplayString` | method | `internal/tui` | root |
| `tui/playback` | `RepeatMode.String` | method | `internal/app`, `internal/tui` | multi |
| `tui/playback` | `RepeatOff` | const | `internal/app`, `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/playback` | `RepeatOne` | const | `internal/app`, `internal/tui`, `tui/overlay†`, `tui/statusbar†` | multi |
| `tui/playback` | `SessionTickInterval` | const | `internal/tui` | root |
| `tui/playback` | `State` | type | `internal/tui`, `tui/overlay`, `tui/queue`, `tui/statusbar` | multi |
| `tui/playback` | `StateIdle` | const | `internal/app†`, `internal/tui`, `tui/overlay†`, `tui/queue†`, `tui/statusbar` | multi |
| `tui/playback` | `StatePaused` | const | `internal/tui`, `tui/overlay`, `tui/queue`, `tui/statusbar` | multi |
| `tui/playback` | `StatePlaying` | const | `internal/tui`, `tui/overlay`, `tui/queue†`, `tui/statusbar` | multi |
| `tui/playback` | `StateStopped` | const | `internal/tui`, `tui/overlay`, `tui/queue`, `tui/statusbar` | multi |
| `tui/playback` | `TrackChangedMsg` | type | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.Artist` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.Hash` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.Index` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.PlaylistSource` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.QueueID` | field | `internal/tui`, `main` | multi |
| `tui/playback` | `TrackChangedMsg.Title` | field | `internal/tui`, `main` | multi |
| `tui/overlay` | `BorderH` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `BorderV` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `Box` | type | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Box.Hint` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Box.MaxInnerWidth` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Box.Render` | method | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Box.Rows` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Box.Title` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `CornerBL` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `CornerBR` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `CornerTL` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `CornerTR` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `CrossL` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `CrossR` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `FilteredHelpSections` | func | `internal/tui` | root |
| `tui/overlay` | `GoToAlbum` | const | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToArtist` | const | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToDismissedMsg` | type | `internal/tui` | root |
| `tui/overlay` | `GoToGenre` | const | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToKind` | type | — | api |
| `tui/overlay` | `GoToModal` | type | `internal/tui` | root |
| `tui/overlay` | `GoToModal.HandleKey` | method | `internal/tui` | root |
| `tui/overlay` | `GoToModal.Render` | method | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToOption` | type | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToOption.Kind` | field | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToOption.Value` | field | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `GoToPickedMsg` | type | `internal/tui` | root |
| `tui/overlay` | `GoToPickedMsg.Kind` | field | `internal/tui` | root |
| `tui/overlay` | `GoToPickedMsg.Value` | field | `internal/tui` | root |
| `tui/overlay` | `GoToPlaylist` | const | `internal/tui` | root |
| `tui/overlay` | `HPadding` | const | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `HelpOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `HelpOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `HelpSection` | type | `internal/tui` | root |
| `tui/overlay` | `HelpSection.Context` | field | `internal/tui†` | root (tests only) |
| `tui/overlay` | `HelpSection.Title` | field | `internal/tui†` | root (tests only) |
| `tui/overlay` | `HelpSections` | func | `internal/tui` | root |
| `tui/overlay` | `HintRows` | func | `internal/tui` | root |
| `tui/overlay` | `MinInnerWidth` | const | `tui/metadata`, `tui/settings` | multi |
| `tui/overlay` | `ModesLegendOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `ModesLegendOverlay.Radio` | field | `internal/tui` | root |
| `tui/overlay` | `ModesLegendOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `ModesLegendOverlay.Repeat` | field | `internal/tui` | root |
| `tui/overlay` | `ModesLegendOverlay.Shuffle` | field | `internal/tui` | root |
| `tui/overlay` | `ModesLegendOverlay.StopAfter` | field | `internal/tui` | root |
| `tui/overlay` | `MusicDirHeadline` | func | `internal/tui` | root |
| `tui/overlay` | `NewGoToModal` | func | `internal/tui`, `tui/uitest†` | multi |
| `tui/overlay` | `NewTrackInfoOverlay` | func | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Album` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Artist` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Elapsed` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.PlayCount` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.State` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.TagsLine` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Title` | field | `internal/tui` | root |
| `tui/overlay` | `NowPlayingOverlay.Total` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Album` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Artist` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Elapsed` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Hash` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.PlayCount` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.TagsLine` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Title` | field | `internal/tui` | root |
| `tui/overlay` | `PreviewOverlay.Total` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.Albums` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.Artists` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.RemainMs` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.RemainN` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.Sources` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.TotalMs` | field | `internal/tui` | root |
| `tui/overlay` | `QueueSummaryOverlay.Tracks` | field | `internal/tui` | root |
| `tui/overlay` | `RenderCenteredLines` | func | `tui/tags` | peer |
| `tui/overlay` | `RenderTopLines` | func | `tui/palette`, `tui/settings` | multi |
| `tui/overlay` | `Row` | type | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Row.Blank` | field | `internal/tui`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Row.Label` | field | `tui/smartrules` | peer |
| `tui/overlay` | `Row.Styled` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `Row.Text` | field | `internal/tui`, `tui/favorites`, `tui/metadata`, `tui/smartrules` | multi |
| `tui/overlay` | `SearchHelpSections` | func | `internal/tui` | root |
| `tui/overlay` | `StatusBarHelpSectionTitle` | const | `internal/tui†` | root (tests only) |
| `tui/overlay` | `TrackInfoCmd` | func | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg` | type | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg.Err` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg.Hash` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg.Path` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg.PlayCount` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoMsg.Tags` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoOverlay` | type | `internal/tui` | root |
| `tui/overlay` | `TrackInfoOverlay.Path` | field | `internal/tui` | root |
| `tui/overlay` | `TrackInfoOverlay.Render` | method | `internal/tui` | root |
| `tui/overlay` | `WelcomeRow` | func | `internal/tui` | root |
| `tui/statusbar` | `LongOpState` | type | `internal/tui` | root |
| `tui/statusbar` | `LongOpState.Active` | field | `internal/tui` | root |
| `tui/statusbar` | `LongOpState.Label` | field | `internal/tui` | root |
| `tui/statusbar` | `LongOpState.StartedAt` | field | `internal/tui` | root |
| `tui/statusbar` | `LongOpState.Update` | method | `internal/tui` | root |
| `tui/statusbar` | `NotifInfo` | const | `internal/tui` | root |
| `tui/statusbar` | `NotifNormal` | const | `internal/tui` | root |
| `tui/statusbar` | `NotifPriority` | type | — | api |
| `tui/statusbar` | `StatusBar` | type | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Album` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Artist` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.BucketLabel` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.ConfigCreated` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.ConfigPath` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Elapsed` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.ElapsedSecs` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.HandleMissingTrack` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.HandleScanProgress` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.LongOp` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.NextFadeAt` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Notifications` | field | `internal/tui†` | root (tests only) |
| `tui/statusbar` | `StatusBar.Notify` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.NotifyDuration` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.NotifyInfo` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.PlaybackState` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.PlaylistSource` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.PruneNotifications` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.PushMsg` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.QueueEmpty` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.QueuePosition` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.RadioMode` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Render` | method | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.RepeatMode` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.SelectionCount` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Shuffle` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.StopAfterCurrent` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.Title` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.TopNotification` | method | `internal/tui†` | root (tests only) |
| `tui/statusbar` | `StatusBar.Total` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.TotalSecs` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.TransientDur` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.UndoExpiresAt` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.VisualMode` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.WatchDegraded` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.YankCount` | field | `internal/tui` | root |
| `tui/statusbar` | `StatusBar.YankIsCut` | field | `internal/tui` | root |
| `tui/statusbar` | `TransientNotification` | type | `internal/tui†` | root (tests only) |
| `tui/statusbar` | `TransientNotification.ExpiresAt` | field | `internal/tui†` | root (tests only) |
| `tui/statusbar` | `TransientNotification.Msg` | field | `internal/tui†` | root (tests only) |
| `tui/statusbar` | `TransientNotifyMsg` | type | `internal/tui`, `tui/queue`, `tui/uitest†` | multi |
| `tui/settings` | `Action` | type | — | api |
| `tui/settings` | `ActionCancel` | const | `internal/tui` | root |
| `tui/settings` | `ActionEQDelete` | const | `internal/tui` | root |
| `tui/settings` | `ActionEQLive` | const | `internal/tui` | root |
| `tui/settings` | `ActionEQSave` | const | `internal/tui` | root |
| `tui/settings` | `ActionSave` | const | `internal/tui` | root |
| `tui/settings` | `ActionStereoLive` | const | `internal/tui` | root |
| `tui/settings` | `ActionThemeLive` | const | `internal/tui` | root |
| `tui/settings` | `Field` | type | `internal/tui` | root |
| `tui/settings` | `Field.Value` | field | `internal/tui†` | root (tests only) |
| `tui/settings` | `NewSettingsInput` | func | `internal/tui†` | root (tests only) |
| `tui/settings` | `NewSettingsOverlay` | func | `internal/tui`, `tui/uitest†` | multi |
| `tui/settings` | `SettingsOverlay` | type | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.EQConfirmDel` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.EQNameInput` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.EQPresets` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.EQState` | method | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.ExclPathInput` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.ExcludedPaths` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.Fields` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.ForwardMsg` | method | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.HandleKey` | method | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.LibDirInput` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.LibraryDirs` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.PrimaryInput` | field | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.Render` | method | `internal/tui`, `tui/uitest†` | multi |
| `tui/settings` | `SettingsOverlay.StereoWideState` | method | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.Tab` | field | `internal/tui†` | root (tests only) |
| `tui/settings` | `SettingsOverlay.ThemeState` | method | `internal/tui` | root |
| `tui/settings` | `SettingsOverlay.ToConfig` | method | `internal/tui` | root |
| `tui/settings` | `Tab` | type | — | api |
| `tui/settings` | `TabDisplay` | const | `internal/tui†` | root (tests only) |
| `tui/settings` | `TabEqualizer` | const | `internal/tui†` | root (tests only) |
| `tui/settings` | `TabPlayback` | const | `internal/tui†` | root (tests only) |
| `tui/smartrules` | `Action` | type | — | api |
| `tui/smartrules` | `ActionClose` | const | `internal/tui` | root |
| `tui/smartrules` | `ActionCommit` | const | `internal/tui` | root |
| `tui/smartrules` | `NewSmartRuleEditor` | func | `internal/tui` | root |
| `tui/smartrules` | `NewSmartRuleEditorEdit` | func | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor` | type | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.BuildQuery` | method | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.EditID` | field | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.ForwardMsg` | method | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.HandleKey` | method | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.IsEdit` | field | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.Name` | method | `internal/tui` | root |
| `tui/smartrules` | `SmartRuleEditor.Render` | method | `internal/tui` | root |
| `tui/metadata` | `Action` | type | — | api |
| `tui/metadata` | `ActionCancel` | const | `internal/tui` | root |
| `tui/metadata` | `ActionSave` | const | `internal/tui` | root |
| `tui/metadata` | `Editor` | type | `internal/tui` | root |
| `tui/metadata` | `Editor.Album` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Artist` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Dirty` | field | `internal/tui†` | root (tests only) |
| `tui/metadata` | `Editor.DirtyFields` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Duration` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Fields` | field | `internal/tui†` | root (tests only) |
| `tui/metadata` | `Editor.ForwardMsg` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Genre` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.HandleKey` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Hash` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Hashes` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.IsBulk` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Mode` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Render` | method | `internal/tui` | root |
| `tui/metadata` | `Editor.Title` | method | `internal/tui` | root |
| `tui/metadata` | `Field` | type | — | api |
| `tui/metadata` | `FieldDuration` | const | `internal/tui†` | root (tests only) |
| `tui/metadata` | `FormatDuration` | func | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay` | type | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.CanApply` | method | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.ChangeRows` | method | `internal/tui†` | root (tests only) |
| `tui/metadata` | `ImportOverlay.Cursor` | field | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.Offset` | field | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.PerPage` | method | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.Plan` | field | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.Reclamp` | method | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.Render` | method | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.RowCount` | method | `internal/tui` | root |
| `tui/metadata` | `ImportOverlay.ShowWarnings` | field | `internal/tui` | root |
| `tui/metadata` | `ImportRowBudget` | func | `internal/tui†` | root (tests only) |
| `tui/metadata` | `Mode` | type | `internal/tui` | root |
| `tui/metadata` | `ModeDefault` | const | `internal/tui` | root |
| `tui/metadata` | `ModeRenameAlbum` | const | `internal/tui` | root |
| `tui/metadata` | `ModeRenameArtist` | const | `internal/tui` | root |
| `tui/metadata` | `NewAlbumRenameEditor` | func | `internal/tui` | root |
| `tui/metadata` | `NewArtistRenameEditor` | func | `internal/tui` | root |
| `tui/metadata` | `NewBulkEditor` | func | `internal/tui` | root |
| `tui/metadata` | `NewEditor` | func | `internal/tui` | root |
| `tui/metadata` | `NewImportOverlay` | func | `internal/tui` | root |
| `tui/metadata` | `NewWriteQueue` | func | `internal/tui` | root |
| `tui/metadata` | `ParseDuration` | func | `internal/tui` | root |
| `tui/metadata` | `WriteQueue` | type | `internal/tui` | root |
| `tui/metadata` | `WriteQueue.Claim` | method | `internal/tui` | root |
| `tui/metadata` | `WriteQueue.Publish` | method | `internal/tui` | root |
| `tui/queue` | `BulkSelectionToggleMsg` | type | `internal/tui` | root |
| `tui/queue` | `BulkSelectionToggleMsg.Index` | field | `internal/tui` | root |
| `tui/queue` | `ClearQueueMsg` | type | `internal/tui` | root |
| `tui/queue` | `DequeueTrackMsg` | type | `internal/tui` | root |
| `tui/queue` | `DequeueTrackMsg.Index` | field | `internal/tui` | root |
| `tui/queue` | `Entry` | type | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Album` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Artist` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.DJOwned` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.DJReasons` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.DiscNumber` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Display` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.DurationMs` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Hash` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.ID` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Missing` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.OrigIdx` | field | `internal/tui` | root |
| `tui/queue` | `Entry.Path` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Position` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Selected` | field | `internal/tui` | root |
| `tui/queue` | `Entry.SourcePlaylist` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.Title` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `Entry.TrackNumber` | field | `internal/app`, `internal/tui` | multi |
| `tui/queue` | `EntryBucket` | func | `internal/tui` | root |
| `tui/queue` | `FileExists` | var | `internal/tui†` | root (tests only) |
| `tui/queue` | `MoveTrackDownMsg` | type | `internal/tui` | root |
| `tui/queue` | `MoveTrackDownMsg.Index` | field | `internal/tui` | root |
| `tui/queue` | `MoveTrackUpMsg` | type | `internal/tui` | root |
| `tui/queue` | `MoveTrackUpMsg.Index` | field | `internal/tui` | root |
| `tui/queue` | `NowPlayingIndicator` | func | `internal/tui` | root |
| `tui/queue` | `PlacementRefusal` | func | `internal/tui` | root |
| `tui/queue` | `SortQueueIndices` | func | `internal/tui†` | root (tests only) |
| `tui/queue` | `Traversal` | type | `internal/tui` | root |
| `tui/queue` | `Traversal.CurrentPosition` | field | `internal/tui` | root |
| `tui/queue` | `Traversal.Order` | field | `internal/tui` | root |
| `tui/queue` | `Traversal.Shuffled` | field | `internal/tui` | root |
| `tui/queue` | `TraversalSource` | type | `internal/tui` | root |
| `tui/queue` | `View` | type | `internal/tui` | root |
| `tui/queue` | `View.CancelSaveAsPlaylist` | method | `internal/tui` | root |
| `tui/queue` | `View.Cursor` | field | `internal/tui` | root |
| `tui/queue` | `View.DisplayOrder` | field | `internal/tui` | root |
| `tui/queue` | `View.Entries` | field | `internal/tui` | root |
| `tui/queue` | `View.EntryIndex` | method | `internal/tui†` | root (tests only) |
| `tui/queue` | `View.Filter` | field | `internal/tui` | root |
| `tui/queue` | `View.FilteredEntries` | method | `internal/tui` | root |
| `tui/queue` | `View.FindEntryDisplayPosition` | method | `internal/tui` | root |
| `tui/queue` | `View.FindEntryFilteredPosition` | method | `internal/tui` | root |
| `tui/queue` | `View.FocusNowPlaying` | method | `internal/tui` | root |
| `tui/queue` | `View.FocusedEntryIndex` | method | `internal/tui` | root |
| `tui/queue` | `View.HandleAction` | method | `internal/tui` | root |
| `tui/queue` | `View.HasSelection` | method | `internal/tui` | root |
| `tui/queue` | `View.Height` | field | `internal/tui†` | root (tests only) |
| `tui/queue` | `View.IsSavingAsPlaylist` | method | `internal/tui` | root |
| `tui/queue` | `View.MarkMissing` | method | `internal/tui` | root |
| `tui/queue` | `View.MarkMissingByHash` | method | `internal/tui` | root |
| `tui/queue` | `View.MoveQueueCursor` | method | `internal/tui†` | root (tests only) |
| `tui/queue` | `View.PlacementBlockReason` | method | `internal/tui` | root |
| `tui/queue` | `View.RecomputeDisplayOrder` | method | `internal/tui` | root |
| `tui/queue` | `View.Render` | method | `internal/tui` | root |
| `tui/queue` | `View.SaveInput` | field | `internal/tui†` | root (tests only) |
| `tui/queue` | `View.SaveInputIsEmpty` | method | `internal/tui` | root |
| `tui/queue` | `View.SaveInputValue` | method | `internal/tui` | root |
| `tui/queue` | `View.SavingAsPlaylist` | field | `internal/tui` | root |
| `tui/queue` | `View.SavingFromSelection` | field | `internal/tui` | root |
| `tui/queue` | `View.ScrollOffset` | field | `internal/tui` | root |
| `tui/queue` | `View.ScrollToNowPlaying` | method | `internal/tui` | root |
| `tui/queue` | `View.SelectedHashes` | method | `internal/tui` | root |
| `tui/queue` | `View.SetSize` | method | — | iface: internal/tui.resizable |
| `tui/queue` | `View.SortState` | field | `internal/tui` | root |
| `tui/queue` | `View.StartSaveAsPlaylist` | method | `internal/tui` | root |
| `tui/queue` | `View.TagFilterHashes` | field | `internal/tui` | root |
| `tui/queue` | `View.Traversal` | field | `internal/tui` | root |
| `tui/queue` | `View.UpdateSaveInput` | method | `internal/tui` | root |
| `tui/queue` | `View.UpdateSaveInputMsg` | method | `internal/tui` | root |
| `tui/queue` | `View.VisibleRows` | method | `internal/tui` | root |
| `tui/queue` | `View.Width` | field | `internal/tui†` | root (tests only) |
| `tui/browse` | `AlbumsView` | type | `internal/tui` | root |
| `tui/browse` | `AlbumsView.AlbumCounts` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Albums` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Back` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.ClampCursor` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Cursor` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.EnqueueFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Filter` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.FocusedAlbum` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.FocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.HandleAction` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Height` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Level` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.OpenTracks` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.PlayNextFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.RecomputeAlbumsSort` | method | `internal/tui` | root |
| `tui/browse` | `AlbumsView.ScrollOffset` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.SelectedAlbum` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.SelectedHashes` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.SortState` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Tracks` | field | `internal/tui` | root |
| `tui/browse` | `AlbumsView.Width` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView` | type | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Albums` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.ArtistCounts` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Artists` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Back` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.ClampCursor` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Cursor` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.EnqueueFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Filter` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.FocusedAlbum` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.FocusedArtist` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.FocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.HandleAction` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Height` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Level` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.OpenAlbums` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.OpenTracks` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.PlayNextFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.RecomputeArtistsSort` | method | `internal/tui` | root |
| `tui/browse` | `ArtistsView.ScrollOffset` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.SelectedAlbum` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.SelectedArtist` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.SelectedHashes` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.SortState` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Tracks` | field | `internal/tui` | root |
| `tui/browse` | `ArtistsView.Width` | field | `internal/tui` | root |
| `tui/browse` | `BrowseMode` | type | `internal/tui` | root |
| `tui/browse` | `BrowseModeAlbums` | const | `internal/tui` | root |
| `tui/browse` | `BrowseModeArtists` | const | `internal/tui` | root |
| `tui/browse` | `BrowseModeGenres` | const | `internal/tui` | root |
| `tui/browse` | `BrowseView` | type | `internal/tui` | root |
| `tui/browse` | `BrowseView.Albums` | field | `internal/tui` | root |
| `tui/browse` | `BrowseView.Artists` | field | `internal/tui` | root |
| `tui/browse` | `BrowseView.Genres` | field | `internal/tui` | root |
| `tui/browse` | `BrowseView.Mode` | field | `internal/tui` | root |
| `tui/browse` | `BrowseView.Render` | method | `internal/tui` | root |
| `tui/browse` | `BrowseView.SetSize` | method | — | iface: internal/tui.resizable |
| `tui/browse` | `GenresView` | type | `internal/tui` | root |
| `tui/browse` | `GenresView.Artists` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.Back` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.ClampCursor` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.Cursor` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.EnqueueFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.Filter` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.FocusedArtist` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.FocusedGenre` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.FocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.GenreCounts` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.Genres` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.HandleAction` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.Height` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.Level` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.OpenArtists` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.OpenTracks` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.PlayNextFocusedTrack` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.RecomputeGenresSort` | method | `internal/tui` | root |
| `tui/browse` | `GenresView.ScrollOffset` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.SelectedArtist` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.SelectedGenre` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.SelectedHashes` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.SortState` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.Tracks` | field | `internal/tui` | root |
| `tui/browse` | `GenresView.Width` | field | `internal/tui` | root |
| `tui/insights` | `BacklogMonth` | type | `internal/tui` | root |
| `tui/insights` | `BacklogMonths` | func | `internal/tui` | root |
| `tui/insights` | `BucketKey` | func | `internal/tui` | root |
| `tui/insights` | `BucketLabel` | func | `internal/tui†` | root (tests only) |
| `tui/insights` | `BuildHealth` | func | `internal/tui` | root |
| `tui/insights` | `ClassFixed` | const | `internal/tui` | root |
| `tui/insights` | `ClassResolution` | const | `internal/tui` | root |
| `tui/insights` | `ClassWindowed` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `DrillAddedNeverPlayed` | const | `internal/tui` | root |
| `tui/insights` | `DrillArtistPlays` | const | `internal/tui` | root |
| `tui/insights` | `DrillDormant` | const | `internal/tui` | root |
| `tui/insights` | `DrillFirstPlaysBucket` | const | `internal/tui` | root |
| `tui/insights` | `DrillHealth` | const | `internal/tui` | root |
| `tui/insights` | `DrillHour` | const | `internal/tui` | root |
| `tui/insights` | `DrillKind` | type | — | api |
| `tui/insights` | `DrillKind.OpensSessions` | method | `internal/tui` | root |
| `tui/insights` | `DrillNone` | const | `internal/tui` | root |
| `tui/insights` | `DrillRhythmGenre` | const | `internal/tui` | root |
| `tui/insights` | `DrillSessionsAll` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `DrillSessionsCloser` | const | `internal/tui` | root |
| `tui/insights` | `DrillSessionsOpener` | const | `internal/tui` | root |
| `tui/insights` | `DrillTopArtists` | const | `internal/tui` | root |
| `tui/insights` | `DrillTrendBucket` | const | `internal/tui` | root |
| `tui/insights` | `DrillUnheardAlbum` | const | `internal/tui` | root |
| `tui/insights` | `DrillUnheardAll` | const | `internal/tui` | root |
| `tui/insights` | `DrillUnheardArtist` | const | `internal/tui` | root |
| `tui/insights` | `DrillUnheardGenre` | const | `internal/tui` | root |
| `tui/insights` | `DrillWeekday` | const | `internal/tui` | root |
| `tui/insights` | `GenreHours` | type | `internal/tui` | root |
| `tui/insights` | `GenreKey` | func | `internal/tui` | root |
| `tui/insights` | `HealthArtistDuplicate` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthArtistKey` | func | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthDeepCutsFlat` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthGenreCollision` | const | `internal/tui` | root |
| `tui/insights` | `HealthMissingMetadata` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthPlaceholderGenre` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthRow` | type | `internal/tui` | root |
| `tui/insights` | `HealthRow.Canonical` | field | `internal/tui` | root |
| `tui/insights` | `HealthRow.Hashes` | field | `internal/tui` | root |
| `tui/insights` | `HealthRow.Kind` | field | `internal/tui` | root |
| `tui/insights` | `HealthRow.Label` | field | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthRow.Section` | field | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthRow.Variants` | field | `internal/tui` | root |
| `tui/insights` | `HealthRowKind` | type | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionArtists` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionCollisions` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionDeepCuts` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionDurations` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionMissing` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthSectionPlaceholders` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HealthUnverifiedDuration` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `HourGenreFold` | func | `internal/tui` | root |
| `tui/insights` | `LevelBriefing` | const | `internal/tui` | root |
| `tui/insights` | `LevelSessionTracks` | const | `internal/tui` | root |
| `tui/insights` | `LevelSessions` | const | `internal/tui` | root |
| `tui/insights` | `LevelTracks` | const | `internal/tui` | root |
| `tui/insights` | `MergePlan` | type | `internal/tui` | root |
| `tui/insights` | `MergePlan.Canonical` | field | `internal/tui` | root |
| `tui/insights` | `MergePlan.Hashes` | field | `internal/tui` | root |
| `tui/insights` | `MergePlan.Variants` | field | `internal/tui` | root |
| `tui/insights` | `Panel` | type | — | api |
| `tui/insights` | `PanelBodyLines` | func | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelClass` | type | — | api |
| `tui/insights` | `PanelConcentration` | const | `internal/tui` | root |
| `tui/insights` | `PanelCoverage` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelDataHealth` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelDiscovery` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelMomentum` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelSessions` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `PanelSpec` | type | — | api |
| `tui/insights` | `PanelSpec.Class` | field | `internal/tui` | root |
| `tui/insights` | `PanelSpec.Name` | field | `internal/tui` | root |
| `tui/insights` | `PanelTimeVsPlays` | const | `internal/tui†` | root (tests only) |
| `tui/insights` | `Row` | type | `internal/tui` | root |
| `tui/insights` | `Row.Drill` | field | `internal/tui` | root |
| `tui/insights` | `Row.Key` | field | `internal/tui` | root |
| `tui/insights` | `Row.Label` | field | `internal/tui` | root |
| `tui/insights` | `Row.Section` | field | `internal/tui†` | root (tests only) |
| `tui/insights` | `RutTopArtists` | const | `internal/tui` | root |
| `tui/insights` | `SessionsMatching` | func | `internal/tui` | root |
| `tui/insights` | `View` | type | `internal/tui` | root |
| `tui/insights` | `View.AtTracks` | method | `internal/tui` | root |
| `tui/insights` | `View.Back` | method | `internal/tui` | root |
| `tui/insights` | `View.Backlog` | field | `internal/tui` | root |
| `tui/insights` | `View.BodyRows` | method | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.ClampCursor` | method | `internal/tui` | root |
| `tui/insights` | `View.Cursor` | field | `internal/tui` | root |
| `tui/insights` | `View.Dormant` | field | `internal/tui` | root |
| `tui/insights` | `View.DrillLabel` | field | `internal/tui` | root |
| `tui/insights` | `View.Empty` | method | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.FocusedHealthRow` | method | `internal/tui` | root |
| `tui/insights` | `View.FocusedRow` | method | `internal/tui` | root |
| `tui/insights` | `View.FocusedSession` | method | `internal/tui` | root |
| `tui/insights` | `View.FocusedTrack` | method | `internal/tui` | root |
| `tui/insights` | `View.Gen` | field | `internal/tui` | root |
| `tui/insights` | `View.GenreCoverage` | field | `internal/tui` | root |
| `tui/insights` | `View.GenreHours` | field | `internal/tui` | root |
| `tui/insights` | `View.HandleCursorAction` | method | `internal/tui` | root |
| `tui/insights` | `View.Health` | field | `internal/tui` | root |
| `tui/insights` | `View.Height` | field | `internal/tui` | root |
| `tui/insights` | `View.Level` | field | `internal/tui` | root |
| `tui/insights` | `View.LoadErr` | field | `internal/tui` | root |
| `tui/insights` | `View.Loaded` | field | `internal/tui` | root |
| `tui/insights` | `View.Loading` | field | `internal/tui` | root |
| `tui/insights` | `View.Loc` | field | `internal/tui` | root |
| `tui/insights` | `View.Location` | method | `internal/tui` | root |
| `tui/insights` | `View.OpenDrill` | method | `internal/tui` | root |
| `tui/insights` | `View.OpenSessionList` | method | `internal/tui` | root |
| `tui/insights` | `View.Panel` | field | `internal/tui` | root |
| `tui/insights` | `View.PanelHeaderLines` | method | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.PendingMerge` | field | `internal/tui` | root |
| `tui/insights` | `View.Plays` | field | `internal/tui` | root |
| `tui/insights` | `View.Render` | method | `internal/tui` | root |
| `tui/insights` | `View.Resolution` | method | `internal/tui` | root |
| `tui/insights` | `View.Rows` | method | `internal/tui` | root |
| `tui/insights` | `View.ScrollOffset` | field | `internal/tui` | root |
| `tui/insights` | `View.SessionLabel` | field | `internal/tui` | root |
| `tui/insights` | `View.SessionList` | field | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.Sessions` | field | `internal/tui` | root |
| `tui/insights` | `View.SetSize` | method | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.ShiftPanel` | method | `internal/tui` | root |
| `tui/insights` | `View.ShiftResolution` | method | `internal/tui` | root |
| `tui/insights` | `View.ShiftWindow` | method | `internal/tui` | root |
| `tui/insights` | `View.Snap` | field | `internal/tui` | root |
| `tui/insights` | `View.Spec` | method | `internal/tui` | root |
| `tui/insights` | `View.Stale` | field | `internal/tui` | root |
| `tui/insights` | `View.Timeline` | field | `internal/tui` | root |
| `tui/insights` | `View.TrackView` | field | `internal/tui` | root |
| `tui/insights` | `View.WindowHint` | method | `internal/tui†` | root (tests only) |
| `tui/insights` | `View.WindowIdx` | field | `internal/tui` | root |
| `tui/insights` | `Window` | type | — | api |
| `tui/insights` | `Window.Label` | field | `internal/tui` | root |
| `tui/insights` | `WindowSince` | func | `internal/tui` | root |
| `tui/insights` | `Windows` | var | `internal/tui` | root |
| `tui/mixes` | `MixesView` | type | `internal/tui` | root |
| `tui/mixes` | `MixesView.ActiveMix` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.Back` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.CachedMixes` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.FocusedMix` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.FocusedTrack` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.HandleCursorAction` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.Height` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.InsufficientData` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.Level` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.Mixes` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.OpenMix` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.PlaysSinceRefresh` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.Render` | method | `internal/tui` | root |
| `tui/mixes` | `MixesView.SelectedMix` | field | `internal/tui` | root |
| `tui/mixes` | `MixesView.SetSize` | method | — | iface: internal/tui.resizable |
| `tui/mixes` | `MixesView.TrackView` | field | `internal/tui` | root |
| `tui/autodj` | `Action` | type | — | api |
| `tui/autodj` | `ActionCancel` | const | `internal/tui` | root |
| `tui/autodj` | `ActionCommit` | const | `internal/tui` | root |
| `tui/autodj` | `ActionPreview` | const | `internal/tui` | root |
| `tui/autodj` | `ActionRecipe` | const | `internal/tui` | root |
| `tui/autodj` | `Applied` | type | `internal/tui` | root |
| `tui/autodj` | `Applied.Active` | field | `internal/tui` | root |
| `tui/autodj` | `Applied.Err` | field | `internal/tui` | root |
| `tui/autodj` | `Applied.Settings` | field | `internal/tui` | root |
| `tui/autodj` | `Applied.Token` | field | `internal/tui` | root |
| `tui/autodj` | `Catalog` | type | `internal/tui` | root |
| `tui/autodj` | `Catalog.Artists` | field | `internal/tui` | root |
| `tui/autodj` | `Catalog.Genres` | field | `internal/tui` | root |
| `tui/autodj` | `Catalog.KnownTags` | field | `internal/tui` | root |
| `tui/autodj` | `Catalog.Tags` | field | `internal/tui` | root |
| `tui/autodj` | `ErrRecipeNameTaken` | var | `internal/tui` | root |
| `tui/autodj` | `EvidenceGap` | type | `internal/tui` | root |
| `tui/autodj` | `EvidenceGap.Detail` | field | `internal/tui` | root |
| `tui/autodj` | `EvidenceGap.Signal` | field | `internal/tui` | root |
| `tui/autodj` | `New` | func | `internal/tui` | root |
| `tui/autodj` | `Options` | type | `internal/tui` | root |
| `tui/autodj` | `Options.Active` | field | `internal/tui` | root |
| `tui/autodj` | `Options.Applied` | field | `internal/tui` | root |
| `tui/autodj` | `Options.Catalog` | field | `internal/tui` | root |
| `tui/autodj` | `Options.Recipes` | field | `internal/tui` | root |
| `tui/autodj` | `Options.RecipesErr` | field | `internal/tui` | root |
| `tui/autodj` | `Preview` | type | `internal/tui` | root |
| `tui/autodj` | `Preview.Ahead` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.AheadMore` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.DiscoveryTarget` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Err` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Gaps` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Outcome` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Reasons` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Rejections` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Replanning` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Settings` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Shortages` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Stale` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Status` | field | `internal/tui` | root |
| `tui/autodj` | `Preview.Tracks` | field | `internal/tui` | root |
| `tui/autodj` | `PreviewFailed` | const | `internal/tui` | root |
| `tui/autodj` | `PreviewIdle` | const | `internal/tui` | root |
| `tui/autodj` | `PreviewPlanning` | const | `internal/tui` | root |
| `tui/autodj` | `PreviewReady` | const | `internal/tui` | root |
| `tui/autodj` | `PreviewStatus` | type | — | api |
| `tui/autodj` | `PreviewTrack` | type | `internal/tui` | root |
| `tui/autodj` | `PreviewTrack.Discovery` | field | `internal/tui` | root |
| `tui/autodj` | `PreviewTrack.Label` | field | `internal/tui` | root |
| `tui/autodj` | `PreviewTrack.Reasons` | field | `internal/tui` | root |
| `tui/autodj` | `QueuedTrack` | type | `internal/tui` | root |
| `tui/autodj` | `QueuedTrack.DJ` | field | `internal/tui` | root |
| `tui/autodj` | `QueuedTrack.Label` | field | `internal/tui` | root |
| `tui/autodj` | `QueuedTrack.Playing` | field | `internal/tui` | root |
| `tui/autodj` | `QueuedTrack.Reasons` | field | `internal/tui` | root |
| `tui/autodj` | `Recipe` | type | `internal/tui` | root |
| `tui/autodj` | `Recipe.ID` | field | `internal/tui` | root |
| `tui/autodj` | `Recipe.Invalid` | field | `internal/tui` | root |
| `tui/autodj` | `Recipe.Name` | field | `internal/tui` | root |
| `tui/autodj` | `Recipe.Settings` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeDelete` | const | `internal/tui` | root |
| `tui/autodj` | `RecipeOp` | type | — | api |
| `tui/autodj` | `RecipeOutcome` | type | `internal/tui` | root |
| `tui/autodj` | `RecipeOutcome.Err` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeOutcome.Name` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeOutcome.Op` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeOutcome.Recipes` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeRename` | const | `internal/tui` | root |
| `tui/autodj` | `RecipeRequest` | type | `internal/tui` | root |
| `tui/autodj` | `RecipeRequest.ID` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeRequest.Name` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeRequest.Op` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeRequest.Settings` | field | `internal/tui` | root |
| `tui/autodj` | `RecipeSave` | const | `internal/tui` | root |
| `tui/autodj` | `RecipeUpdate` | const | `internal/tui` | root |
| `tui/autodj` | `StatusLine` | type | `internal/tui` | root |
| `tui/autodj` | `StatusLine.Text` | field | `internal/tui` | root |
| `tui/autodj` | `StatusLine.Tone` | field | `internal/tui` | root |
| `tui/autodj` | `Tone` | type | — | api |
| `tui/autodj` | `ToneError` | const | `internal/tui` | root |
| `tui/autodj` | `ToneInfo` | const | `internal/tui` | root |
| `tui/autodj` | `ToneWarn` | const | `internal/tui` | root |
| `tui/autodj` | `TunePanel` | type | `internal/tui` | root |
| `tui/autodj` | `TunePanel.Ack` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.AckRecipe` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.AppliedSettings` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.Busy` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.CommitLabel` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.Cursor` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.Dirty` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.Draft` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.Err` | method | `internal/tui†` | root (tests only) |
| `tui/autodj` | `TunePanel.HandleKey` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.PendingRecipe` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.PreviewActive` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.RecipesActive` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.RemapTag` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.Render` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.Submit` | method | `internal/tui` | root |
| `tui/autodj` | `TunePanel.Unresolved` | method | `internal/tui†` | root (tests only) |
| `tui/uitest` | `NewUITestDriver` | func | `main` | harness |
| `tui/uitest` | `ObserveOptions` | type | `main` | harness |
| `tui/uitest` | `ObserveOptions.IncludeANSI` | field | `main` | harness |
| `tui/uitest` | `ObserveOptions.IncludeCommands` | field | `main` | harness |
| `tui/uitest` | `ParseScenario` | func | `main` | harness |
| `tui/uitest` | `ParseSize` | func | `main` | harness |
| `tui/uitest` | `RunOptions` | type | `main` | harness |
| `tui/uitest` | `RunOptions.CollectObservations` | field | `main` | harness |
| `tui/uitest` | `RunOptions.DrainTimeout` | field | `main` | harness |
| `tui/uitest` | `RunOptions.Fixture` | field | `main` | harness |
| `tui/uitest` | `RunOptions.IncludeANSI` | field | `main` | harness |
| `tui/uitest` | `RunOptions.Size` | field | `main` | harness |
| `tui/uitest` | `RunScenario` | func | `main` | harness |
| `tui/uitest` | `UITestAction` | type | `main` | harness |
| `tui/uitest` | `UITestAction.Assert` | field | — | json |
| `tui/uitest` | `UITestAction.Command` | field | — | json |
| `tui/uitest` | `UITestAction.Key` | field | — | json |
| `tui/uitest` | `UITestAction.Label` | field | — | json |
| `tui/uitest` | `UITestAction.Resize` | field | — | json |
| `tui/uitest` | `UITestAction.Text` | field | — | json |
| `tui/uitest` | `UITestAction.Wait` | field | — | json |
| `tui/uitest` | `UITestActionError` | type | — | api |
| `tui/uitest` | `UITestActionError.Action` | field | — | json |
| `tui/uitest` | `UITestActionError.Index` | field | `main` | harness |
| `tui/uitest` | `UITestActionError.Label` | field | `main` | harness |
| `tui/uitest` | `UITestActionError.Observation` | field | `main†` | harness (tests only) |
| `tui/uitest` | `UITestActionError.Reason` | field | `main` | harness |
| `tui/uitest` | `UITestAssertion` | type | — | api |
| `tui/uitest` | `UITestAssertion.Contains` | field | — | json |
| `tui/uitest` | `UITestAssertion.NoLayoutIssues` | field | — | json |
| `tui/uitest` | `UITestAssertion.NotContains` | field | — | json |
| `tui/uitest` | `UITestAssertion.Overlay` | field | — | json |
| `tui/uitest` | `UITestAssertion.StatusContains` | field | — | json |
| `tui/uitest` | `UITestAssertion.View` | field | — | json |
| `tui/uitest` | `UITestDriver` | type | `main` | harness |
| `tui/uitest` | `UITestDriver.ApplyAction` | method | `main` | harness |
| `tui/uitest` | `UITestDriver.Observe` | method | `main` | harness |
| `tui/uitest` | `UITestDriver.Release` | method | `main` | harness |
| `tui/uitest` | `UITestIssue` | type | — | api |
| `tui/uitest` | `UITestIssue.Actual` | field | — | json |
| `tui/uitest` | `UITestIssue.Detail` | field | — | json |
| `tui/uitest` | `UITestIssue.Kind` | field | — | json |
| `tui/uitest` | `UITestIssue.Limit` | field | — | json |
| `tui/uitest` | `UITestIssue.Line` | field | — | json |
| `tui/uitest` | `UITestIssueKind` | type | — | api |
| `tui/uitest` | `UITestObservation` | type | `main` | harness |
| `tui/uitest` | `UITestObservation.AnsiScreen` | field | — | json |
| `tui/uitest` | `UITestObservation.AvailableCommands` | field | `main†` | harness (tests only) |
| `tui/uitest` | `UITestObservation.CurrentView` | field | — | json |
| `tui/uitest` | `UITestObservation.Height` | field | — | json |
| `tui/uitest` | `UITestObservation.LayoutIssues` | field | — | json |
| `tui/uitest` | `UITestObservation.Overlay` | field | `main†` | harness (tests only) |
| `tui/uitest` | `UITestObservation.PendingAsync` | field | `main†` | harness (tests only) |
| `tui/uitest` | `UITestObservation.PlainScreen` | field | — | json |
| `tui/uitest` | `UITestObservation.PrimaryView` | field | `main†` | harness (tests only) |
| `tui/uitest` | `UITestObservation.Snapshot` | field | — | json |
| `tui/uitest` | `UITestObservation.Status` | field | — | json |
| `tui/uitest` | `UITestObservation.Width` | field | — | json |
| `tui/uitest` | `UITestScenario` | type | `main†` | harness (tests only) |
| `tui/uitest` | `UITestScenario.Actions` | field | — | json |
| `tui/uitest` | `UITestScenario.Fixture` | field | — | json |
| `tui/uitest` | `UITestScenario.Name` | field | — | json |
| `tui/uitest` | `UITestScenario.Size` | field | — | json |
| `tui/uitest` | `UITestScenarioResult` | type | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.ActionsRun` | field | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.Error` | field | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.FinalObservation` | field | — | json |
| `tui/uitest` | `UITestScenarioResult.Fixture` | field | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.OK` | field | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.Observations` | field | — | json |
| `tui/uitest` | `UITestScenarioResult.Scenario` | field | `main` | harness |
| `tui/uitest` | `UITestScenarioResult.Size` | field | `main` | harness |

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
