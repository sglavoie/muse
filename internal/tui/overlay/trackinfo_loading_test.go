package overlay

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

func TestTrackInfoEnrichmentKeepsStoredDetailsAndScroll(t *testing.T) {
	track := library.Track{Title: "Correct title", Artist: "Artist", Duration: 123.5, PlayCount: 7, Path: "/music/song.flac"}
	info := NewLibraryTrackInfoOverlay(track, []db.TagRow{{Name: "favorite tag"}}, 4, "Loading file details…")
	info.offset = 3
	stored := info.rows[:len(info.rows)-1]
	for _, result := range []TrackInfoMsg{
		{Tags: map[string]any{"Title": "Original title", "SampleRate": 44100}},
		{Err: errors.New("file unavailable")},
	} {
		next := info.WithFileResult(result)
		if !reflect.DeepEqual(next.rows[:len(stored)], stored) || next.offset != 3 || next.Request != 4 {
			t.Fatal("file probe moved or discarded stored details")
		}
		out := next.Render(100, 35, newTestStyles())
		if result.Err != nil {
			if !strings.Contains(out, "File details unavailable: file unavailable") || strings.Contains(out, "(missing)") {
				t.Fatal(out)
			}
		} else if !strings.Contains(out, "Original title") || !strings.Contains(out, "44100") {
			t.Fatal(out)
		}
		if info.rows[len(info.rows)-1].Text != "Loading file details…" {
			t.Fatal("result mutated the previous model")
		}
	}
}

func TestTrackInfoProbeErrorKeepsDiagnostic(t *testing.T) {
	err := &exec.ExitError{Stderr: []byte("Error: File not found - /music/missing.flac\n")}
	got := trackInfoError(err)
	if !errors.Is(got, err) || !strings.Contains(got.Error(), "File not found") {
		t.Fatal(got)
	}
	if trackInfoError(nil) != nil {
		t.Fatal("successful probe became an error")
	}
}
