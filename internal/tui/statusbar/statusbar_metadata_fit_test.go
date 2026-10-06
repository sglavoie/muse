package statusbar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/playback"
)

func TestLongMetadataKeepsTitleAndClock(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		for _, scanning := range []bool{false, true} {
			s := StatusBar{Artist: strings.Repeat("音楽", 40), Title: "Recognizable song title",
				Album: strings.Repeat("Long album ", 20), PlaylistSource: strings.Repeat("Long source ", 20),
				PlaybackState: playback.StatePlaying, Elapsed: "12:30", Total: "1:23:20",
				ElapsedSecs: 750, TotalSecs: 5000, LongOp: LongOpState{Active: scanning, Label: "Scan"}}
			lines := renderStatusBarLines(t, s, width)
			if !strings.Contains(ansi.Strip(lines[0]), "Recognizable") {
				t.Errorf("width %d: title lost: %s", width, lines[0])
			}
			if !strings.Contains(ansi.Strip(lines[1]), "12:30 / 1:23:20") {
				t.Errorf("width %d: clock lost: %s", width, lines[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > width {
					t.Errorf("width %d overflow: %s", width, line)
				}
			}
		}
	}
}
