package tui

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// minActionCount is a plausibility floor for the number of parsed action
// identifiers. Go exposes no enum reflection, so a truncated parse (for
// instance if the enum were split into two const declarations) would silently
// mis-map action values to names; this guard turns that into a loud failure.
const minActionCount = 100

// actionNames returns the identifiers of the `action` enum in declaration
// order, so that names[i] is the name of the action whose value is i.
func actionNames(t *testing.T) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "keys/keybindings.go", nil, 0)
	if err != nil {
		t.Fatalf("parse keys/keybindings.go: %v", err)
	}

	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}

		var block []string
		found := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, ident := range value.Names {
				block = append(block, ident.Name)
				if ident.Name == "ActionNone" {
					found = true
				}
			}
		}
		if found {
			names = block
			break
		}
	}

	if names == nil {
		t.Fatal("no const block containing ActionNone found in keys/keybindings.go")
	}
	if names[0] != "ActionNone" {
		t.Fatalf("first action constant is %q, want ActionNone: index no longer equals action value", names[0])
	}
	if len(names) <= minActionCount {
		t.Fatalf("parsed only %d action names (want > %d): the enum is probably split across several const declarations", len(names), minActionCount)
	}
	return names
}

func TestActionNamesRoundTrip(t *testing.T) {
	names := actionNames(t)

	cases := []struct {
		a    keys.Action
		want string
	}{
		{keys.ActionNone, "ActionNone"},
		{keys.ActionQuit, "ActionQuit"},
		{keys.ActionMixUndismiss, "ActionMixUndismiss"},
		{keys.ActionInsightsMergeGenre, "ActionInsightsMergeGenre"},
	}
	for _, tc := range cases {
		if int(tc.a) >= len(names) {
			t.Fatalf("action %d (%s) is out of range of the %d parsed names", tc.a, tc.want, len(names))
		}
		if got := names[tc.a]; got != tc.want {
			t.Errorf("names[%d] = %q, want %q", tc.a, got, tc.want)
		}
	}

	if last := names[len(names)-1]; last != "ActionInsightsMergeGenre" {
		t.Errorf("last action constant is %q, want ActionInsightsMergeGenre", last)
	}
}

// actionLabel names an action for readable failure output, falling back to the
// numeric value when the AST-parsed name list does not reach it.
func actionLabel(names []string, a keys.Action) string {
	if int(a) >= 0 && int(a) < len(names) {
		return names[a]
	}
	return fmt.Sprintf("action(%d)", a)
}

// paletteCatalogActions returns the set of actions declared in paletteCatalog.
func paletteCatalogActions() map[keys.Action]struct{} {
	out := make(map[keys.Action]struct{}, len(paletteCatalog))
	for _, e := range paletteCatalog {
		out[e.act] = struct{}{}
	}
	return out
}

// paletteTriageModel builds a model whose palette command list exercises every
// conditional and dynamic entry (config/data dir openers, the tag filter
// clear entry, and the per-tag enqueue entries), so ID checks see the real ID
// space rather than a subset of it.
func paletteTriageModel() Model {
	m := New(context.Background())
	m.configPath = "/tmp/muse-test/config.toml"
	m.dataDir = "/tmp/muse-test"
	m.tags = []db.TagRow{{ID: 1, Name: "chill"}, {ID: 2, Name: "focus"}}
	tagID := int64(1)
	m.activeTagFilter = &tagID
	return m
}

// TestPaletteCatalogCoversRegistryActions asserts that every action bound in
// the registry has an explicit palette decision: catalogued, or permanently
// excluded with a written reason. There is no third outcome.
func TestPaletteCatalogCoversRegistryActions(t *testing.T) {
	names := actionNames(t)
	catalog := paletteCatalogActions()

	reported := make(map[keys.Action]struct{})
	for _, a := range keys.NewRegistry().Bindings {
		if a == keys.ActionNone {
			continue
		}
		if _, seen := reported[a]; seen {
			continue
		}
		_, inCatalog := catalog[a]
		_, inExcluded := paletteExcluded[a]
		if !inCatalog && !inExcluded {
			reported[a] = struct{}{}
			t.Errorf("%s is bound in the registry but is in neither paletteCatalog nor paletteExcluded", actionLabel(names, a))
		}
	}
}

// TestPaletteCatalogMapsAreDisjoint asserts no action carries two conflicting
// palette decisions at once.
func TestPaletteCatalogMapsAreDisjoint(t *testing.T) {
	names := actionNames(t)

	for a := range paletteCatalogActions() {
		if _, ok := paletteExcluded[a]; ok {
			t.Errorf("%s is both catalogued and permanently excluded", actionLabel(names, a))
		}
	}
}

// TestPaletteCatalogEntriesResolveToAKey asserts every catalogued action is
// reachable by at least one key, so the palette never advertises a blank
// shortcut for an entry that is meant to have one.
func TestPaletteCatalogEntriesResolveToAKey(t *testing.T) {
	names := actionNames(t)
	reg := keys.NewRegistry()

	for _, e := range paletteCatalog {
		bound := reg.KeysFor(e.ctx, e.act)
		if len(bound) == 0 && e.ctx != keys.ContextGlobal {
			bound = reg.KeysFor(keys.ContextGlobal, e.act)
		}
		if len(bound) == 0 {
			t.Errorf("catalog entry %q (%s) resolves to no key in ctx %v or contextGlobal", e.id, actionLabel(names, e.act), e.ctx)
		}
	}
}

// TestPaletteExclusionReasonsAreWritten asserts every exclusion carries a
// reason, so "why is this not in the palette?" is always answerable from the
// source.
func TestPaletteExclusionReasonsAreWritten(t *testing.T) {
	names := actionNames(t)

	for a, reason := range paletteExcluded {
		if reason == "" {
			t.Errorf("%s is excluded with an empty reason", actionLabel(names, a))
		}
	}
}

// TestPaletteCommandIDsAreUnique asserts catalog IDs are unique among
// themselves and do not collide with the hand-written palette entries, which
// share the same usage-tracking key space.
func TestPaletteCommandIDsAreUnique(t *testing.T) {
	catalogIDs := make(map[string]struct{}, len(paletteCatalog))
	for _, e := range paletteCatalog {
		if e.id == "" {
			t.Errorf("catalog entry %q has no id: usage tracking would silently fall back to the name", e.name)
			continue
		}
		if _, dup := catalogIDs[e.id]; dup {
			t.Errorf("duplicate catalog id %q", e.id)
		}
		catalogIDs[e.id] = struct{}{}
	}

	seen := make(map[string]string)
	for _, c := range paletteTriageModel().BuildPaletteCommands() {
		if prev, dup := seen[c.Key()]; dup {
			t.Errorf("palette id %q is used by both %q and %q", c.Key(), prev, c.Name)
		}
		seen[c.Key()] = c.Name
	}
	for id := range catalogIDs {
		if _, ok := seen[id]; !ok {
			t.Errorf("catalog id %q is missing from the built command list", id)
		}
	}
}

// TestPaletteCatalogShortcutsRoundTrip asserts the advertised shortcut really
// triggers the entry's action in its own context.
func TestPaletteCatalogShortcutsRoundTrip(t *testing.T) {
	names := actionNames(t)
	m := paletteTriageModel()
	reg := keys.NewRegistry()

	for _, e := range paletteCatalog {
		shortcut := m.paletteShortcutFor(e)
		if shortcut == "" {
			continue
		}
		if got := reg.Resolve(e.ctx, shortcut); got != e.act {
			t.Errorf("catalog entry %q advertises %q, which resolves to %s in ctx %v, want %s",
				e.id, shortcut, actionLabel(names, got), e.ctx, actionLabel(names, e.act))
		}
	}
}

// TestPaletteCatalogPreferKeysAreBound asserts a preferKey override names a
// key that is actually bound to the entry's action in its own context.
func TestPaletteCatalogPreferKeysAreBound(t *testing.T) {
	names := actionNames(t)
	reg := keys.NewRegistry()

	for _, e := range paletteCatalog {
		if e.preferKey == "" {
			continue
		}
		if got := reg.Lookup(e.ctx, e.preferKey); got != e.act {
			t.Errorf("catalog entry %q prefers key %q, which is bound to %s in ctx %v, want %s",
				e.id, e.preferKey, actionLabel(names, got), e.ctx, actionLabel(names, e.act))
		}
	}
}

// TestPaletteCatalogViewsAreReal asserts the switch-then-run target of an
// entry is a primary view, since 0 is the run-in-place sentinel and the legacy
// sub-view IDs are not switchable destinations.
func TestPaletteCatalogViewsAreReal(t *testing.T) {
	primary := make(map[viewID]struct{}, len(viewOrder))
	for _, v := range viewOrder {
		primary[v] = struct{}{}
	}

	for _, e := range paletteCatalog {
		if e.view == 0 {
			continue
		}
		if _, ok := primary[e.view]; !ok {
			t.Errorf("catalog entry %q targets view %v, which is not a primary view in viewOrder", e.id, e.view)
		}
	}
}

// TestEveryDeclaredActionHasAPaletteDecision walks the action enum itself
// rather than the registry, so an action added to the enum but not yet bound
// to any key is still forced through the same decision.
func TestEveryDeclaredActionHasAPaletteDecision(t *testing.T) {
	names := actionNames(t)
	catalog := paletteCatalogActions()

	for i, name := range names {
		a := keys.Action(i)
		if a == keys.ActionNone {
			continue
		}
		_, inCatalog := catalog[a]
		_, inExcluded := paletteExcluded[a]
		if !inCatalog && !inExcluded {
			t.Errorf("%s is declared in the action enum but is in neither paletteCatalog nor paletteExcluded", name)
		}
	}
}

// TestPaletteDecisionSurfaceIsCatalogAndExcludedOnly asserts the shrinking
// triage map is gone for good. The coverage assertions above would still pass
// with a third bucket reintroduced — they only test the actions that exist
// today — so this walks every source file in the package for the identifier
// instead. A deleted symbol cannot be named in Go, hence the AST scan.
func TestPaletteDecisionSurfaceIsCatalogAndExcludedOnly(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	if len(sources) < 2 {
		t.Fatalf("found %d package sources, expected the whole tui package", len(sources))
	}

	fset := token.NewFileSet()
	for _, path := range sources {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if ok && ident.Name == "palettePendingTriage" {
				t.Errorf("%s: palettePendingTriage is back; a new action must be catalogued or excluded with a reason, not parked", fset.Position(ident.Pos()))
			}
			return true
		})
	}

	names := actionNames(t)
	catalog := paletteCatalogActions()
	if got := len(catalog) + len(paletteExcluded); got != len(names) {
		t.Errorf("catalog (%d) plus excluded (%d) covers %d actions, want all %d: the two sets must partition the enum exactly",
			len(catalog), len(paletteExcluded), got, len(names))
	}
}

// TestPaletteCatalogNamesFollowNamingRules asserts the mechanical parts of the
// naming rules, so an over-long or badly punctuated name fails here rather than
// as a truncated row in a layout scenario.
func TestPaletteCatalogNamesFollowNamingRules(t *testing.T) {
	for _, e := range paletteCatalog {
		if e.name == "" {
			t.Errorf("catalog entry %q has no name", e.id)
			continue
		}
		if w := ansi.StringWidth(e.name); w > maxPaletteNameWidth {
			t.Errorf("catalog entry %q name %q is %d columns, over the %d-column cap", e.id, e.name, w, maxPaletteNameWidth)
		}
		if last, _ := utf8.DecodeLastRuneInString(e.name); strings.ContainsRune(".,:;!?", last) {
			t.Errorf("catalog entry %q name %q ends in punctuation", e.id, e.name)
		}
	}
}

// TestPaletteCategoriesAreKnown asserts every category rendered in the palette
// comes from the closed vocabulary and stays within the 5-column budget the
// category column is sized against.
func TestPaletteCategoriesAreKnown(t *testing.T) {
	for category, meaning := range paletteCategories {
		if w := ansi.StringWidth(category); w > 5 {
			t.Errorf("category %q is %d columns, over the 5-column budget", category, w)
		}
		if meaning == "" {
			t.Errorf("category %q has no description", category)
		}
	}

	for _, c := range paletteTriageModel().BuildPaletteCommands() {
		if c.Category == "" {
			continue
		}
		if _, ok := paletteCategories[c.Category]; !ok {
			t.Errorf("palette command %q uses category %q, which is not in paletteCategories", c.Name, c.Category)
		}
	}
}

// TestSafeDirectCommandIDsExist asserts the fixture-safety allowlist names
// commands that really exist, so a renamed catalog ID cannot silently unlist a
// fixture-safe command.
func TestSafeDirectCommandIDsExist(t *testing.T) {
	built := make(map[string]struct{})
	for _, c := range paletteTriageModel().BuildPaletteCommands() {
		built[c.Key()] = struct{}{}
	}

	for id := range safeDirectCommandIDs() {
		if _, ok := built[id]; !ok {
			t.Errorf("fixture-safe command id %q is not in the built palette command list", id)
		}
	}
}
