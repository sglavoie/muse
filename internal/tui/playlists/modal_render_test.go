package playlists

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// --- Render / buildBoxLines ---

func TestModalBuildBoxLinesContainsBorders(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta"))
	lines := m.buildBoxLines(120, styles.NewStyles(true))

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 box lines, got %d", len(lines))
	}
	// First line is top border
	if lines[0][0:len(cornerTL)] != cornerTL {
		t.Errorf("first line must start with top-left corner, got %q", lines[0][:3])
	}
	// Last line before hint is bottom border
	bottomBorder := lines[len(lines)-2]
	if bottomBorder[0:len(cornerBL)] != cornerBL {
		t.Errorf("bottom border must start with bottom-left corner, got %q", bottomBorder[:3])
	}
	// Final line is hint
	last := lines[len(lines)-1]
	if last == "" || last[0:2] != "  " {
		t.Errorf("last line must be keybinding hint, got %q", last)
	}
}

func TestModalBuildBoxLinesContainsTitle(t *testing.T) {
	m := NewDestinationPicker(nil)
	lines := m.buildBoxLines(120, styles.NewStyles(true))

	found := false
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), "Destination") {
			found = true
			break
		}
	}
	if !found {
		t.Error("box lines must contain 'Destination' title")
	}
}

func TestModalBuildBoxLinesContainsDestinations(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Chill", "Rock"))
	lines := m.buildBoxLines(120, styles.NewStyles(true))

	foundQueue := false
	foundChill := false
	foundRock := false
	for _, line := range lines {
		if strings.Contains(line, queueLabel) {
			foundQueue = true
		}
		if strings.Contains(line, "Chill") {
			foundChill = true
		}
		if strings.Contains(line, "Rock") {
			foundRock = true
		}
	}
	if !foundQueue {
		t.Error("box lines must contain Play Queue")
	}
	if !foundChill {
		t.Error("box lines must contain Chill")
	}
	if !foundRock {
		t.Error("box lines must contain Rock")
	}
}

func TestModalBuildBoxLinesFocusedRowHasAccent(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	m.cursor = 0 // Play Queue focused
	s := styles.NewStyles(true)
	lines := m.buildBoxLines(120, s)

	accentMarker := styledText(s.Accent, queueLabel)
	if ansi.Strip(accentMarker) == accentMarker {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}

	found := false
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, queueLabel) && line != stripped {
			found = true
			break
		}
	}
	if !found {
		t.Error("focused row must be styled with ANSI codes")
	}
}

func TestModalBuildBoxLinesClampsNegativeFocusToQueue(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	m.cursor = -4
	s := styles.NewStyles(true)
	lines := m.buildBoxLines(120, s)

	if ansi.Strip(s.Accent.Render(queueLabel)) == s.Accent.Render(queueLabel) {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}

	assertStyledModalRow(t, lines, queueLabel)
}

func TestModalBuildBoxLinesClampsPastEndFocusToCreate(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha"))
	m.cursor = 99
	s := styles.NewStyles(true)
	lines := m.buildBoxLines(120, s)

	if ansi.Strip(s.Accent.Render(createLabel)) == s.Accent.Render(createLabel) {
		t.Skip("lipgloss produced no ANSI output in this terminal environment")
	}

	assertStyledModalRow(t, lines, createLabel)
}

func assertStyledModalRow(t *testing.T, lines []string, label string) {
	t.Helper()
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, label) {
			if line == stripped {
				t.Fatalf("row for %q must be styled; got unstyled line %q", label, line)
			}
			return
		}
	}
	t.Fatalf("expected row for %q in modal lines", label)
}

func TestModalRenderCentered(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("A"))
	out := m.Render(80, 24, styles.NewStyles(true))

	// Should have some leading newlines (vertical centering)
	if out[0] != '\n' {
		t.Error("Render must have leading newlines for vertical centering")
	}
	// Should contain the box content
	if !strings.Contains(ansi.Strip(out), "Destination") {
		t.Error("Render output must contain 'Destination'")
	}
}

func TestModalRenderZeroDimensions(t *testing.T) {
	m := NewDestinationPicker(nil)
	out := m.Render(0, 0, styles.NewStyles(true))

	// Should not panic and should contain content without centering
	if !strings.Contains(ansi.Strip(out), "Destination") {
		t.Error("Render(0,0) must still contain 'Destination'")
	}
}

// --- columns edge cases ---

func TestComputeNarrowWidth(t *testing.T) {
	cols := Compute(5)
	// Very narrow — should not panic
	if cols.Name < 0 {
		t.Errorf("Compute(5): Name=%d, should not be negative", cols.Name)
	}
}

func TestComputeTrackNarrowWidth(t *testing.T) {
	cols := ComputeTrack(10)
	// Very narrow — should not panic
	if cols.Title < 0 {
		t.Errorf("ComputeTrack(10): Title=%d, should not be negative", cols.Title)
	}
}

// TestBuildBoxLines_LongPlaylistNameClamped verifies that on a narrow terminal
// the box width is clamped and long playlist names are truncated.
func TestBuildBoxLines_LongPlaylistNameClamped(t *testing.T) {
	longName := strings.Repeat("A", 100)
	m := NewDestinationPicker(fakePlaylistRows(longName))
	const termWidth = 40
	lines := m.buildBoxLines(termWidth, styles.NewStyles(true))

	// The first line is the top border: its rune length equals innerWidth + 2.
	boxWidth := len([]rune(lines[0]))
	if boxWidth > termWidth {
		t.Errorf("box width %d exceeds terminal width %d", boxWidth, termWidth)
	}
}

// TestBuildBoxLines_VeryNarrowTerminalNoPanic verifies that buildBoxLines does
// not panic when the terminal is extremely narrow (contentWidth could be 0).
func TestBuildBoxLines_VeryNarrowTerminalNoPanic(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("SomeName"))
	for _, width := range []int{0, 1, 2, 3, 4, 5, 6} {
		lines := m.buildBoxLines(width, styles.NewStyles(true))
		if len(lines) == 0 {
			t.Errorf("buildBoxLines(%d): expected non-empty output", width)
		}
	}
}

// TestBuildBoxLines_CJKLabelsAligned verifies that wide characters (CJK) in
// playlist names produce rows with correct display-width alignment. Each
// content row (between the vertical borders) must have the same display width.
func TestBuildBoxLines_CJKLabelsAligned(t *testing.T) {
	// "テスト" is 6 display columns (3 CJK chars × 2 cols each).
	m := NewDestinationPicker(fakePlaylistRows("テスト"))
	lines := m.buildBoxLines(80, styles.NewStyles(true))

	// The top border line defines the expected box display width.
	expectedWidth := ansi.StringWidth(lines[0])
	for i, line := range lines {
		// Skip the hint line (last line, outside the box).
		if i == len(lines)-1 {
			continue
		}
		got := ansi.StringWidth(ansi.Strip(line))
		if got != expectedWidth {
			t.Errorf("line %d: display width %d, expected %d\n  line: %q", i, got, expectedWidth, ansi.Strip(line))
		}
	}
}

func TestDestinationSummarySurvivesSearchAndPaging(t *testing.T) {
	m := NewDestinationPicker(fakePlaylistRows("Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta", "Theta")).WithTrackSummary(8, "32:14+")
	m.SetHeight(15)
	for _, width := range []int{60, 80, 120} {
		out := m.Render(width, 15, styles.NewStyles(true))
		if !strings.Contains(ansi.Strip(out), "Add 8 tracks · 32:14+") {
			t.Fatalf("missing summary: %s", out)
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("overflow: %s", line)
			}
		}
	}
	m, _ = m.HandleKey("G")
	m.search.Text = "Alpha"
	if out := m.Render(60, 15, styles.NewStyles(true)); !strings.Contains(ansi.Strip(out), "Add 8 tracks · 32:14+") {
		t.Fatal("search changed summary")
	}
	m = m.WithTrackSummary(1, "3:00")
	if out := m.Render(60, 15, styles.NewStyles(true)); !strings.Contains(ansi.Strip(out), "Add 1 track · 3:00") {
		t.Fatal("missing singular summary")
	}
}
