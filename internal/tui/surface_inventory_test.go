package tui

import (
	"go/types"
	"os"
	"reflect"
	"sort"
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
