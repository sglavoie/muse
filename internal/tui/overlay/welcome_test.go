package overlay

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

const testConfigPath = "/home/user/.config/muse/config.toml"

func TestHintRows_DropsBlankLines(t *testing.T) {
	rows := HintRows(config.ConfigHint(testConfigPath))
	if len(rows) != 2 {
		t.Fatalf("HintRows = %d rows, want 2: %+v", len(rows), rows)
	}
	if !strings.Contains(rows[0].Text, testConfigPath) {
		t.Errorf("row 0 = %q, want the config path", rows[0].Text)
	}
	if !strings.Contains(rows[1].Text, "music_directories = [") {
		t.Errorf("row 1 = %q, want the music_directories key", rows[1].Text)
	}
}
