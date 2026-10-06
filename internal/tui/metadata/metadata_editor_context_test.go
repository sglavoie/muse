package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMetadataFilenameSurvivesTitleEditsAndResize(t *testing.T) {
	ed := NewEditor("h", "Song", "Artist", "Album", "Jazz", 90).
		WithAlbumArtists("Album Artist").WithPath("/music/original-recording.flac")
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '!', Text: "!"})
	for _, size := range [][2]int{{100, 24}, {60, 12}, {100, 24}} {
		out := ansi.Strip(ed.Render(size[0], size[1], newTestStyles()))
		if size[1] > 12 && !strings.Contains(out, "File: original-recording.flac") {
			t.Fatalf("missing original filename:\n%s", out)
		}
		if strings.Contains(out, "/music/") || !strings.Contains(out, "Song!") {
			t.Fatalf("filename context or edited title incorrect:\n%s", out)
		}
		assertEditorFits(t, out, size[0], size[1])
	}
}

func TestMetadataVisualAndKeyboardOrderAgree(t *testing.T) {
	ed := NewEditor("h", "Song", "Artist", "Album", "Jazz", 90).WithAlbumArtists("Various")
	out := ansi.Strip(ed.Render(100, 24, newTestStyles()))
	last := -1
	for _, label := range []string{"Title:", "Artist:", "Album Artist:", "Album:", "Genre:", "Duration:"} {
		pos := strings.Index(out, label)
		if pos <= last {
			t.Fatalf("%s out of order:\n%s", label, out)
		}
		last = pos
	}
	for _, field := range []Field{FieldDuration, fieldGenre, fieldAlbum, FieldAlbumArtist, fieldArtist, fieldTitle} {
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if ed.focused != field {
			t.Fatalf("backward navigation focused %v, want %v", ed.focused, field)
		}
	}
}

func TestMetadataDurationHintAndLongFilenameFit(t *testing.T) {
	for _, bulk := range []bool{false, true} {
		ed := NewEditor("h", "Song", "Artist", "Album", "Jazz", 90).
			WithPath("/music/" + strings.Repeat("音楽", 50) + ".flac")
		if bulk {
			ed = NewBulkEditor([]string{"a", "b"}, "", "", "", "", "")
		}
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: '0', Text: "0"})
		for _, size := range [][2]int{{100, 24}, {60, 12}, {60, 15}} {
			out := ansi.Strip(ed.Render(size[0], size[1], newTestStyles()))
			if !strings.Contains(out, "s/m:ss/h:mm:ss") || !strings.Contains(out, "enter save") || !strings.Contains(out, "ctrl+r reset") {
				t.Fatalf("missing duration formats or controls:\n%s", out)
			}
			assertEditorFits(t, out, size[0], size[1])
		}
	}
}

func assertEditorFits(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) > height {
		t.Fatalf("editor exceeds height %d:\n%s", height, out)
	}
	for _, line := range lines {
		if ansi.StringWidth(line) > width {
			t.Fatalf("editor exceeds width %d: %q", width, line)
		}
	}
}
