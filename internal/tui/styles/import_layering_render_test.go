package styles_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

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
