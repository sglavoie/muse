package library

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
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

// TestHandleFsEvent_ChmodAfterActualRemoveKeepsDeleted verifies that a
// trailing metadata event for a path that is already gone does not erase the
// deletion marker. macOS-style event streams can include Chmod after Remove;
// clearing deleted in that case leaves the stale track in the in-memory index.
func TestHandleFsEvent_ChmodAfterActualRemoveKeepsDeleted(t *testing.T) {
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

	if err := os.Remove(audioPath); err != nil {
		t.Fatalf("remove: %v", err)
	}
	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Remove}, nil, pending, deleted, w)
	handleFsEvent(fsnotify.Event{Name: audioPath, Op: fsnotify.Chmod}, nil, pending, deleted, w)

	if _, ok := deleted[audioPath]; !ok {
		t.Errorf("Chmod after actual Remove must keep path in deleted")
	}
	if _, ok := pending[audioPath]; ok {
		t.Errorf("Chmod after actual Remove must not add missing path to pending")
	}
}

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

// TestWalk_RootWithLeadingDotIsNotSkipped verifies that a root directory
// whose basename starts with '.' (e.g. "~/.musiclib") is still traversed.
// Without this guard the hidden-file rule would abort the walk on the very
// first entry — the root itself — and silently scan zero files.
func TestWalk_RootWithLeadingDotIsNotSkipped(t *testing.T) {
	parent := t.TempDir()
	dotRoot := filepath.Join(parent, ".musiclib")
	if err := os.MkdirAll(dotRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mkfile(t, filepath.Join(dotRoot, "track.mp3"))
	mkfile(t, filepath.Join(dotRoot, "album", "song.flac"))

	got := collectNames(Walk(context.Background(), []string{dotRoot}, nil))
	want := []string{"song.flac", "track.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v (root with leading dot must be traversed)", got, want)
	}
}

// TestFilteredWalk_RootWithLeadingDotIsNotSkipped is the FilteredWalk variant
// of the same fix — DiffScan goes through this code path on incremental rescans.
func TestFilteredWalk_RootWithLeadingDotIsNotSkipped(t *testing.T) {
	parent := t.TempDir()
	dotRoot := filepath.Join(parent, ".musiclib")
	if err := os.MkdirAll(dotRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mkfile(t, filepath.Join(dotRoot, "track.mp3"))

	changed, _, err := FilteredWalk(context.Background(), []string{dotRoot}, nil, nil, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}
	if len(changed) != 1 {
		t.Errorf("changed: got %d entries, want 1 (root with leading dot)", len(changed))
	}
}

// TestWalk_HiddenChildrenStillSkippedUnderDotRoot verifies that the dot-root
// exemption applies only to the root entry, not to hidden descendants.
func TestWalk_HiddenChildrenStillSkippedUnderDotRoot(t *testing.T) {
	parent := t.TempDir()
	dotRoot := filepath.Join(parent, ".musiclib")
	if err := os.MkdirAll(dotRoot, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mkfile(t, filepath.Join(dotRoot, "visible.mp3"))
	mkfile(t, filepath.Join(dotRoot, ".hidden.mp3"))
	mkfile(t, filepath.Join(dotRoot, ".cache", "buried.mp3"))

	got := collectNames(Walk(context.Background(), []string{dotRoot}, nil))
	want := []string{"visible.mp3"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v (hidden descendants under dot-root must still be skipped)", got, want)
	}
}

// TestWalk_SymlinkUsesLinkNameForExtension verifies that a symlink pointing
// to an audio target is included based on the link's own name and extension,
// not the target's. Previously the symlink path used info.Name() after
// os.Stat, which produces platform-dependent results.
func TestWalk_SymlinkUsesLinkNameForExtension(t *testing.T) {
	dir := t.TempDir()
	// Real file with audio extension.
	target := filepath.Join(dir, "target.mp3")
	mkfile(t, target)
	// Symlink with audio extension -> follow.
	audioLink := filepath.Join(dir, "linked.flac")
	if err := os.Symlink(target, audioLink); err != nil {
		t.Skip("symlinks not supported:", err)
	}
	// Symlink without audio extension -> ignore even though target is audio.
	nonAudioLink := filepath.Join(dir, "linkedbin")
	if err := os.Symlink(target, nonAudioLink); err != nil {
		t.Skip("symlinks not supported:", err)
	}

	got := collectNames(Walk(context.Background(), []string{dir}, nil))
	if !slices.Contains(got, "linked.flac") {
		t.Errorf("expected linked.flac to be included (link name has audio ext): %v", got)
	}
	if slices.Contains(got, "linkedbin") {
		t.Errorf("expected linkedbin to be skipped (link name has no audio ext): %v", got)
	}
	if !slices.Contains(got, "target.mp3") {
		t.Errorf("expected target.mp3 to be included: %v", got)
	}
}

// TestHashWorkers_StatFailureAfterHashSkipsFile verifies that when os.Stat
// fails after a successful hash (file removed between read and stat), the
// HashedFile is not emitted at all. Previously a HashedFile with zero
// ModTime/SizeBytes leaked downstream and caused DiffScan to flag the
// path as changed on every subsequent rescan.
func TestHashWorkers_StatFailureAfterHashSkipsFile(t *testing.T) {
	in := make(chan string, 2)
	in <- "/does/not/exist/track1.mp3"
	in <- "/does/not/exist/track2.flac"
	close(in)

	// Custom hashFn that "succeeds" without reading the file so the
	// downstream os.Stat fails (path does not exist).
	hashFn := func(_ string) (string, error) { return "deadbeef", nil }

	out := hashWorkersN(2, in, hashFn)
	var got []HashedFile
	for hf := range out {
		got = append(got, hf)
	}
	if len(got) != 0 {
		t.Errorf("expected no emissions (Stat failed); got %d: %+v", len(got), got)
	}
}

// TestHashWorkers_StatSuccessPopulatesMetadata is the positive-case companion
// to the previous test: when Stat succeeds, ModTime and SizeBytes are filled in.
func TestHashWorkers_StatSuccessPopulatesMetadata(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(p, []byte("audio bytes"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	in := make(chan string, 1)
	in <- p
	close(in)

	hashFn := func(_ string) (string, error) { return "deadbeef", nil }

	out := hashWorkersN(1, in, hashFn)
	var got []HashedFile
	for hf := range out {
		got = append(got, hf)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 emission, got %d", len(got))
	}
	if got[0].SizeBytes != int64(len("audio bytes")) {
		t.Errorf("SizeBytes: got %d, want %d", got[0].SizeBytes, len("audio bytes"))
	}
	if got[0].ModTime == 0 {
		t.Errorf("ModTime: got 0, want a populated timestamp")
	}
}

// TestFilenameStem_DotPrefixedFile verifies that a filename consisting only
// of the extension (e.g. ".mp3") does not produce an empty Title via the
// filename fallback. An empty Title slips past downstream "if Title == ”"
// guards and produces blank library rows.
func TestFilenameStem_DotPrefixedFile(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/music/.mp3", ".mp3"},
		{"/music/.flac", ".flac"},
		{"/music/track.mp3", "track"},
		{"/music/no_ext", "no_ext"},
		{"/music/album/song.opus", "song"},
		{"/music/a.b.c.mp3", "a.b.c"},
	}
	for _, c := range cases {
		got := filenameStem(c.path)
		if got != c.want {
			t.Errorf("filenameStem(%q): got %q, want %q", c.path, got, c.want)
		}
	}
}

// TestExtractWithBinaryRetry_DotPrefixedFilenameNonEmptyTitle verifies the
// filename fallback path keeps a non-empty Title when the filename is
// extension-only (".mp3"). Previously this produced Title == "".
func TestExtractWithBinaryRetry_DotPrefixedFilenameNonEmptyTitle(t *testing.T) {
	// emptyFieldExtractor returns Title == "", forcing the filename fallback.
	files := []HashedFile{{Path: "/music/.mp3", Hash: "h"}}
	tracks := extractWithBinaryRetry(context.Background(), &emptyFieldExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Title == "" {
		t.Errorf("Title must not be empty for dot-prefixed filename, got %q", tracks[0].Title)
	}
}

// TestExtractWithBinaryRetry_SingleFileFailureDotPrefixed verifies the same
// guard in the single-file failure branch of extractWithBinaryRetry.
func TestExtractWithBinaryRetry_SingleFileFailureDotPrefixed(t *testing.T) {
	files := []HashedFile{{Path: "/music/.flac", Hash: "h"}}
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, files)
	if len(tracks) != 1 {
		t.Fatalf("expected 1 fallback track, got %d", len(tracks))
	}
	if tracks[0].Title == "" {
		t.Errorf("Title must not be empty in single-file fallback for %q", files[0].Path)
	}
}

// TestCoerceString_HeterogeneousArrayPreservesNonEmpty verifies that a mixed
// JSON array (string + number + null) is joined element-by-element instead
// of being discarded entirely.
func TestCoerceString_HeterogeneousArrayPreservesNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"mixed string and number", `["Rock", 42, "Pop"]`, "Rock; 42; Pop"},
		{"array with null", `["A", null, "B"]`, "A; B"},
		{"array with bool", `["A", true]`, "A; true"},
		{"all strings still works", `["Rock", "Jazz"]`, "Rock; Jazz"},
		{"empty array", `[]`, ""},
		{"single number", `42`, "42"},
		{"plain string", `"hello"`, "hello"},
		{"bool", `true`, "true"},
		{"null", `null`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceString([]byte(c.in))
			if got != c.want {
				t.Errorf("coerceString(%s): got %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestShouldExclude_RootOnlyPatternIsIgnored verifies that a degenerate
// pattern of separator characters only (e.g. "/" or "//") does not
// trigger a catch-all exclusion of every absolute path. Without the
// guard, every track would be silently filtered out.
func TestShouldExclude_RootOnlyPatternIsIgnored(t *testing.T) {
	cases := []string{"/", "//", "///"}
	for _, p := range cases {
		if ShouldExclude("/music/track.mp3", []string{p}) {
			t.Errorf("pattern %q must not exclude /music/track.mp3 (would hide entire library)", p)
		}
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
