package styles_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestNewStyles exercises the NewStyles constructor for both hasDark values,
// checking all three exported fields (HasDark, Dim, Accent).
func TestNewStyles(t *testing.T) {
	for _, hasDark := range []bool{true, false} {
		s := styles.NewStyles(hasDark)

		if s.HasDark != hasDark {
			t.Errorf("NewStyles(%v).HasDark = %v, want %v", hasDark, s.HasDark, hasDark)
		}

		dimOut := s.Dim.Render("x")
		if dimOut == "" {
			t.Errorf("NewStyles(%v).Dim.Render() returned empty string", hasDark)
		}

		accentOut := s.Accent.Render("x")
		if accentOut == "" {
			t.Errorf("NewStyles(%v).Accent.Render() returned empty string", hasDark)
		}
	}
}

// TestStyleNesting verifies that Reverse wrapping Bold output preserves inner
// styling. Bold sequences must survive the outer Reverse render, satisfying
// ac-f4704a90675005f2b2db92c90011a914.
func TestStyleNesting(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	reverse := lipgloss.NewStyle().Reverse(true)

	inner := bold.Render("hello")
	if inner == "hello" {
		t.Fatal("bold.Render did not apply any ANSI sequences to the text")
	}

	outer := reverse.Render(inner)
	if outer == "" {
		t.Fatal("outer render is empty")
	}

	// The bold sequences from inner must be present inside the outer render.
	if !strings.Contains(outer, inner) {
		t.Errorf("outer render does not contain inner bold sequences; outer=%q inner=%q", outer, inner)
	}

	stripped := ansi.Strip(outer)
	if stripped != "hello" {
		t.Errorf("stripped text = %q, want %q", stripped, "hello")
	}

	// Verify that the package's own Accent style round-trips through ansi.Strip,
	// tying this contract test to styles.NewStyles rather than raw lipgloss only.
	accentOut := styles.NewStyles(true).Accent.Render("hello")
	if ansi.Strip(accentOut) != "hello" {
		t.Errorf("ansi.Strip(NewStyles(true).Accent.Render(%q)) = %q, want %q", "hello", ansi.Strip(accentOut), "hello")
	}
}

// TestNoRawANSI walks all .go files under the module root and fails if any file
// contains a raw ANSI escape sequence literal. The scan root is derived from
// this file's path (internal/tui/styles/ → three levels up = module root) so
// the guard covers the entire codebase, not just this package.
//
// Patterns are built at runtime so this source file does not trigger its own
// check.
//
// TODO: Remove the t.Skip once all migration phases are complete and no
// pre-migration files contain raw ANSI escapes (library/highlight.go,
// library/view.go, library/view_test.go, playlists/ansi.go, playlists/view.go,
// tui/views.go). Until then this guard would fail CI on every push.
func TestNoRawANSI(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../internal/tui/styles/styles_test.go; walk three levels up.
	root := filepath.Join(filepath.Dir(thisFile), "../../..")
	t.Logf("TestNoRawANSI: scanning module root %s", root)

	// Build forbidden patterns at runtime so this source file doesn't match itself.
	// Bytes: backslash + "033[" (octal) and backslash + "x1b[" (hex).
	esc := string([]byte{0x5c, 0x30, 0x33, 0x33, 0x5b})
	hex := string([]byte{0x5c, 0x78, 0x31, 0x62, 0x5b})
	forbidden := []string{esc, hex}

	var fileCount int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fileCount++
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(root, path)

		for _, pat := range forbidden {
			if strings.Contains(content, pat) {
				t.Errorf("%s: contains raw ANSI escape sequence (use lipgloss instead)", rel)
				break
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", root, err)
	}

	// Sanity check: ensure we scanned well beyond this package's own files.
	if fileCount <= 2 {
		t.Errorf("only %d .go file(s) scanned; expected more than 2 (root: %s)", fileCount, root)
	}
	t.Logf("TestNoRawANSI: scanned %d .go files", fileCount)
}
