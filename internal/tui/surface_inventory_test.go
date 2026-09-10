package tui

import (
	"fmt"
	"go/types"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// The exported-surface inventory in docs/tui-package-surface.md is generated
// from the type-checked module rather than written by hand, so the audit cannot
// drift away from the code it describes. Regenerate it with:
//
//	MUSE_UPDATE_SURFACE_INVENTORY=1 go test ./internal/tui -run TestExportedSurfaceInventoryCurrent
const (
	surfaceModulePath  = "github.com/sglavoie/muse"
	surfaceDocPath     = "../../docs/tui-package-surface.md"
	surfaceBeginMarker = "<!-- BEGIN GENERATED: per-identifier inventory -->"
	surfaceEndMarker   = "<!-- END GENERATED: per-identifier inventory -->"
	surfaceUpdateEnv   = "MUSE_UPDATE_SURFACE_INVENTORY"
	surfaceSummaryHead = "## Per-package summary"
)

// surfaceAuditedPackages are the packages the TUI decomposition plan extracted
// out of internal/tui, in the order docs/tui-package-surface.md lists them. The
// packages that pre-date the plan are deliberately absent: their surface is
// reported in the "Known remaining surface" section instead of inventoried.
var surfaceAuditedPackages = []string{
	"github.com/sglavoie/muse/internal/desktop",
	"github.com/sglavoie/muse/internal/tui/sorting",
	"github.com/sglavoie/muse/internal/tui/keys",
	"github.com/sglavoie/muse/internal/tui/playback",
	"github.com/sglavoie/muse/internal/tui/overlay",
	"github.com/sglavoie/muse/internal/tui/statusbar",
	"github.com/sglavoie/muse/internal/tui/settings",
	"github.com/sglavoie/muse/internal/tui/smartrules",
	"github.com/sglavoie/muse/internal/tui/metadata",
	"github.com/sglavoie/muse/internal/tui/queue",
	"github.com/sglavoie/muse/internal/tui/browse",
	"github.com/sglavoie/muse/internal/tui/insights",
	"github.com/sglavoie/muse/internal/tui/mixes",
	"github.com/sglavoie/muse/internal/tui/autodj",
	"github.com/sglavoie/muse/internal/tui/uitest",
}

// surfacePreExistingPackages are the packages under internal/tui that pre-date
// the decomposition plan. Their surface is summarised in the "Known remaining
// surface" section of the audit rather than inventoried per identifier. Every
// package under internal/tui must appear here or in surfaceAuditedPackages, so a
// package added later cannot slip past the inventory unnoticed.
var surfacePreExistingPackages = []string{
	"github.com/sglavoie/muse/internal/tui",
	"github.com/sglavoie/muse/internal/tui/albums",
	"github.com/sglavoie/muse/internal/tui/artists",
	"github.com/sglavoie/muse/internal/tui/buckets",
	"github.com/sglavoie/muse/internal/tui/chart",
	"github.com/sglavoie/muse/internal/tui/favorites",
	"github.com/sglavoie/muse/internal/tui/filter",
	"github.com/sglavoie/muse/internal/tui/genres",
	"github.com/sglavoie/muse/internal/tui/layout",
	"github.com/sglavoie/muse/internal/tui/library",
	"github.com/sglavoie/muse/internal/tui/palette",
	"github.com/sglavoie/muse/internal/tui/playlists",
	"github.com/sglavoie/muse/internal/tui/strip",
	"github.com/sglavoie/muse/internal/tui/styles",
	"github.com/sglavoie/muse/internal/tui/tags",
	"github.com/sglavoie/muse/internal/tui/text",
}

// surfaceManualJustifications records a human decision for an exported
// identifier that has no external consumer and no mechanical justification
// (API reachability, interface satisfaction or a JSON tag). Keys are
// "<short package>|<identifier>". An empty map means every export in the
// audited packages is justified by the code itself.
var surfaceManualJustifications = map[string]string{}

type surfaceKey struct {
	pkg  string // declaring package path
	recv string // owning type name for methods and fields, empty otherwise
	name string
}

type surfaceRefs struct {
	total int
	tests int // references made from _test.go files
}

type surfaceDecl struct {
	key       surfaceKey
	kind      string // type, func, const, var, method, field
	obj       types.Object
	jsonTag   bool
	iface     string // interface the receiver satisfies, for methods
	consumers map[string]*surfaceRefs
}

// importers returns the consuming packages in short form, sorted, with a dagger
// on consumers that only reach the identifier from their test files.
func (d *surfaceDecl) importers() []string {
	out := make([]string, 0, len(d.consumers))
	for pkg, refs := range d.consumers {
		name := surfaceShortPkg(pkg)
		if refs.tests == refs.total {
			name += "†"
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (d *surfaceDecl) ident() string {
	if d.key.recv == "" {
		return d.key.name
	}
	return d.key.recv + "." + d.key.name
}

func TestExportedSurfaceInventoryCurrent(t *testing.T) {
	decls, inAPI := analyzeExportedSurface(t)

	rows, unjustified := surfaceRows(decls, inAPI)
	if len(unjustified) > 0 {
		t.Errorf("%d exported identifier(s) have no external consumer and no justification.\n"+
			"Unexport them, or record why they must stay exported in surfaceManualJustifications:\n  %s",
			len(unjustified), strings.Join(unjustified, "\n  "))
	}

	doc, err := os.ReadFile(surfaceDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", surfaceDocPath, err)
	}
	checkSurfaceSummaryTable(t, string(doc), decls)

	want := renderSurfaceInventory(rows)
	got, err := surfaceGeneratedBlock(string(doc))
	if err != nil {
		t.Fatalf("%s: %v", surfaceDocPath, err)
	}
	if got == want {
		return
	}
	if os.Getenv(surfaceUpdateEnv) == "1" {
		updated, err := replaceSurfaceBlock(string(doc), want)
		if err != nil {
			t.Fatalf("%s: %v", surfaceDocPath, err)
		}
		//nolint:gosec // surfaceDocPath is a constant path to a doc in this repository.
		if err := os.WriteFile(surfaceDocPath, []byte(updated), 0o644); err != nil {
			t.Fatalf("write %s: %v", surfaceDocPath, err)
		}
		t.Fatalf("%s regenerated; rerun without %s=1", surfaceDocPath, surfaceUpdateEnv)
	}
	t.Errorf("%s is stale: the committed inventory no longer matches the exported\n"+
		"declarations and uses in the module.\n%s\nRegenerate with:\n"+
		"\t%s=1 go test ./internal/tui -run TestExportedSurfaceInventoryCurrent",
		surfaceDocPath, surfaceLineDiff(got, want), surfaceUpdateEnv)
}

// analyzeExportedSurface type-checks the whole module (test files included) and
// returns every exported identifier of the audited packages with the set of
// packages that reference it, plus the set of package-level names that are
// reachable through some consumed exported signature.
func analyzeExportedSurface(t *testing.T) (map[surfaceKey]*surfaceDecl, map[surfaceKey]bool) {
	t.Helper()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir:   "../..",
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, surfaceModulePath+"/...")
	if err != nil {
		t.Fatalf("load packages: %v", err)
	}
	var loadErrs []string
	for _, p := range pkgs {
		for _, e := range p.Errors {
			loadErrs = append(loadErrs, p.ID+": "+e.Error())
		}
	}
	if len(loadErrs) > 0 {
		t.Fatalf("the module does not type-check:\n  %s", strings.Join(loadErrs, "\n  "))
	}

	audited := make(map[string]bool, len(surfaceAuditedPackages))
	for _, p := range surfaceAuditedPackages {
		audited[p] = true
	}

	decls := map[surfaceKey]*surfaceDecl{}
	fieldOwner := map[*types.Var]string{}
	seen := map[string]bool{}
	for _, p := range pkgs {
		if p.Types == nil || !strings.HasPrefix(p.PkgPath, surfaceModulePath) {
			continue
		}
		collectFieldOwners(p.Types.Scope(), fieldOwner)
		if !audited[p.PkgPath] || seen[p.PkgPath] {
			continue
		}
		seen[p.PkgPath] = true
		collectExportedDecls(p.PkgPath, p.Types.Scope(), decls)
	}
	for _, path := range surfaceAuditedPackages {
		if !seen[path] {
			t.Fatalf("audited package %s was not loaded; update surfaceAuditedPackages", path)
		}
	}
	checkSurfaceCoverage(t, pkgs)

	recordSurfaceUses(pkgs, decls, fieldOwner)
	markSurfaceInterfaces(pkgs, decls)
	return decls, markSurfaceAPI(decls)
}

// checkSurfaceCoverage fails when a package under internal/tui is neither
// inventoried nor recorded as pre-existing, so the audit cannot quietly stop
// covering the tree it claims to describe.
func checkSurfaceCoverage(t *testing.T, pkgs []*packages.Package) {
	t.Helper()

	classified := make(map[string]bool, len(surfaceAuditedPackages)+len(surfacePreExistingPackages))
	for _, path := range surfaceAuditedPackages {
		classified[path] = true
	}
	for _, path := range surfacePreExistingPackages {
		classified[path] = true
	}
	reported := map[string]bool{}
	for _, p := range pkgs {
		path := p.PkgPath
		if !strings.HasPrefix(path, surfaceModulePath+"/internal/tui") ||
			strings.HasSuffix(path, "_test") || strings.HasSuffix(path, ".test") ||
			classified[path] || reported[path] {
			continue
		}
		reported[path] = true
		t.Errorf("package %s is under internal/tui but appears in neither surfaceAuditedPackages "+
			"nor surfacePreExistingPackages: inventory it or record it as out of scope", path)
	}
}

func collectFieldOwners(scope *types.Scope, owners map[*types.Var]string) {
	for _, name := range scope.Names() {
		named, ok := surfaceNamed(scope.Lookup(name))
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for i := range st.NumFields() {
			owners[st.Field(i)] = name
		}
	}
}

func collectExportedDecls(pkgPath string, scope *types.Scope, decls map[surfaceKey]*surfaceDecl) {
	add := func(recv, name, kind string, obj types.Object, jsonTag bool) {
		key := surfaceKey{pkg: pkgPath, recv: recv, name: name}
		decls[key] = &surfaceDecl{key: key, kind: kind, obj: obj, jsonTag: jsonTag, consumers: map[string]*surfaceRefs{}}
	}
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		kind := "var"
		switch obj.(type) {
		case *types.Func:
			kind = "func"
		case *types.TypeName:
			kind = "type"
		case *types.Const:
			kind = "const"
		}
		add("", name, kind, obj, false)

		named, ok := surfaceNamed(obj)
		if !ok {
			continue
		}
		for i := range named.NumMethods() {
			if m := named.Method(i); m.Exported() {
				add(name, m.Name(), "method", m, false)
			}
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for i := range st.NumFields() {
			f := st.Field(i)
			if !f.Exported() {
				continue
			}
			add(name, f.Name(), "field", f, reflect.StructTag(st.Tag(i)).Get("json") != "")
		}
	}
}

// recordSurfaceUses attributes every reference to an audited export to the
// package that makes it. A package's own references — including the ones in its
// own test files — are not importers: otherwise a test could keep dead surface
// alive forever. Non-test files are only visited through the plain package, so
// the test variant of a package does not double-count them.
func recordSurfaceUses(pkgs []*packages.Package, decls map[surfaceKey]*surfaceDecl, fieldOwner map[*types.Var]string) {
	for _, p := range pkgs {
		if p.TypesInfo == nil || strings.HasSuffix(p.PkgPath, ".test") {
			continue
		}
		consumer := strings.TrimSuffix(p.PkgPath, "_test")
		if !strings.HasPrefix(consumer, surfaceModulePath) {
			continue
		}
		isVariant := p.ID != p.PkgPath
		for ident, obj := range p.TypesInfo.Uses {
			if obj == nil || obj.Pkg() == nil || !obj.Exported() {
				continue
			}
			declPkg := obj.Pkg().Path()
			if declPkg == consumer {
				continue
			}
			inTestFile := strings.HasSuffix(p.Fset.Position(ident.Pos()).Filename, "_test.go")
			if isVariant && !inTestFile {
				continue
			}
			d := decls[surfaceKey{pkg: declPkg, recv: surfaceOwner(obj, fieldOwner), name: obj.Name()}]
			if d == nil {
				continue
			}
			refs := d.consumers[consumer]
			if refs == nil {
				refs = &surfaceRefs{}
				d.consumers[consumer] = refs
			}
			refs.total++
			if inTestFile {
				refs.tests++
			}
		}
	}
}

// markSurfaceInterfaces flags exported methods that are never called by name but
// are reached through an interface. Only interfaces mentioned somewhere in the
// module count, and only when the interface declares a method of that name and
// the receiver implements it.
func markSurfaceInterfaces(pkgs []*packages.Package, decls map[surfaceKey]*surfaceDecl) {
	type namedIface struct {
		name string
		it   *types.Interface
	}
	// The same interface is met both through its type name and as a bare
	// interface type; keep the name, and the lexically first one when a type is
	// aliased, so the reported justification does not depend on map order.
	names := map[*types.Interface]string{}
	add := func(t types.Type) {
		if t == nil {
			return
		}
		it, ok := t.Underlying().(*types.Interface)
		if !ok || it.NumMethods() == 0 {
			return
		}
		name := ""
		if n, ok := types.Unalias(t).(*types.Named); ok && n.Obj().Pkg() != nil {
			name = surfaceShortPkg(n.Obj().Pkg().Path()) + "." + n.Obj().Name()
		}
		switch existing, known := names[it]; {
		case !known, existing == "" && name != "", existing != "" && name != "" && name < existing:
			names[it] = name
		}
	}
	for _, p := range pkgs {
		if p.TypesInfo == nil || !strings.HasPrefix(p.PkgPath, surfaceModulePath) {
			continue
		}
		for _, tv := range p.TypesInfo.Types {
			add(tv.Type)
		}
		for _, obj := range p.TypesInfo.Defs {
			if obj != nil {
				add(obj.Type())
			}
		}
		for _, obj := range p.TypesInfo.Uses {
			if obj != nil {
				add(obj.Type())
			}
		}
	}
	ifaces := make([]namedIface, 0, len(names))
	for it, name := range names {
		if name == "" {
			name = "anonymous interface"
		}
		ifaces = append(ifaces, namedIface{name: name, it: it})
	}
	// Named interfaces first: they are the ones worth naming as the reason a
	// method stays exported.
	sort.Slice(ifaces, func(i, j int) bool {
		a, b := ifaces[i].name, ifaces[j].name
		if (a == "anonymous interface") != (b == "anonymous interface") {
			return b == "anonymous interface"
		}
		return a < b
	})

	for _, d := range decls {
		if d.kind != "method" || len(d.consumers) > 0 {
			continue
		}
		fn, ok := d.obj.(*types.Func)
		if !ok {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Recv() == nil {
			continue
		}
		recv := sig.Recv().Type()
		ptr := types.NewPointer(recv)
		for _, candidate := range ifaces {
			if candidate.it.NumMethods() == 0 || !surfaceIfaceHasMethod(candidate.it, d.key.name) {
				continue
			}
			if types.Implements(recv, candidate.it) || types.Implements(ptr, candidate.it) {
				d.iface = candidate.name
				break
			}
		}
	}
}

func surfaceIfaceHasMethod(it *types.Interface, name string) bool {
	for i := range it.NumMethods() {
		if it.Method(i).Name() == name {
			return true
		}
	}
	return false
}

// markSurfaceAPI returns the package-level names that a caller cannot avoid
// naming: every type reachable from the type of an identifier that does have an
// external consumer, and from the methods of such a type.
func markSurfaceAPI(decls map[surfaceKey]*surfaceDecl) map[surfaceKey]bool {
	inAPI := map[surfaceKey]bool{}
	for _, d := range decls {
		if len(d.consumers) == 0 {
			continue
		}
		markSurfaceType(d.key.pkg, d.obj.Type(), inAPI, map[types.Type]bool{}, 0)
		named, ok := surfaceNamed(d.obj)
		if !ok {
			continue
		}
		for i := range named.NumMethods() {
			if m := named.Method(i); m.Exported() {
				markSurfaceType(d.key.pkg, m.Type(), inAPI, map[types.Type]bool{}, 0)
			}
		}
	}
	return inAPI
}

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

func renderSurfaceInventory(rows []surfaceRow) string {
	var b strings.Builder
	b.WriteString(surfaceTotals(rows) + "\n\n")
	b.WriteString("| Package | Identifier | Kind | Importers | Disposition |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range rows {
		name := r.name
		if r.member == 1 {
			name = r.sortRecv + "." + r.name
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | %s |\n",
			surfaceShortPkg(r.pkg), name, r.kind, surfaceMarkupPkgs(r.importers), r.disposition)
	}
	return strings.TrimRight(b.String(), "\n")
}

// surfaceTotals is the one-line roll-up that opens the generated block, so the
// shape of the surface is readable without counting 800 rows by hand.
func surfaceTotals(rows []surfaceRow) string {
	var multi, single, testsOnly int
	zero := map[string]int{}
	for _, r := range rows {
		base, isTestsOnly := strings.CutSuffix(r.disposition, " (tests only)")
		if isTestsOnly {
			testsOnly++
		}
		switch {
		case base == "multi":
			multi++
		case base == "api", base == "json", strings.HasPrefix(base, "iface: "), strings.HasPrefix(base, "keep: "):
			zero[strings.SplitN(base, ":", 2)[0]]++
		default:
			single++
		}
	}
	kinds := make([]string, 0, len(zero))
	for _, kind := range []string{"api", "iface", "json", "keep"} {
		if n := zero[kind]; n > 0 {
			kinds = append(kinds, fmt.Sprintf("%d `%s`", n, kind))
		}
	}
	total := multi + single
	for _, n := range zero {
		total += n
	}
	return fmt.Sprintf("**%d exported identifiers across %d packages**: %d reached from two or more "+
		"packages, %d from exactly one, %d from none (%s). %d of the reached identifiers are named "+
		"only by `_test.go` files outside their own package.",
		total, len(surfaceAuditedPackages), multi, single, total-multi-single, strings.Join(kinds, ", "), testsOnly)
}

// surfaceMarkupPkgs code-quotes each package name in a comma-separated list.
func surfaceMarkupPkgs(list string) string {
	if list == "—" {
		return list
	}
	parts := strings.Split(list, ", ")
	for i, p := range parts {
		parts[i] = "`" + p + "`"
	}
	return strings.Join(parts, ", ")
}

func surfaceGeneratedBlock(doc string) (string, error) {
	start := strings.Index(doc, surfaceBeginMarker)
	end := strings.Index(doc, surfaceEndMarker)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("missing %s / %s markers around the generated inventory", surfaceBeginMarker, surfaceEndMarker)
	}
	return strings.TrimSpace(doc[start+len(surfaceBeginMarker) : end]), nil
}

func replaceSurfaceBlock(doc, block string) (string, error) {
	start := strings.Index(doc, surfaceBeginMarker)
	end := strings.Index(doc, surfaceEndMarker)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("missing %s / %s markers around the generated inventory", surfaceBeginMarker, surfaceEndMarker)
	}
	return doc[:start+len(surfaceBeginMarker)] + "\n\n" + block + "\n\n" + doc[end:], nil
}

// surfaceLineDiff reports the first differing lines plus the size change, which
// is enough to see what moved without dumping an 800-row table into the log.
func surfaceLineDiff(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "committed rows: %d, current rows: %d\n", len(gotLines), len(wantLines))
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
