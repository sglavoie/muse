package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestInsightsCountsOfOneAreSingular guards the counted nouns in the strip,
// the Sessions headline, and the Coverage header against "1 plays",
// "1 sessions", and "1 tracks".
func TestInsightsCountsOfOneAreSingular(t *testing.T) {
	snap := insightsdata.Snapshot{TotalPlays: 1, SpanDays: 1}
	snap.SessionStats.Count = 1
	snap.Coverage.HeardTracks = 1
	snap.Coverage.TotalTracks = 3
	v := View{Snap: snap, width: 120}

	if got := ansi.Strip(v.renderStrip(styles.Styles{})); !strings.Contains(got, "1 play ·") {
		t.Errorf("strip should read \"1 play\", got %q", got)
	}
	if got := v.headline(PanelSessions); !strings.HasPrefix(got, "1 session ·") {
		t.Errorf("sessions headline should read \"1 session\", got %q", got)
	}
	cov := ansi.Strip(strings.Join(v.coverageHeaderLines(styles.Styles{}), "\n"))
	if !strings.Contains(cov, "you played 1 track.") {
		t.Errorf("coverage header should read \"played 1 track\", got %q", cov)
	}
}
