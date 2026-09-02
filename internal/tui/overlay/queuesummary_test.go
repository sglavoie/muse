package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestQueueSummaryOverlay_PluralizesTracksLabel verifies the bug fix where the
// remaining-tracks label was hard-coded to the plural form, so a single
// remaining track was rendered as "(1 tracks)" instead of "(1 track)".
func TestQueueSummaryOverlay_PluralizesTracksLabel(t *testing.T) {
	q := QueueSummaryOverlay{
		Tracks:   2,
		RemainMs: 60_000,
		RemainN:  1,
		TotalMs:  120_000,
	}
	out := ansi.Strip(q.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(1 track)") {
		t.Errorf("expected singular '(1 track)' in output, got:\n%s", out)
	}
	if strings.Contains(out, "(1 tracks)") {
		t.Error("must not render plural 'tracks' for a single remaining track")
	}
}

// TestQueueSummaryOverlay_UsesPluralWhenMany verifies the plural form is still
// used for counts other than 1.
func TestQueueSummaryOverlay_UsesPluralWhenMany(t *testing.T) {
	q := QueueSummaryOverlay{
		Tracks:   5,
		RemainMs: 300_000,
		RemainN:  3,
		TotalMs:  600_000,
	}
	out := ansi.Strip(q.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "(3 tracks)") {
		t.Errorf("expected '(3 tracks)' in output, got:\n%s", out)
	}
}

// TestQueueSummaryOverlay_OmitsRemainingCountWhenZero verifies that the
// "(N tracks)" suffix is not appended when no tracks remain.
func TestQueueSummaryOverlay_OmitsRemainingCountWhenZero(t *testing.T) {
	q := QueueSummaryOverlay{
		Tracks:   2,
		RemainMs: 0,
		RemainN:  0,
		TotalMs:  120_000,
	}
	out := ansi.Strip(q.Render(80, 24, newTestStyles()))
	if strings.Contains(out, "tracks)") || strings.Contains(out, "track)") {
		t.Errorf("expected no remaining-count parenthetical when remainN==0, got:\n%s", out)
	}
}
