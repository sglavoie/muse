package styles_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNoRawANSI walks all .go files in this package and fails if any file
// contains a raw ANSI escape sequence literal.
//
// Patterns are built at runtime so this source file does not trigger its own
// check.
func TestNoRawANSI(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	stylesDir := filepath.Dir(thisFile)
	t.Logf("TestNoRawANSI: scanning styles dir %s", stylesDir)

	// Build forbidden patterns at runtime so this source file doesn't match itself.
	// Bytes: backslash + "033[" (octal) and backslash + "x1b[" (hex).
	esc := string([]byte{0x5c, 0x30, 0x33, 0x33, 0x5b})
	hex := string([]byte{0x5c, 0x78, 0x31, 0x62, 0x5b})
	forbidden := []string{esc, hex}

	var fileCount int
	err := filepath.WalkDir(stylesDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fileCount++
		src, readErr := os.ReadFile(path) //nolint:gosec // G122: test walks its own tree, no symlink risk
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(stylesDir, path)

		for _, pat := range forbidden {
			if strings.Contains(content, pat) {
				t.Errorf("%s: contains raw ANSI escape sequence (use lipgloss instead)", rel)
				break
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", stylesDir, err)
	}

	if fileCount == 0 {
		t.Errorf("no .go files scanned in %s", stylesDir)
	}
	t.Logf("TestNoRawANSI: scanned %d .go files", fileCount)
}

// isStdlibImport returns true for standard library import paths. Standard
// library paths have no dot in the first path segment (e.g. "fmt", "os",
// "go/parser"), while all third-party modules start with a hostname.
func isStdlibImport(path string) bool {
	first := strings.SplitN(path, "/", 2)[0]
	return !strings.Contains(first, ".")
}

// TestStylesPackageImportIsolation scans non-test .go files in
// internal/tui/styles/ and fails if:
//   - any import path contains "internal/tui/" (upward TUI dependency), or
//   - any import is not from the standard library and is not charm.land/lipgloss/v2
//
// This keeps the styles package a pure leaf that only depends on stdlib and
// the single charmbracelet styling library.
func TestStylesPackageImportIsolation(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	stylesDir := filepath.Dir(thisFile)
	t.Logf("TestStylesPackageImportIsolation: scanning styles dir %s", stylesDir)

	entries, err := os.ReadDir(stylesDir)
	if err != nil {
		t.Fatalf("cannot read styles dir %s: %v", stylesDir, err)
	}

	fset := token.NewFileSet()
	var scanned int

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(stylesDir, name)
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("cannot parse %s: %v", name, parseErr)
		}
		scanned++

		for _, imp := range f.Imports {
			// imp.Path.Value includes surrounding quotes, e.g. `"internal/tui/library"`
			impPath := strings.Trim(imp.Path.Value, `"`)

			if strings.Contains(impPath, "internal/tui/") {
				t.Errorf("%s: forbidden import %q (styles must not import from internal/tui/)", name, impPath)
				continue
			}

			if !isStdlibImport(impPath) && impPath != "charm.land/lipgloss/v2" {
				t.Errorf("%s: non-whitelisted import %q (only stdlib and charm.land/lipgloss/v2 are allowed)", name, impPath)
			}
		}
	}

	if scanned == 0 {
		t.Fatalf("no non-test .go files found in %s", stylesDir)
	}
	t.Logf("TestStylesPackageImportIsolation: scanned %d non-test file(s)", scanned)
}

// TestNoLipglossRenderHelpers walks this package's .go files and fails if any
// banned lipgloss print-helper calls — lipgloss.Println, lipgloss.Sprint, or
// lipgloss.Fprintln — appear in production code. Only lipgloss.Style.Render()
// is permitted for styled output in View() methods.
func TestNoLipglossRenderHelpers(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	stylesDir := filepath.Dir(thisFile)
	t.Logf("TestNoLipglossRenderHelpers: scanning %s", stylesDir)

	forbidden := []string{
		"lipgloss.Println",
		"lipgloss.Sprint",
		"lipgloss.Fprintln",
	}

	var fileCount int
	err := filepath.WalkDir(stylesDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileCount++
		src, readErr := os.ReadFile(path) //nolint:gosec // G122: test walks its own tree, no symlink risk
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(stylesDir, path)
		for _, call := range forbidden {
			if strings.Contains(content, call) {
				t.Errorf("%s: forbidden call %q found (use lipgloss.Style.Render() instead)", rel, call)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", stylesDir, err)
	}

	if fileCount == 0 {
		t.Fatalf("no non-test .go files found under %s", stylesDir)
	}
	t.Logf("TestNoLipglossRenderHelpers: scanned %d non-test file(s)", fileCount)
}
