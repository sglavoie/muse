package library

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

// TestHandleFsEvent_CreateAfterRemoveClearsDeleted verifies that a Create
// event for a path that was previously Removed/Renamed within the same
// debounce window clears the path from the deleted set. Without this
// behaviour, the TUI would briefly receive a Deleted=true event for a file
// that still exists (e.g. atomic-save dance, rapid rename-back), causing a
// visible flicker until the rescan re-adds the track.
func TestHandleFsEvent_CreateAfterRemoveClearsDeleted(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.mp3")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	pending := make(map[string]bool)
	deleted := make(map[string]struct{})

	// Simulate: file removed, then immediately re-created in the same window.
	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, nil, pending, deleted, w)
	if _, ok := deleted[audioPath]; !ok {
		t.Fatalf("precondition: expected %q in deleted after Remove", audioPath)
	}
	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Create}, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; ok {
		t.Errorf("Create after Remove must clear path from deleted; still present: %v", deleted)
	}
	if _, ok := pending[audioPath]; !ok {
		t.Errorf("Create after Remove must add path to pending")
	}
}

// TestHandleFsEvent_WriteAfterRemoveClearsDeleted is the Write variant of the
// same race: some editors emit Remove followed by Write+Chmod (e.g. via
// rewrite-in-place). The file still exists; we must not propagate a
// deletion message.
func TestHandleFsEvent_WriteAfterRemoveClearsDeleted(t *testing.T) {
	root := t.TempDir()
	audioPath := filepath.Join(root, "song.flac")
	mkfile(t, audioPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer w.Close()

	pending := make(map[string]bool)
	deleted := make(map[string]struct{})

	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Rename}, nil, pending, deleted, w)
	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Write}, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; ok {
		t.Errorf("Write after Rename must clear path from deleted; still present: %v", deleted)
	}
	if _, ok := pending[audioPath]; !ok {
		t.Errorf("Write after Rename must add path to pending")
	}
}

// whitespaceFieldExtractor returns tracks whose metadata fields contain only
// whitespace, simulating files written by broken taggers that emit " " or
// "\t" instead of leaving the tag blank.
type whitespaceFieldExtractor struct{}

func (e *whitespaceFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{
			Path:   f.Path,
			Hash:   f.Hash,
			Title:  "   ",
			Artist: "\t",
			Album:  "  ",
			Genre:  " ",
		}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*whitespaceFieldExtractor)(nil)

// TestBinaryRetry_WhitespaceMetadataTreatedAsEmpty verifies that an extractor
// returning whitespace-only Title/Artist values triggers the same
// filename-derived fallback as empty values. Otherwise the library shows
// blank-looking rows that still claim to have metadata.
func TestBinaryRetry_WhitespaceMetadataTreatedAsEmpty(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/Estas Tonne/INTERNAL FLIGHT.opus", Hash: "abc"},
	}
	tracks := extractWithBinaryRetry(context.Background(), &whitespaceFieldExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	tr := tracks[0]
	if tr.Title != "INTERNAL FLIGHT" {
		t.Errorf("title: want %q (filename stem fallback), got %q", "INTERNAL FLIGHT", tr.Title)
	}
	if tr.Artist != "Unknown Artist" {
		t.Errorf("artist: want %q, got %q", "Unknown Artist", tr.Artist)
	}
	if tr.Album != "" {
		t.Errorf("album: want %q (whitespace trimmed), got %q", "", tr.Album)
	}
	if tr.Genre != "" {
		t.Errorf("genre: want %q (whitespace trimmed), got %q", "", tr.Genre)
	}
}

// TestParseTrackNumber_RejectsNegative verifies that negative track or disc
// numbers are normalised to 0 (the unknown sentinel) rather than propagating
// into sorting/display.
func TestParseTrackNumber_RejectsNegative(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"-3", 0},
		{"-1/12", 0},
		{"3", 3},
		{"3/12", 3},
		{"", 0},
		{"  ", 0},
		{"abc", 0},
		{"0", 0},
	}
	for _, c := range cases {
		got := parseTrackNumber(c.in)
		if got != c.want {
			t.Errorf("parseTrackNumber(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}
