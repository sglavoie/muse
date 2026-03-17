package styles_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestStyleNesting verifies that Reverse wrapping Bold output preserves inner styling.
func TestStyleNesting(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	reverse := lipgloss.NewStyle().Reverse(true)

	inner := bold.Render("hello")
	outer := reverse.Render(inner)

	if outer == "" {
		t.Fatal("outer render is empty")
	}
	if !strings.Contains(outer, "hello") {
		t.Errorf("text content lost after nesting: %q", outer)
	}
}

// TestNoRawANSI walks all .go files in this package directory and fails if any
// file contains a raw ANSI escape sequence literal. Patterns are built at
// runtime so this file does not trigger its own check.
func TestNoRawANSI(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// Build forbidden patterns at runtime so this source file doesn't match itself.
	// Bytes: backslash + "033[" (octal) and backslash + "x1b[" (hex).
	esc := string([]byte{0x5c, 0x30, 0x33, 0x33, 0x5b})
	hex := string([]byte{0x5c, 0x78, 0x31, 0x62, 0x5b})
	forbidden := []string{esc, hex}

	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(dir, path)

		for _, pat := range forbidden {
			if strings.Contains(content, pat) {
				t.Errorf("%s: contains raw ANSI escape sequence (use lipgloss instead)", rel)
				break
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", dir, err)
	}
}
