package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestRenderListEmpty(t *testing.T) {
	v := View{}
	out := v.Render()
	if !strings.Contains(out, "no playlists") {
		t.Fatalf("expected '(no playlists)' in output, got:\n%s", out)
	}
}

func TestRenderListShowsNameAndCount(t *testing.T) {
	v := View{
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Chill", TrackCount: 5},
			{ID: 2, Name: "Workout", TrackCount: 12},
		},
		Width: 40,
	}
	out := v.Render()
	if !strings.Contains(out, "Chill") {
		t.Errorf("expected 'Chill' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(5)") {
		t.Errorf("expected '(5)' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Workout") {
		t.Errorf("expected 'Workout' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(12)") {
		t.Errorf("expected '(12)' in output, got:\n%s", out)
	}
}
