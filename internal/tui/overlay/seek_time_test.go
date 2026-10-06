package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestParseSeekTime(t *testing.T) {
	for value, want := range map[string]float64{"0": 0, "90": 90, "12:30": 750, "90:00": 5400, "1:02:03": 3723} {
		got, err := ParseSeekTime(" " + value + " ")
		if err != nil || got != want {
			t.Errorf("%q = %v, %v; want %v", value, got, err, want)
		}
	}
	for _, value := range []string{"", "-1", "+1", "1:60", "1:60:00", "1:", ":30", "NaN", "1.5", "1:2:3:4", "99999999999999999999"} {
		if _, err := ParseSeekTime(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestSeekTimePromptFitsMinimumWindow(t *testing.T) {
	prompt := NewSeekTime(strings.Repeat("音楽", 40))
	prompt.Error = "Invalid time; minutes and seconds after ':' must be below 60"
	out := prompt.Render(60, 11, styles.NewStyles(true))
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("line overflow: %q", line)
		}
	}
	if len(strings.Split(out, "\n")) > 11 {
		t.Fatalf("prompt too tall: %s", out)
	}
}
