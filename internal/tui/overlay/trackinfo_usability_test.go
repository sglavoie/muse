package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
)

func TestTrackInfoScrollPreservesAllContent(t *testing.T) {
	info := NewTrackInfoOverlay("/music/"+strings.Repeat("日本語/", 25)+"last.flac", map[string]any{"Comment": strings.Repeat("long comment ", 30) + "END-COMMENT"}, 123, nil)
	for _, size := range [][2]int{{60, 12}, {80, 21}, {100, 27}} {
		width, height := size[0], size[1]
		info.Scroll("home", width, height)
		var screens string
		for i := 0; i < len(info.wrappedRows(width)); i++ {
			out := ansi.Strip(info.Render(width, height, newTestStyles()))
			if len(strings.Split(strings.TrimRight(out, "\n"), "\n")) > height {
				t.Fatalf("height overflow: %s", out)
			}
			for _, line := range strings.Split(out, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("width overflow: %q", line)
				}
			}
			screens += out
			info.Scroll("down", width, height)
		}
		for _, want := range []string{"last.flac", "END-COMMENT", "123"} {
			if !strings.Contains(screens, want) {
				t.Errorf("%dx%d lost %q", width, height, want)
			}
		}
		if len(info.wrappedRows(width)) > height-7 && !strings.Contains(screens, "↓") {
			t.Fatal("missing more-content indicator")
		}
		info.Scroll("end", width, height)
		if !strings.Contains(ansi.Strip(info.Render(width, height, newTestStyles())), "123") {
			t.Fatal("end must reveal final row")
		}
		info.Scroll("home", width, height)
		if info.offset != 0 {
			t.Fatal("home must reset scroll")
		}
	}
	// Growing the window after scrolling clamps to the available content.
	info.Scroll("end", 60, 12)
	out := ansi.Strip(info.Render(100, 100, newTestStyles()))
	if !strings.Contains(out, "Path:") || !strings.Contains(out, "123") {
		t.Fatal("resize lost content")
	}
}

func TestTrackInfoLibraryAndFileValues(t *testing.T) {
	info := NewTrackInfoOverlay("/song.flac", map[string]any{"Title": "Old title", "Artist": "Same artist", "Genre": "Rock", "AlbumArtist": "File album artist"}, 0, nil)
	info = info.withLibraryMetadata(library.Track{Title: "Correct title", Artist: "Same artist", Album: "New album", Genre: "", AlbumArtist: "Various Artists"})
	rows := make(map[string]string)
	for _, row := range info.rows {
		if row.Label != "" {
			rows[row.Label] = row.Text
		}
	}
	for label, want := range map[string]string{"Title:": "Correct title", "File Title:": "Old title", "Artist:": "Same artist", "Album:": "New album", "File Album:": "(missing)", "Genre:": "(blank)", "File Genre:": "Rock", "Album Artist:": "Various Artists", "File Album Artist:": "File album artist"} {
		if rows[label] != want {
			t.Errorf("%s = %q, want %q", label, rows[label], want)
		}
	}
	if _, ok := rows["File Artist:"]; ok {
		t.Fatal("identical original should not repeat")
	}
	if info.rows[0].Text != "Library metadata" {
		t.Fatal("effective metadata must come first")
	}
}
