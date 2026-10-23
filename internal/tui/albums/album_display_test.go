package albums

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestAlbumLabelsStayDisambiguatedWhenFiltering(t *testing.T) {
	a := library.AlbumKey(library.Track{Album: "Live", Path: "/music/alice/live/1.flac"})
	b := library.AlbumKey(library.Track{Album: "Live", Path: "/music/bob/live/1.flac"})
	v := View{Albums: []string{a, b}, Width: 80, Height: 20, Styles: styles.NewStyles(false)}
	for _, query := range []string{"", "alice", "/music/alice"} {
		v.FilterText = query
		out := ansi.Strip(v.Render())
		if !strings.Contains(out, "▎ Live — alice/live") {
			t.Fatalf("query %q lost the stable release label:\n%s", query, out)
		}
		if query != "" && strings.Contains(out, "Live — bob/live") {
			t.Fatalf("query %q failed to filter releases:\n%s", query, out)
		}
	}
	v.Level = TrackLevel
	v.SelectedAlbum = a
	v.Tracks = []library.Track{{Title: "Song", Album: "Live", Path: "/music/alice/live/1.flac"}}
	v.FilterText = ""
	if out := ansi.Strip(v.Render()); !strings.Contains(out, "Live — alice/live") || strings.Contains(out, "/music/") {
		t.Fatalf("drill-down lost the concise release label:\n%s", out)
	}
}
