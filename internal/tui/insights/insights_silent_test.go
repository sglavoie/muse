package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// The empty-panel phrases the briefing used to repeat once per listening panel.
var emptyPanelLines = []string{
	"no plays in this window", "no artists in this window", "no sessions in this window",
	"not enough completed", "0 first-time tracks",
}

func silentView(width, height int) View {
	return View{
		Loaded: true, width: width, Height: height,
		Snap: insightsdata.Snapshot{Coverage: insightsdata.Coverage{TotalTracks: 40, UnheardTracks: 40, UnheardSecs: 7200}},
	}
}

func TestSilentWindowCollapsesListeningPanels(t *testing.T) {
	v := silentView(120, 36)
	if !v.silent() {
		t.Fatal("precondition: no plays anywhere")
	}
	out := ansi.Strip(v.Render(styles.NewStyles(false), nil, nil, nil, "", ""))
	for _, gone := range append(emptyPanelLines, "Rhythm", "Sessions", "Data Health") {
		if strings.Contains(out, gone) {
			t.Errorf("silent briefing still shows %q:\n%s", gone, out)
		}
	}
	for _, want := range []string{"[Coverage]", "40 of 40 tracks never played", silentMessage, "All Time · w/W change window"} {
		if !strings.Contains(out, want) {
			t.Errorf("silent briefing missing %q:\n%s", want, out)
		}
	}
}

func TestSilentWindowKeepsDataHealthWithFindings(t *testing.T) {
	v := silentView(120, 36)
	v.Snap.Quality.Issues = []insightsdata.HealthIssue{{Detail: "3 tracks have no duration"}}
	out := ansi.Strip(v.renderBriefing(styles.NewStyles(false)))
	if !strings.Contains(out, "Data Health") || !strings.Contains(out, "3 tracks have no duration") {
		t.Fatalf("Data Health with a finding dropped:\n%s", out)
	}
	if got := v.ShiftPanel(1).Panel; got != PanelDataHealth {
		t.Fatalf("next panel from Coverage = %v, want Data Health", got)
	}
	if got := v.ShiftPanel(1).ShiftPanel(1).Panel; got != PanelCoverage {
		t.Fatalf("silent cycle should wrap over the two offered panels, got %v", got)
	}
}

// Switching to a silent window while a listening panel is active keeps that
// panel offered, so the cursor is never left on a panel the briefing hides.
func TestSilentWindowKeepsActivePanel(t *testing.T) {
	v := silentView(120, 36)
	v.Panel = PanelSessions
	out := ansi.Strip(v.renderBriefing(styles.NewStyles(false)))
	if !strings.Contains(out, "[Sessions]") || strings.Contains(out, "Rhythm") {
		t.Fatalf("active panel lost or others kept:\n%s", out)
	}
	if got := v.ShiftPanel(1).Panel; got != PanelCoverage {
		t.Fatalf("next from Sessions = %v, want Coverage", got)
	}
}

// One listening panel with data keeps the full briefing, empty lines included.
func TestPartlyEmptyWindowKeepsEveryPanel(t *testing.T) {
	v := silentView(120, 36)
	v.Snap.Trends.Buckets = []insightsdata.TrendBucket{{Plays: 2, ListenedSecs: 400}}
	v.Snap.Trends.AvgSecs = 400
	if v.silent() {
		t.Fatal("all-time trends have plays: not silent")
	}
	out := ansi.Strip(v.renderBriefing(styles.NewStyles(false)))
	if strings.Contains(out, silentMessage) || !strings.Contains(out, "no plays in this window") || !strings.Contains(out, "Data Health") {
		t.Fatalf("partly empty window should keep current behavior:\n%s", out)
	}
	if len(v.visiblePanels()) != len(insightsPanelSpecs) {
		t.Fatal("partly empty window hid panels")
	}
}

func TestSilentWindowHeaderBudget(t *testing.T) {
	v := silentView(100, 30)
	want := 30 - 1 - (1 + silentLines) - insightsMinBodyRows
	if got := v.headerBudget(); got != want {
		t.Fatalf("silent budget: got %d, want %d", got, want)
	}
	// The header height must agree with what the briefing really draws.
	if got := strings.Count(v.renderBriefing(styles.Styles{}), "\n") + 1; got != v.briefingLines() {
		t.Fatalf("briefing draws %d lines, counted %d", got, v.briefingLines())
	}
}

func TestSilentWindowCompactSelector(t *testing.T) {
	v := silentView(60, 15)
	out := ansi.Strip(v.renderPanelSelector(styles.NewStyles(false)))
	if out != "Coverage · "+silentMessageShort {
		t.Fatalf("compact selector: %q", out)
	}
	narrow := silentView(60, 36)
	if note := ansi.Strip(strings.Join(narrow.silentNote(styles.NewStyles(false)), "\n")); !strings.Contains(note, silentMessageShort) || strings.Contains(note, "play something") {
		t.Fatalf("narrow note should use the short message:\n%s", note)
	}
}
