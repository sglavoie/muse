package library

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSearchFieldOffsetsAllBasic(t *testing.T) {
	aOff, aLen, tOff, tLen, lOff, lLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	if aOff != 0 {
		t.Errorf("artistOffset: expected 0, got %d", aOff)
	}
	if aLen != 5 {
		t.Errorf("artistLen: expected 5, got %d", aLen)
	}
	// "Alice" (5) + SP = title starts at 6
	if tOff != 6 {
		t.Errorf("titleOffset: expected 6, got %d", tOff)
	}
	if tLen != 8 {
		t.Errorf("titleLen: expected 8, got %d", tLen)
	}
	// 6 + 8 + 1 = 15
	if lOff != 15 {
		t.Errorf("albumOffset: expected 15, got %d", lOff)
	}
	if lLen != 5 {
		t.Errorf("albumLen: expected 5, got %d", lLen)
	}
}

func TestSearchFieldOffsetsAllEmptyAlbum(t *testing.T) {
	_, _, _, _, lOff, lLen := searchFieldOffsetsAll("Art", "Tit", "")
	// "Art" (3) + SP + "Tit" (3) + SP = 8
	if lOff != 8 {
		t.Errorf("albumOffset with empty album: expected 8, got %d", lOff)
	}
	if lLen != 0 {
		t.Errorf("albumLen with empty album: expected 0, got %d", lLen)
	}
}

func TestFieldMatchPositionsAlbum(t *testing.T) {
	// Layout: "Alice Song One Debut file.mp3"
	// Artist: 0-4, Title: 6-13, Album: 15-19
	_, _, _, _, albumOff, albumLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	matchedIndexes := []int{15, 16, 17} // "Deb" in album
	got := fieldMatchPositions(matchedIndexes, albumOff, albumLen)
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Errorf("fieldMatchPositions for album: expected [0 1 2], got %v", got)
	}
}

func TestFieldMatchPositionsIgnoresOutOfRange(t *testing.T) {
	_, _, _, _, albumOff, albumLen := searchFieldOffsetsAll("Alice", "Song One", "Debut")
	// positions in artist and title should be excluded
	matchedIndexes := []int{0, 1, 6, 7, 15}
	got := fieldMatchPositions(matchedIndexes, albumOff, albumLen)
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("fieldMatchPositions filter: expected [0], got %v", got)
	}
}

// TestPadHighlightNoBaseStyleBackwardCompat verifies that calling padHighlight
// without a base style produces the same output as the original behaviour:
// bold ANSI on matched chars, plain runes elsewhere.
func TestPadHighlightNoBaseStyleBackwardCompat(t *testing.T) {
	got := padHighlight("Hello", 10, []int{0, 1})
	bold := lipgloss.NewStyle().Bold(true)
	want := bold.Render("He") + "llo" + strings.Repeat(" ", 5)
	if got != want {
		t.Errorf("padHighlight without base style:\n got %q\nwant %q", got, want)
	}
}

// TestPadHighlightWithBaseStylePreservesColor verifies that when a foreground
// style is passed as base, both matched and non-matched characters carry the
// foreground colour — preventing the inner bold reset from clearing it.
func TestPadHighlightWithBaseStylePreservesColor(t *testing.T) {
	purple := lipgloss.NewStyle().Foreground(lipgloss.Color("#c678dd"))
	got := padHighlight("Alice", 10, []int{0, 1}, purple)

	// The stripped (visible) text should be the same regardless of style.
	stripped := ansi.Strip(got)
	wantVisible := "Alice     "
	if stripped != wantVisible {
		t.Errorf("visible text: got %q, want %q", stripped, wantVisible)
	}

	// Bold+purple on "Al", plain purple on "ice", plain purple on padding.
	boldPurple := purple.Bold(true)
	if !strings.Contains(got, boldPurple.Render("Al")) {
		t.Errorf("expected bold+purple 'Al' in output, got %q", got)
	}
	if !strings.Contains(got, purple.Render("ice")) {
		t.Errorf("expected purple 'ice' in output, got %q", got)
	}
}

// TestPadHighlightBaseStyleNoPositions verifies that a base style with no
// matched positions wraps the entire padded string with the base style.
func TestPadHighlightBaseStyleNoPositions(t *testing.T) {
	purple := lipgloss.NewStyle().Foreground(lipgloss.Color("#c678dd"))
	got := padHighlight("Hi", 5, nil, purple)
	want := purple.Render("Hi   ")
	if got != want {
		t.Errorf("padHighlight base style, no positions:\n got %q\nwant %q", got, want)
	}
}

// TestPadHighlightGroupsConsecutiveMatches verifies that consecutive matched
// positions produce fewer ANSI sequences than per-character rendering.
func TestPadHighlightGroupsConsecutiveMatches(t *testing.T) {
	got := padHighlight("Hello", 10, []int{0, 1, 2})
	bold := lipgloss.NewStyle().Bold(true)
	// Should contain "Hel" as a single bold render, not three separate ones.
	if !strings.Contains(got, bold.Render("Hel")) {
		t.Errorf("expected grouped bold 'Hel', got %q", got)
	}
}
