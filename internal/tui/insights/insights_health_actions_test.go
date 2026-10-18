package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestHealthRowsShowRepairActionsWithoutCharts(t *testing.T) {
	v := View{Panel: PanelDataHealth, Health: []HealthRow{
		{Kind: HealthGenreCollision, Label: "Jazz / jazz", Hashes: []string{"one", "two"}},
		{Kind: HealthMissingMetadata, Label: "No title", Hashes: []string{"one"}},
		{Kind: HealthArtistDuplicate, Label: "Alice / The Alice", Hashes: []string{"two"}},
		{Kind: HealthUnverifiedDuration, Label: "Unverified", Hashes: []string{"two"}},
	}}
	v.width = 100
	rows := v.healthRows()
	for i, want := range []string{"m merge genres", "M edit missing titles", "enter compare artists", "enter inspect durations"} {
		out := ansi.Strip(v.renderRow(styles.NewStyles(true), rows[i], true))
		if !strings.Contains(out, want) || !strings.Contains(out, "track") || strings.ContainsAny(out, "█░▍") {
			t.Fatalf("not an actionable finding: %s", out)
		}
		if strings.Contains(out, "Review:") != (i >= 2) {
			t.Fatalf("wrong advisory classification: %s", out)
		}
	}
}
