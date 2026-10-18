package settings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestLibrarySettingsUsesSpareRowsForControls(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), []string{"/music"})
	for _, size := range [][2]int{{60, 12}, {80, 21}} {
		out := ansi.Strip(o.Render(size[0], size[1], styles.NewStyles(true)))
		for _, want := range []string{"tab/⇧tab tabs", "e excluded", "a add", "d delete", "ctrl+s save", "esc cancel"} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %s at %v:\n%s", want, size, out)
			}
		}
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
		if strings.Count(strings.TrimRight(out, "\n"), "\n")+1 > size[1] {
			t.Fatal("height overflow")
		}
	}
}

func TestSettingsGuidanceExplainsFocusedFieldAndTiming(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), nil)
	for tab := TabPlayback; tab < tabCount; tab++ {
		o.Tab = tab
		for i, field := range o.Fields[tab] {
			o.cursor = i
			description := o.focusedDescription()
			if description == "" {
				t.Fatalf("no explanation for %s", field.label)
			}
			out := ansi.Strip(o.Render(100, 37, styles.NewStyles(false)))
			normalized := strings.Join(strings.Fields(strings.ReplaceAll(out, "│", " ")), " ")
			if !strings.Contains(normalized, description) {
				t.Fatalf("missing explanation: %s\n%s", description, out)
			}
		}
	}
	o.Tab = TabPlayback
	o.cursor = 3
	if !strings.Contains(o.focusedDescription(), "next launch") {
		t.Fatal("startup volume timing unclear")
	}
	o.cursor = 6
	if !strings.Contains(o.focusedDescription(), "Previews live") {
		t.Fatal("stereo live preview unclear")
	}
}
