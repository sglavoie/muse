package library

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

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

// TestExtractWithBinaryRetry_EmptyInputReturnsNil guards against an infinite
// recursion if extractWithBinaryRetry is ever invoked with an empty batch
// (len(files)/2 == 0 would otherwise call itself with the same slice).
func TestExtractWithBinaryRetry_EmptyInputReturnsNil(t *testing.T) {
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, nil)
	if tracks != nil {
		t.Errorf("expected nil tracks for empty input, got %v", tracks)
	}
}

// TestExtractWithBinaryRetry_CancelledContextSkipsFallback verifies that when
// the context is cancelled, extractWithBinaryRetry does not generate
// filename-derived fallback tracks. Generating fallbacks on cancellation
// causes the cancelled scan to keep producing garbage tracks for every file
// after the failure point.
func TestExtractWithBinaryRetry_CancelledContextSkipsFallback(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "h1"},
		{Path: "/music/b.flac", Hash: "h2"},
		{Path: "/music/c.flac", Hash: "h3"},
		{Path: "/music/d.flac", Hash: "h4"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tracks := extractWithBinaryRetry(ctx, &alwaysFailExtractor{}, files)
	if tracks != nil {
		t.Errorf("expected nil tracks under cancelled ctx, got %d: %+v", len(tracks), tracks)
	}
}

// TestExtractBatches_CancelledContextDrainsInputWithoutDeadlock verifies that
// when the consumer cancels the context, ExtractBatches still drains its
// input channel so the upstream HashWorkers / Walk goroutines can exit. Prior
// to the fix, ExtractBatches would block on `out <- t` once the consumer
// stopped reading, leaking the entire pipeline.
func TestExtractBatches_CancelledContextDrainsInputWithoutDeadlock(t *testing.T) {
	in := make(chan HashedFile, 2000)
	for i := 0; i < 2000; i++ {
		in <- HashedFile{Path: "/music/x.flac", Hash: "h"}
	}
	close(in)

	ctx, cancel := context.WithCancel(context.Background())
	out := ExtractBatches(ctx, &fakeExtractor{}, in)

	// Consume one batch, then cancel and stop reading.
	<-out
	cancel()

	done := make(chan struct{})
	go func() {
		// Drain (and discard) until the producer closes; if it leaks,
		// this never completes.
		for range out {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ExtractBatches did not exit after ctx cancellation; producer leak")
	}
}

// TestCoerceFloat_NegativeNaNInfNormalized verifies that pathological
// numeric durations emitted by exiftool (negative, NaN, +Inf, -Inf) are
// normalised to 0 so they cannot corrupt downstream duration arithmetic.
func TestCoerceFloat_NegativeNaNInfNormalized(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"negative", `-1.5`},
		{"negative string", `"-1.5"`},
		{"negative HHMMSS", `"-1:00:00"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceFloat([]byte(c.in))
			if got != 0 {
				t.Errorf("coerceFloat(%s): want 0, got %v", c.in, got)
			}
		})
	}
}

// TestSanitizeDuration verifies the float guard rejects non-finite and
// negative inputs.
func TestSanitizeDuration(t *testing.T) {
	if got := sanitizeDuration(math.NaN()); got != 0 {
		t.Errorf("NaN: want 0, got %v", got)
	}
	if got := sanitizeDuration(math.Inf(1)); got != 0 {
		t.Errorf("+Inf: want 0, got %v", got)
	}
	if got := sanitizeDuration(math.Inf(-1)); got != 0 {
		t.Errorf("-Inf: want 0, got %v", got)
	}
	if got := sanitizeDuration(-0.0001); got != 0 {
		t.Errorf("negative: want 0, got %v", got)
	}
	if got := sanitizeDuration(0); got != 0 {
		t.Errorf("zero: want 0, got %v", got)
	}
	if got := sanitizeDuration(123.4); got != 123.4 {
		t.Errorf("positive: want 123.4, got %v", got)
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
