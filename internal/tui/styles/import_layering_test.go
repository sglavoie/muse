package styles_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The layering contract of the packages the TUI decomposition produced lives in
// docs/tui-package-surface.md and is generated from `go list` rather than drawn
// by hand, so the architecture assessment cannot drift away from the graph the
// compiler actually builds. Regenerate the table with:
//
//	MUSE_UPDATE_TUI_LAYERING=1 go test ./internal/tui/styles -run TestPackageLayersMatchImportGraph
//
// These checks live in the lowest package of the graph on purpose: this file is
// an external test (package styles_test) that imports nothing from the module,
// so the guard cannot add an edge to the graph it describes.
const (
	layerModule    = "github.com/sglavoie/muse"
	layerDocRel    = "docs/tui-package-surface.md"
	layerBegin     = "<!-- BEGIN GENERATED: package layers -->"
	layerEnd       = "<!-- END GENERATED: package layers -->"
	layerUpdateEnv = "MUSE_UPDATE_TUI_LAYERING"
	layerRegenCmd  = "MUSE_UPDATE_TUI_LAYERING=1 go test ./internal/tui/styles -run TestPackageLayersMatchImportGraph"
	layerListFmt   = `{{.ImportPath}}|{{join .Imports " "}}|{{join .TestImports " "}}|{{join .XTestImports " "}}`
)

// layerGraph is the direct import graph of the packages the TUI owns: main,
// internal/app, internal/desktop, internal/tui and every internal/tui/*
// sub-package. Everything else in the module (internal/library, internal/db,
// …) sits underneath the whole graph and is deliberately out of scope.
type layerGraph struct {
	nodes       []string            // in-scope packages, short names, sorted
	imports     map[string][]string // in-scope non-test edges, sorted
	testImports map[string][]string // in-scope edges added by _test.go files, sorted
	outside     map[string][]string // out-of-scope package -> in-scope packages it imports
	levels      map[string]int      // assigned from imports; see layerGraph.assignLevels
}

// TestBuildNoImportCycles proves the graph documented in
// docs/tui-package-surface.md is acyclic, in the two ways it can be broken.
//
// The built graph is covered by loading it: the Go toolchain rejects an import
// cycle between packages outright, and `go list` refuses to report a graph that
// has one — it prints the offending chain and exits non-zero instead — so
// getting a graph at all is the proof. Test files are the gap. An external test
// package (`package foo_test`) may import a package that imports foo, which the
// toolchain accepts and which still makes the layering claim false. The walk
// below covers that half and reports the offending path.
func TestBuildNoImportCycles(t *testing.T) {
	g := loadLayerGraph(t)
	combined := make(map[string][]string, len(g.nodes))
	for _, n := range g.nodes {
		seen := map[string]bool{}
		for _, list := range [][]string{g.imports[n], g.testImports[n]} {
			for _, dep := range list {
				if dep != n && !seen[dep] {
					seen[dep] = true
					combined[n] = append(combined[n], dep)
				}
			}
		}
		sort.Strings(combined[n])
	}
	if cycle := layerFindCycle(g.nodes, combined); cycle != nil {
		t.Fatalf("import cycle across %d packages (test imports included):\n\t%s",
			len(cycle)-1, strings.Join(cycle, " -> "))
	}
}

// TestPackageLayersMatchImportGraph enforces the layering contract stated in
// the "Layering" section of docs/tui-package-surface.md and regenerates the
// table that section publishes.
func TestPackageLayersMatchImportGraph(t *testing.T) {
	g := loadLayerGraph(t)
	if err := g.assignLevels(); err != nil {
		t.Fatalf("cannot assign package levels: %v", err)
	}

	checkLayerDescends(t, g)
	checkLayerRootImporters(t, g)
	checkLayerScopeClosed(t, g)

	docPath := filepath.Join(layerRepoRoot(t), layerDocRel)
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", layerDocRel, err)
	}

	want := renderLayerBlock(g)
	got, err := layerGeneratedBlock(string(doc))
	if err != nil {
		t.Fatalf("%s: %v", layerDocRel, err)
	}
	if got == want {
		return
	}
	if os.Getenv(layerUpdateEnv) == "1" {
		updated, err := replaceLayerBlock(string(doc), want)
		if err != nil {
			t.Fatalf("%s: %v", layerDocRel, err)
		}
		//nolint:gosec // layerDocRel is a constant path to a doc in this repository.
		if err := os.WriteFile(docPath, []byte(updated), 0o644); err != nil {
			t.Fatalf("write %s: %v", layerDocRel, err)
		}
		t.Fatalf("%s regenerated; rerun without %s=1", layerDocRel, layerUpdateEnv)
	}
	t.Errorf("%s is stale: the committed layering no longer matches the direct\n"+
		"imports reported by go list.\n%s\nRegenerate with:\n\t%s",
		layerDocRel, layerLineDiff(got, want), layerRegenCmd)
}

// checkLayerDescends is the contract itself: an import must go from a package
// at one level to a package at a strictly lower level. Non-test edges satisfy
// it by construction — assignLevels derives the level from them — so the
// substantive half is the test edges, which are checked against the levels the
// built graph produced.
func checkLayerDescends(t *testing.T, g *layerGraph) {
	t.Helper()

	for _, n := range g.nodes {
		for _, dep := range g.imports[n] {
			if g.levels[n] <= g.levels[dep] {
				t.Errorf("import %s (L%d) -> %s (L%d) does not descend a level",
					n, g.levels[n], dep, g.levels[dep])
			}
		}
		for _, dep := range g.testImports[n] {
			if dep == n {
				continue
			}
			if g.levels[n] <= g.levels[dep] {
				t.Errorf("test files of %s (L%d) import %s (L%d), which does not descend a level;\n"+
					"the toolchain allows this but it breaks the layering the audit documents",
					n, g.levels[n], dep, g.levels[dep])
			}
		}
	}
}

// checkLayerRootImporters keeps the internal/tui <-> sub-package relationship
// one-way: internal/tui owns the root Model and imports the view packages, so a
// view package importing it back would invert the architecture. tui/uitest is
// the documented exception — it drives the root model as a test harness and
// nothing in internal/tui references it.
func checkLayerRootImporters(t *testing.T, g *layerGraph) {
	t.Helper()

	const root, harness = "internal/tui", "tui/uitest"
	for _, n := range g.nodes {
		if !strings.HasPrefix(n, "tui/") || n == harness {
			continue
		}
		for _, list := range [][]string{g.imports[n], g.testImports[n]} {
			for _, dep := range list {
				if dep == root {
					t.Errorf("%s imports %s; only %s may depend on the root model package",
						n, root, harness)
				}
			}
		}
	}
}

// checkLayerScopeClosed keeps the graph in the audit complete: no package
// outside it may reach into internal/tui/*, so the documented importer sets are
// the whole truth rather than a subset.
func checkLayerScopeClosed(t *testing.T, g *layerGraph) {
	t.Helper()

	outsiders := make([]string, 0, len(g.outside))
	for pkg := range g.outside {
		outsiders = append(outsiders, pkg)
	}
	sort.Strings(outsiders)
	for _, pkg := range outsiders {
		t.Errorf("%s is outside the documented graph but imports %s",
			pkg, strings.Join(g.outside[pkg], ", "))
	}
}

// loadLayerGraph reads the direct imports of every package in the module from
// go list and keeps the edges between the packages the audit covers.
func loadLayerGraph(t *testing.T) *layerGraph {
	t.Helper()

	cmd := exec.Command("go", "list", "-f", layerListFmt, layerModule+"/...")
	cmd.Dir = layerRepoRoot(t)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("go list failed: %v\nAn import cycle in the module is reported here:\n%s",
				err, stderr.String())
		}
		t.Fatalf("go list failed: %v", err)
	}

	g := &layerGraph{
		imports:     map[string][]string{},
		testImports: map[string][]string{},
		outside:     map[string][]string{},
		levels:      map[string]int{},
	}
	type record struct{ imports, testImports []string }
	records := map[string]record{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.SplitN(line, "|", 4)
		if len(fields) != 4 {
			t.Fatalf("unexpected go list line: %q", line)
		}
		pkg := layerShortName(fields[0])
		rec := record{imports: layerScoped(fields[1])}
		rec.testImports = layerScoped(fields[2] + " " + fields[3])
		records[pkg] = rec
		if layerInScope(pkg) {
			g.nodes = append(g.nodes, pkg)
		}
	}
	if len(g.nodes) == 0 {
		t.Fatalf("go list reported no packages under %s", layerModule)
	}
	sort.Strings(g.nodes)

	for pkg, rec := range records {
		if !layerInScope(pkg) {
			outside := append(append([]string{}, rec.imports...), rec.testImports...)
			if len(outside) > 0 {
				g.outside[pkg] = layerDedupe(outside)
			}
			continue
		}
		g.imports[pkg] = rec.imports
		g.testImports[pkg] = rec.testImports
	}
	return g
}

// assignLevels gives every package the only level consistent with a strictly
// descending graph: a package that imports nothing else in the graph is L0, and
// every other package sits one level above the highest package it imports. Such
// an assignment exists exactly when the graph is acyclic, so a cycle is
// reported as an error instead of looping forever.
func (g *layerGraph) assignLevels() error {
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(g.nodes))

	var visit func(string, []string) (int, error)
	visit = func(n string, path []string) (int, error) {
		switch state[n] {
		case done:
			return g.levels[n], nil
		case visiting:
			return 0, fmt.Errorf("import cycle: %s", strings.Join(append(path, n), " -> "))
		}
		state[n] = visiting
		level := 0
		for _, dep := range g.imports[n] {
			depLevel, err := visit(dep, append(path, n))
			if err != nil {
				return 0, err
			}
			if depLevel+1 > level {
				level = depLevel + 1
			}
		}
		state[n] = done
		g.levels[n] = level
		return level, nil
	}

	for _, n := range g.nodes {
		if _, err := visit(n, nil); err != nil {
			return err
		}
	}
	return nil
}

// layerFindCycle returns a cycle as a path whose first and last element are the
// same package, or nil when the graph is acyclic.
func layerFindCycle(nodes []string, edges map[string][]string) []string {
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(nodes))

	var walk func(string, []string) []string
	walk = func(n string, path []string) []string {
		switch state[n] {
		case done:
			return nil
		case visiting:
			for i, p := range path {
				if p == n {
					return append(append([]string{}, path[i:]...), n)
				}
			}
			return []string{n, n}
		}
		state[n] = visiting
		for _, dep := range edges[n] {
			if cycle := walk(dep, append(path, n)); cycle != nil {
				return cycle
			}
		}
		state[n] = done
		return nil
	}

	for _, n := range nodes {
		if cycle := walk(n, nil); cycle != nil {
			return cycle
		}
	}
	return nil
}

func renderLayerBlock(g *layerGraph) string {
	edges, top := 0, 0
	byLevel := map[int][]string{}
	for _, n := range g.nodes {
		edges += len(g.imports[n])
		level := g.levels[n]
		byLevel[level] = append(byLevel[level], n)
		if level > top {
			top = level
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%d packages, %d direct imports between them, %d levels (L0–L%d).** "+
		"Levels are read off the graph, not assigned by hand: a package that imports nothing "+
		"else in the table is L0, and every other package sits one level above the highest "+
		"package it imports.\n\n", len(g.nodes), edges, top+1, top)

	b.WriteString("| Level | Packages |\n|---:|---|\n")
	for level := top; level >= 0; level-- {
		fmt.Fprintf(&b, "| L%d | %s |\n", level, layerMarkup(byLevel[level]))
	}

	ordered := append([]string{}, g.nodes...)
	sort.Slice(ordered, func(i, j int) bool {
		if g.levels[ordered[i]] != g.levels[ordered[j]] {
			return g.levels[ordered[i]] > g.levels[ordered[j]]
		}
		return ordered[i] < ordered[j]
	})

	b.WriteString("\n| Package | Level | Direct imports within the graph |\n|---|---:|---|\n")
	for _, n := range ordered {
		deps := append([]string{}, g.imports[n]...)
		sort.Slice(deps, func(i, j int) bool {
			if g.levels[deps[i]] != g.levels[deps[j]] {
				return g.levels[deps[i]] > g.levels[deps[j]]
			}
			return deps[i] < deps[j]
		})
		cells := make([]string, 0, len(deps))
		for _, dep := range deps {
			cells = append(cells, fmt.Sprintf("`%s` (L%d)", dep, g.levels[dep]))
		}
		list := "—"
		if len(cells) > 0 {
			list = strings.Join(cells, ", ")
		}
		fmt.Fprintf(&b, "| `%s` | L%d | %s |\n", n, g.levels[n], list)
	}
	return strings.TrimRight(b.String(), "\n")
}

func layerMarkup(pkgs []string) string {
	if len(pkgs) == 0 {
		return "—"
	}
	sorted := append([]string{}, pkgs...)
	sort.Strings(sorted)
	for i, p := range sorted {
		sorted[i] = "`" + p + "`"
	}
	return strings.Join(sorted, ", ")
}

func layerGeneratedBlock(doc string) (string, error) {
	start := strings.Index(doc, layerBegin)
	end := strings.Index(doc, layerEnd)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("missing %s / %s markers around the generated layer table", layerBegin, layerEnd)
	}
	return strings.TrimSpace(doc[start+len(layerBegin) : end]), nil
}

func replaceLayerBlock(doc, block string) (string, error) {
	start := strings.Index(doc, layerBegin)
	end := strings.Index(doc, layerEnd)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("missing %s / %s markers around the generated layer table", layerBegin, layerEnd)
	}
	return doc[:start+len(layerBegin)] + "\n\n" + block + "\n\n" + doc[end:], nil
}

// layerLineDiff reports the differing lines so a stale table can be read from
// the failure without diffing the document by hand.
func layerLineDiff(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "committed lines: %d, current lines: %d\n", len(gotLines), len(wantLines))
	shown := 0
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g == w {
			continue
		}
		if shown == 10 {
			b.WriteString("  ... (further differences omitted)\n")
			break
		}
		fmt.Fprintf(&b, "  line %d:\n    committed: %s\n    current:   %s\n", i+1, g, w)
		shown++
	}
	return strings.TrimRight(b.String(), "\n")
}

// layerScoped turns a space-separated import list from go list into the sorted
// short names of the in-scope packages it names.
func layerScoped(list string) []string {
	var out []string
	for _, path := range strings.Fields(list) {
		if !strings.HasPrefix(path, layerModule) {
			continue
		}
		if short := layerShortName(path); layerInScope(short) {
			out = append(out, short)
		}
	}
	return layerDedupe(out)
}

func layerDedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

// layerShortName is the naming convention docs/tui-package-surface.md uses:
// the module root is "main" and internal/tui sub-packages lose the "internal/"
// prefix, everything else keeps its module-relative path.
func layerShortName(path string) string {
	switch {
	case path == layerModule:
		return "main"
	case strings.HasPrefix(path, layerModule+"/internal/tui/"):
		return "tui/" + strings.TrimPrefix(path, layerModule+"/internal/tui/")
	case strings.HasPrefix(path, layerModule+"/"):
		return strings.TrimPrefix(path, layerModule+"/")
	}
	return path
}

func layerInScope(short string) bool {
	switch short {
	case "main", "internal/app", "internal/desktop", "internal/tui":
		return true
	}
	return strings.HasPrefix(short, "tui/")
}

func layerRepoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}
