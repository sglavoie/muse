package tui

import (
	"go/types"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func markSurfaceType(pkg string, t types.Type, inAPI map[surfaceKey]bool, visited map[types.Type]bool, depth int) {
	if t == nil || depth > 8 || visited[t] {
		return
	}
	visited[t] = true
	switch tt := t.(type) {
	case *types.Alias:
		markSurfaceType(pkg, types.Unalias(tt), inAPI, visited, depth+1)
	case *types.Named:
		if obj := tt.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Exported() {
			inAPI[surfaceKey{pkg: pkg, name: obj.Name()}] = true
		}
		for i := range tt.TypeArgs().Len() {
			markSurfaceType(pkg, tt.TypeArgs().At(i), inAPI, visited, depth+1)
		}
		markSurfaceType(pkg, tt.Underlying(), inAPI, visited, depth+1)
	case *types.Pointer:
		markSurfaceType(pkg, tt.Elem(), inAPI, visited, depth+1)
	case *types.Slice:
		markSurfaceType(pkg, tt.Elem(), inAPI, visited, depth+1)
	case *types.Array:
		markSurfaceType(pkg, tt.Elem(), inAPI, visited, depth+1)
	case *types.Chan:
		markSurfaceType(pkg, tt.Elem(), inAPI, visited, depth+1)
	case *types.Map:
		markSurfaceType(pkg, tt.Key(), inAPI, visited, depth+1)
		markSurfaceType(pkg, tt.Elem(), inAPI, visited, depth+1)
	case *types.Signature:
		for _, tuple := range []*types.Tuple{tt.Params(), tt.Results()} {
			for i := range tuple.Len() {
				markSurfaceType(pkg, tuple.At(i).Type(), inAPI, visited, depth+1)
			}
		}
	case *types.Struct:
		for i := range tt.NumFields() {
			markSurfaceType(pkg, tt.Field(i).Type(), inAPI, visited, depth+1)
		}
	case *types.Interface:
		for i := range tt.NumMethods() {
			markSurfaceType(pkg, tt.Method(i).Type(), inAPI, visited, depth+1)
		}
	}
}

func surfaceNamed(obj types.Object) (*types.Named, bool) {
	tn, ok := obj.(*types.TypeName)
	if !ok {
		return nil, false
	}
	named, ok := types.Unalias(tn.Type()).(*types.Named)
	return named, ok
}

func surfaceOwner(obj types.Object, fieldOwner map[*types.Var]string) string {
	switch o := obj.(type) {
	case *types.Var:
		if o.IsField() {
			return fieldOwner[o]
		}
	case *types.Func:
		sig, ok := o.Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			return ""
		}
		t := sig.Recv().Type()
		if ptr, ok := t.(*types.Pointer); ok {
			t = ptr.Elem()
		}
		if named, ok := types.Unalias(t).(*types.Named); ok {
			return named.Obj().Name()
		}
	}
	return ""
}

// surfaceShortPkg renders an import path the way docs/tui-package-surface.md
// does: the module root is main, the extracted TUI packages are tui/<name>, and
// everything else keeps its internal/ path.
func surfaceShortPkg(path string) string {
	if path == surfaceModulePath {
		return "main"
	}
	short := strings.TrimPrefix(path, surfaceModulePath+"/")
	if rest, ok := strings.CutPrefix(short, "internal/tui/"); ok {
		return "tui/" + rest
	}
	return short
}

type surfaceRow struct {
	pkg         string
	sortRecv    string
	member      int
	name        string
	kind        string
	importers   string
	disposition string
}

// surfaceRows renders one row per exported identifier, and returns the
// identifiers that have no consumer and nothing that justifies the export.
func surfaceRows(decls map[surfaceKey]*surfaceDecl, inAPI map[surfaceKey]bool) ([]surfaceRow, []string) {
	order := map[string]int{}
	for i, path := range surfaceAuditedPackages {
		order[path] = i
	}
	var rows []surfaceRow
	var unjustified []string
	for _, d := range decls {
		disposition, ok := surfaceDisposition(d, inAPI)
		if !ok {
			unjustified = append(unjustified, surfaceShortPkg(d.key.pkg)+"."+d.ident()+" ("+d.kind+")")
		}
		importers := strings.Join(d.importers(), ", ")
		if importers == "" {
			importers = "—"
		}
		row := surfaceRow{
			pkg:         d.key.pkg,
			sortRecv:    d.key.recv,
			member:      1,
			name:        d.key.name,
			kind:        d.kind,
			importers:   importers,
			disposition: disposition,
		}
		if d.key.recv == "" {
			row.sortRecv, row.member = d.key.name, 0
		}
		rows = append(rows, row)
	}
	sort.Strings(unjustified)
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.pkg != b.pkg:
			return order[a.pkg] < order[b.pkg]
		case a.sortRecv != b.sortRecv:
			return a.sortRecv < b.sortRecv
		case a.member != b.member:
			return a.member < b.member
		default:
			return a.name < b.name
		}
	})
	return rows, unjustified
}

// surfaceDisposition states why an identifier is exported. Identifiers with a
// consumer are justified by that consumer; identifiers without one must be
// reachable through an exported signature, through an interface, or through
// encoding/json.
func surfaceDisposition(d *surfaceDecl, inAPI map[surfaceKey]bool) (string, bool) {
	// An identifier every consumer reaches only from _test.go files is exported
	// for a test that lives outside the declaring package, which is a weaker
	// reason than production use and is called out as such.
	testsOnly := len(d.consumers) > 0
	for _, refs := range d.consumers {
		if refs.tests != refs.total {
			testsOnly = false
		}
	}
	suffix := ""
	if testsOnly {
		suffix = " (tests only)"
	}
	switch len(d.consumers) {
	case 0:
	case 1:
		var only string
		for pkg := range d.consumers {
			only = pkg
		}
		switch surfaceShortPkg(only) {
		case "internal/tui":
			return "root" + suffix, true
		case "internal/app":
			return "app" + suffix, true
		case "main":
			return "harness" + suffix, true
		default:
			return "peer" + suffix, true
		}
	default:
		return "multi" + suffix, true
	}
	switch {
	case d.jsonTag:
		return "json", true
	case d.iface != "":
		return "iface: " + d.iface, true
	case d.key.recv == "" && inAPI[surfaceKey{pkg: d.key.pkg, name: d.key.name}]:
		return "api", true
	}
	if why := surfaceManualJustifications[surfaceShortPkg(d.key.pkg)+"|"+d.ident()]; why != "" {
		return "keep: " + why, true
	}
	return "unjustified", false
}

var surfaceSummaryRow = regexp.MustCompile(`^\| ` + "`" + `([^` + "`" + `]+)` + "`" + ` \| (\d+) \| (\d+) \| (\d+) \| (\d+) \|`)

// checkSurfaceSummaryTable keeps the hand-written per-package counts honest: the
// same analysis that generates the inventory must reproduce them exactly.
func checkSurfaceSummaryTable(t *testing.T, doc string, decls map[surfaceKey]*surfaceDecl) {
	t.Helper()

	type counts struct{ exp, multi, single, zero int }
	want := map[string]*counts{}
	for _, path := range surfaceAuditedPackages {
		want[surfaceShortPkg(path)] = &counts{}
	}
	for _, d := range decls {
		c := want[surfaceShortPkg(d.key.pkg)]
		c.exp++
		switch {
		case len(d.consumers) == 0:
			c.zero++
		case len(d.consumers) == 1:
			c.single++
		default:
			c.multi++
		}
	}

	head := strings.Index(doc, surfaceSummaryHead)
	if head < 0 {
		t.Fatalf("%s: no %q section", surfaceDocPath, surfaceSummaryHead)
	}
	section := doc[head:]
	if next := strings.Index(section[len(surfaceSummaryHead):], "\n## "); next >= 0 {
		section = section[:len(surfaceSummaryHead)+next]
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(section, "\n") {
		m := surfaceSummaryRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pkg := m[1]
		c, ok := want[pkg]
		if !ok {
			t.Errorf("%s: %q in the per-package summary is not an audited package", surfaceDocPath, pkg)
			continue
		}
		seen[pkg] = true
		got := counts{atoi(m[2]), atoi(m[3]), atoi(m[4]), atoi(m[5])}
		if got != *c {
			t.Errorf("%s: per-package summary for %s says exp=%d multi=%d single=%d zero=%d, current code has exp=%d multi=%d single=%d zero=%d",
				surfaceDocPath, pkg, got.exp, got.multi, got.single, got.zero, c.exp, c.multi, c.single, c.zero)
		}
	}
	for pkg := range want {
		if !seen[pkg] {
			t.Errorf("%s: the per-package summary has no row for %s", surfaceDocPath, pkg)
		}
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
