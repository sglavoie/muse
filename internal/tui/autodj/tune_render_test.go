package autodj

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

const (
	// minWidth and minHeight mirror tui.MinTerminalWidth and
	// tui.MinTerminalHeight, which this package cannot import: the root model
	// imports the panel, not the other way round.
	minWidth  = 60
	minHeight = 15
	// minOverlayHeight is the body height the root hands an overlay at that
	// size; renderOverlayBody reserves three lines of surrounding chrome.
	minOverlayHeight = minHeight - 3
)

// plain strips the styling so an assertion reads the text the user sees. Styles
// are applied per run, so a styled heading contains escape codes between its
// letters and would never match a substring.
func plain(out string) string { return ansi.Strip(out) }

// assertFitsBox fails when the rendered overlay spills out of the area it was
// given, in either direction.
func assertFitsBox(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > height {
		t.Fatalf("render is %d lines tall, want at most %d:\n%s", len(lines), height, out)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("line %d is %d columns wide, want at most %d: %q", i, w, width, line)
		}
	}
}

func TestRenderShowsEveryRowOnARoomyTerminal(t *testing.T) {
	out := newTestPanel(true).Render(100, 40, styles.NewStyles(true), nil, Preview{})

	for r := row(0); r < rowCommit; r++ {
		if label := rowLabels[r]; label != "" && !strings.Contains(plain(out), label) {
			t.Fatalf("render is missing the %q row:\n%s", label, out)
		}
	}
	if !strings.Contains(plain(out), "[ Apply ]") {
		t.Fatalf("render is missing the commit button:\n%s", out)
	}
	if strings.Contains(plain(out), "j/k scrolls") {
		t.Fatalf("render offers scrolling although every row fits:\n%s", out)
	}
	assertFitsBox(t, out, 100, 40)
}

func TestRenderAtTheMinimumTerminalKeepsTheCursorVisible(t *testing.T) {
	p := newTestPanel(true)
	s := styles.NewStyles(true)

	// Walk the whole list at the smallest supported size. Every stop must be
	// drawn, and the box must stay inside the overlay area.
	for range int(rowCount) {
		out := p.Render(minWidth, minOverlayHeight, s, nil, Preview{})
		assertFitsBox(t, out, minWidth, minOverlayHeight)

		start, end := p.rowWindow(rowLinesFor(t, p, minOverlayHeight, nil))
		if p.Cursor() < start || p.Cursor() >= end {
			t.Fatalf("cursor %d outside the visible window [%d,%d)", p.Cursor(), start, end)
		}
		if label := rowLabels[p.focusedRow()]; label != "" && !strings.Contains(plain(out), label) {
			t.Fatalf("focused row %q is not drawn:\n%s", label, out)
		}
		p, _ = press(t, p, "down")
	}
}

// rowLinesFor reports how many row lines the panel budgets for a given height.
func rowLinesFor(t *testing.T, p TunePanel, termHeight int, status []StatusLine) int {
	t.Helper()
	rows, _ := p.splitBudget(termHeight, p.statusLines(status))
	return rows
}

func TestRenderAtTheMinimumTerminalOffersScrolling(t *testing.T) {
	out := newTestPanel(true).Render(minWidth, minOverlayHeight, styles.NewStyles(true), nil, Preview{})
	if !strings.Contains(plain(out), "j/k scrolls") {
		t.Fatalf("a scrolled panel does not say so:\n%s", out)
	}
}

func TestRenderDropsTheStatusBlockBeforeTheRows(t *testing.T) {
	p := newTestPanel(true)
	status := []StatusLine{{Text: "Last refresh: 3 replaced, 1 retired", Tone: ToneInfo}}

	// Nine lines leave four for content: too few to pay for a separator and an
	// explanation on top of a usable row window.
	rows, shown := p.splitBudget(9, p.statusLines(status))
	if len(shown) != 0 {
		t.Fatalf("status lines = %v, want them dropped at 9 lines", shown)
	}
	if rows < minRowLines {
		t.Fatalf("row budget = %d, want at least %d", rows, minRowLines)
	}
	assertFitsBox(t, p.Render(minWidth, 9, styles.NewStyles(true), status, Preview{}), minWidth, 9)
}

func TestRenderCapsTheStatusBlock(t *testing.T) {
	p := newTestPanel(true)
	status := []StatusLine{
		{Text: "one", Tone: ToneInfo}, {Text: "two", Tone: ToneWarn},
		{Text: "three", Tone: ToneError}, {Text: "four", Tone: ToneInfo},
	}

	_, shown := p.splitBudget(40, p.statusLines(status))
	if len(shown) != maxStatusLines {
		t.Fatalf("%d status lines shown, want the cap of %d", len(shown), maxStatusLines)
	}
}

func TestStatusLinesLeadWithThePanelsOwnState(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")
	p.draft.Include.TagIDs = []int64{42}
	p.err = errors.New("queue refresh: mpv is not responding")

	fromRoot := StatusLine{Text: "Additions paused: nothing matches", Tone: ToneWarn}
	lines := p.statusLines([]StatusLine{fromRoot})

	if len(lines) != 4 {
		t.Fatalf("status lines = %v, want the error, the repair offer, the dirty note and the root line", lines)
	}
	if lines[0].Tone != ToneError || !strings.Contains(lines[0].Text, "mpv") {
		t.Fatalf("first line = %+v, want the refusal", lines[0])
	}
	if !strings.Contains(lines[1].Text, "1 unresolved tag reference") || !strings.Contains(lines[1].Text, "press r") {
		t.Fatalf("second line = %q, want the repair offer", lines[1].Text)
	}
	if !strings.Contains(lines[2].Text, "ctrl+s to apply") {
		t.Fatalf("third line = %q, want the unapplied-changes note", lines[2].Text)
	}
	if lines[3] != fromRoot {
		t.Fatalf("last line = %+v, want the root's own line last", lines[3])
	}
}

func TestStatusReportsACommitInProgress(t *testing.T) {
	p := newTestPanel(false).Submit(2)

	lines := p.statusLines(nil)
	if len(lines) != 1 || lines[0].Text != "Start in progress…" {
		t.Fatalf("status lines = %v, want the commit in flight", lines)
	}
	if got := p.rowValue(rowCommit); got != "[ Starting… ]" {
		t.Fatalf("commit button = %q, want it to report progress", got)
	}
}

func TestRenderNarrowsWithTheTerminal(t *testing.T) {
	out := newTestPanel(true).Render(40, minOverlayHeight, styles.NewStyles(true), nil, Preview{})
	assertFitsBox(t, out, 40, minOverlayHeight)
}

func TestToneStylesAreDistinct(t *testing.T) {
	s := styles.NewStyles(true)
	info := toneStyle(ToneInfo, s).Render("x")
	warn := toneStyle(ToneWarn, s).Render("x")
	fail := toneStyle(ToneError, s).Render("x")
	if info == warn || warn == fail || info == fail {
		t.Fatalf("tones render identically: info=%q warn=%q error=%q", info, warn, fail)
	}
}
