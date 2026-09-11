package autodj

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// previewPanel is a panel already showing the read-only page.
func previewPanel(t *testing.T) TunePanel {
	t.Helper()
	p, action := press(t, newTestPanel(true), "tab")
	if action != ActionPreview {
		t.Fatalf("switching to Preview returned %v, want a draft-plan request", action)
	}
	if !p.PreviewActive() {
		t.Fatal("tab did not reach the Preview page")
	}
	return p
}

// previewScenarios are the five shapes the tab has to stay readable in. They are
// fixtures rather than planner output on purpose: a rendering test that had to
// produce a shortage by arranging a library would be testing the planner.
func previewScenarios() map[string]Preview {
	return map[string]Preview{
		"normal":   readyPreview(),
		"narrow":   narrowPreview(),
		"empty":    emptyPreview(),
		"shortage": shortagePreview(),
		"error":    failedPreview(),
	}
}

// shortagePreview is a batch that hit its size but not its discovery split.
func shortagePreview() Preview {
	v := readyPreview()
	v.Outcome = radio.PlanOutcomeRelaxed
	v.DiscoveryTarget = 4
	v.Shortages = radio.Shortages{{Kind: radio.ShortageDiscovery, Want: 4, Got: 2}}
	v.Reasons = radio.Reasons{{Code: radio.ReasonDiscoveryRelaxed}}
	return v
}

func failedPreview() Preview {
	return Preview{
		Status:   PreviewFailed,
		Settings: readyPreview().Settings,
		Err:      errPreviewFixture,
	}
}

var errPreviewFixture = errors.New("library index unavailable")

// Every scenario has to fit the area it is handed, at the roomiest size and at
// the smallest terminal Muse supports. Wrapping a long explanation is what makes
// this worth asserting: the reasons are sentences, not fields.
func TestPreviewRenderFitsEverySize(t *testing.T) {
	s := styles.NewStyles(true)
	sizes := []struct{ w, h int }{{100, 40}, {minWidth, minOverlayHeight}, {40, minOverlayHeight}}

	for name, v := range previewScenarios() {
		for _, size := range sizes {
			out := previewPanel(t).Render(size.w, size.h, s, nil, v)
			assertFitsBox(t, out, size.w, size.h)
			if !strings.Contains(plain(out), "Auto-DJ") {
				t.Fatalf("%s at %dx%d lost its title:\n%s", name, size.w, size.h, out)
			}
		}
	}
}

func TestPreviewRenderMarksTheActiveTab(t *testing.T) {
	s := styles.NewStyles(true)
	tune := newTestPanel(true).Render(100, 40, s, nil, readyPreview())
	preview := previewPanel(t).Render(100, 40, s, nil, readyPreview())

	for _, out := range []string{tune, preview} {
		if !strings.Contains(plain(out), "Tune") || !strings.Contains(plain(out), "Preview") {
			t.Fatalf("tab bar is incomplete:\n%s", out)
		}
	}
	if tune == preview {
		t.Fatal("both pages render identically, so the active tab is not marked")
	}
	if strings.Contains(plain(tune), "Would add") {
		t.Fatalf("the Tune page is showing the draft batch:\n%s", tune)
	}
}

// The batch is the point of the page, so a short terminal must still reach it.
func TestPreviewRenderScrollsToTheBatch(t *testing.T) {
	s := styles.NewStyles(true)
	p := previewPanel(t)
	v := readyPreview()
	v.Gaps = []EvidenceGap{{Signal: "completion rates"}}

	out := p.Render(minWidth, minOverlayHeight, s, nil, v)
	if !strings.Contains(plain(out), "j/k scrolls") {
		t.Fatalf("a page that does not fit does not offer scrolling:\n%s", out)
	}

	for range 40 {
		p, _ = press(t, p, "j")
	}
	bottom := plain(p.Render(minWidth, minOverlayHeight, s, nil, v))
	if !strings.Contains(bottom, "finished listens only") {
		t.Fatalf("scrolling to the end does not reach the footnote:\n%s", bottom)
	}
	assertFitsBox(t, p.Render(minWidth, minOverlayHeight, s, nil, v), minWidth, minOverlayHeight)
}

// An offset that was legal on a tall terminal must not strand the page past its
// own end when the terminal shrinks.
func TestPreviewWindowClampsToTheLineCount(t *testing.T) {
	start, end := previewWindow(10, 500, 12)
	if end != 10 {
		t.Fatalf("window = [%d,%d), want it to end at the last line", start, end)
	}
	if start != 10-(12-previewChromeLines) {
		t.Fatalf("window start = %d, want the last full page", start)
	}
	if start, end := previewWindow(3, 7, 40); start != 0 || end != 3 {
		t.Fatalf("window = [%d,%d), want the whole page when it fits", start, end)
	}
}

func TestPreviewScrollingNeverLeavesTheOffsetUnbounded(t *testing.T) {
	p := previewPanel(t)
	for range maxPreviewOffset + 50 {
		p, _ = press(t, p, "j")
	}
	if p.previewOffset != maxPreviewOffset {
		t.Fatalf("offset = %d, want it capped at %d", p.previewOffset, maxPreviewOffset)
	}
	for range maxPreviewOffset + 50 {
		p, _ = press(t, p, "k")
	}
	if p.previewOffset != 0 {
		t.Fatalf("offset = %d, want it back at the top", p.previewOffset)
	}
}

// Scrolling is a read. It may move the window and nothing else — in particular
// it must not edit the draft the way the same keys do on the Tune page.
func TestPreviewKeysDoNotEditTheDraft(t *testing.T) {
	p := previewPanel(t)
	before := p.Draft()

	p, action := press(t, p, "j", "k", "h", "l", "x", "enter", "down", "up", "left", "right")
	if action != actionNone {
		t.Fatalf("reading the page asked the root to %v", action)
	}
	if !p.Draft().Equal(before) || p.Dirty() {
		t.Fatalf("draft = %+v, want it untouched by a read-only page", p.Draft())
	}
}

func TestPreviewRKeyAsksForAFreshDraft(t *testing.T) {
	p, action := press(t, previewPanel(t), "j", "j", "r")
	if action != ActionPreview {
		t.Fatalf("r returned %v, want a draft-plan request", action)
	}
	if p.previewOffset != 0 {
		t.Fatalf("offset = %d, want a replan to start at the top", p.previewOffset)
	}
}

func TestTabWrapsBackToTune(t *testing.T) {
	p, _ := press(t, previewPanel(t), "tab")
	if p.PreviewActive() {
		t.Fatal("tab did not return to the Tune page")
	}
	back, action := press(t, p, "shift+tab")
	if !back.PreviewActive() || action != ActionPreview {
		t.Fatalf("shift+tab = (preview %v, %v), want the Preview page", back.PreviewActive(), action)
	}
}

// Leaving and returning must start at the top rather than at wherever the last
// visit ended, which would look like a page that failed to render its headline.
func TestSwitchingTabsResetsTheScroll(t *testing.T) {
	p := previewPanel(t)
	p, _ = press(t, p, "j", "j", "j")
	p, _ = press(t, p, "tab")
	p, _ = press(t, p, "tab")
	if p.previewOffset != 0 {
		t.Fatalf("offset = %d, want a fresh visit to start at the top", p.previewOffset)
	}
}

// The commit gesture keeps working from the read-only page: the contract calls
// for Apply from the preview, and only Apply may change anything.
func TestCommitStillWorksFromThePreviewPage(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")
	p, _ = press(t, p, "tab")

	_, action := press(t, p, "ctrl+s")
	if action != ActionCommit {
		t.Fatalf("ctrl+s from the Preview page returned %v, want a commit", action)
	}
}

func TestEscapeFromThePreviewPageCancels(t *testing.T) {
	if _, action := press(t, previewPanel(t), "esc"); action != ActionCancel {
		t.Fatalf("esc returned %v, want a cancel", action)
	}
}

func TestPreviewHintNamesTheCommitVerb(t *testing.T) {
	s := styles.NewStyles(true)
	running := plain(previewPanel(t).Render(100, 40, s, nil, readyPreview()))
	if !strings.Contains(running, "ctrl+s apply") {
		t.Fatalf("hint does not offer apply on a running session:\n%s", running)
	}

	idle, _ := press(t, newTestPanel(false), "tab")
	if out := plain(idle.Render(100, 40, s, nil, readyPreview())); !strings.Contains(out, "ctrl+s start") {
		t.Fatalf("hint does not offer start when no session runs:\n%s", out)
	}
}

func TestWrapTextKeepsOverlongWords(t *testing.T) {
	lines := wrapText("/music/"+strings.Repeat("x", 40)+".flac short", 10)
	if len(lines) < 2 {
		t.Fatalf("wrapped = %v, want the long word on its own line", lines)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 10 {
			t.Fatalf("line %d is %q, %d columns wide", i, l, w)
		}
	}
	if !strings.Contains(strings.Join(lines, " "), "short") {
		t.Fatalf("wrapped = %v, want the trailing word kept", lines)
	}
}
