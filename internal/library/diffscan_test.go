package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBulkLoad_SortsCorrectly verifies that BulkLoad produces the same sorted
// order as inserting the same tracks one by one with Insert.
func TestBulkLoad_SortsCorrectly(t *testing.T) {
	tracks := []Track{
		{Artist: "Zappa", Title: "Bobby Brown", Path: "/z.flac", Hash: "h1"},
		{Artist: "Aphex Twin", Title: "Xtal", Path: "/x.flac", Hash: "h2"},
		{Artist: "Aphex Twin", Title: "Actium", Path: "/a.flac", Hash: "h3"},
		{Artist: unknownArtist, Title: "Unknown", Path: "/u.flac", Hash: "h4"},
		{Artist: "Miles Davis", Title: "So What", Path: "/m.flac", Hash: "h5"},
	}

	var insertIdx LibraryIndex
	for _, tr := range tracks {
		insertIdx.Insert(tr)
	}

	var bulkIdx LibraryIndex
	bulkIdx.BulkLoad(tracks)

	got := bulkIdx.Tracks()
	want := insertIdx.Tracks()
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Hash != w.Hash {
			t.Errorf("position %d: got %q, want %q", i, got[i].Hash, w.Hash)
		}
	}
}

// TestBulkLoad_ReplacesExistingContent verifies that calling BulkLoad on an
// already-populated index replaces the previous content entirely.
func TestBulkLoad_ReplacesExistingContent(t *testing.T) {
	var idx LibraryIndex
	idx.Insert(Track{Artist: "Old", Title: "Old", Hash: "old", Path: "/old.flac"})

	fresh := []Track{
		{Artist: "New", Title: "New", Hash: "new", Path: "/new.flac"},
	}
	idx.BulkLoad(fresh)

	if idx.Len() != 1 {
		t.Fatalf("Len: got %d, want 1", idx.Len())
	}
	if idx.Tracks()[0].Hash != "new" {
		t.Errorf("expected new track, got %q", idx.Tracks()[0].Hash)
	}
}

// BenchmarkBulkLoad measures BulkLoad vs N individual Inserts for 50,000 tracks.
func BenchmarkBulkLoad(b *testing.B) {
	const n = 50_000
	tracks := make([]Track, n)
	for i := range n {
		tracks[i] = Track{
			Artist: fmt.Sprintf("Artist%05d", i%1000),
			Title:  fmt.Sprintf("Track%05d", i),
			Hash:   fmt.Sprintf("%064x", i),
			Path:   fmt.Sprintf("/music/track%05d.flac", i),
		}
	}
	b.ResetTimer()
	for b.Loop() {
		var idx LibraryIndex
		idx.BulkLoad(tracks)
	}
}

// makeAudioFiles creates n audio files in dir and returns their paths.
func makeAudioFiles(tb testing.TB, dir string, n int) []string {
	tb.Helper()
	paths := make([]string, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("track_%04d.mp3", i))
		if err := os.WriteFile(p, []byte("ID3"), 0o644); err != nil {
			tb.Fatalf("write file: %v", err)
		}
		paths[i] = p
	}
	return paths
}

// TestFilteredWalk_AllNewFiles verifies that when known is empty, all audio
// files are returned as changed and deleted is empty.
func TestFilteredWalk_AllNewFiles(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 5)

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != len(paths) {
		t.Errorf("changed: got %d, want %d", len(changed), len(paths))
	}
}

// TestFilteredWalk_UnchangedFilesSkipped verifies that files whose mtime and
// size match the known map are not included in the changed channel.
func TestFilteredWalk_UnchangedFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 3)

	// Build a known map from the current on-disk state.
	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != 0 {
		t.Errorf("changed: got %v, want empty (all files unchanged)", changed)
	}
}

// TestFilteredWalk_ModifiedFileIncluded verifies that a file with changed mtime
// appears in the changed channel even when its path is in the known map.
func TestFilteredWalk_ModifiedFileIncluded(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 2)

	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	known := map[string]KnownFile{
		paths[0]: {ModTime: info.ModTime().Unix() - 1, SizeBytes: info.Size()}, // stale mtime
		paths[1]: {ModTime: info.ModTime().Unix(), SizeBytes: info.Size()},     // current
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if len(changed) != 1 || changed[0] != paths[0] {
		t.Errorf("changed: got %v, want [%s]", changed, paths[0])
	}
}

// TestFilteredWalk_DeletedFileReported verifies that a path in known that no
// longer exists on disk appears in the deleted slice.
func TestFilteredWalk_DeletedFileReported(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 2)

	// Build known, then delete paths[1].
	known := make(map[string]KnownFile, 2)
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}
	if err := os.Remove(paths[1]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	changed, deleted, err := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(changed) != 0 {
		t.Errorf("changed: got %v, want empty", changed)
	}
	if len(deleted) != 1 || deleted[0] != paths[1] {
		t.Errorf("deleted: got %v, want [%s]", deleted, paths[1])
	}
}

// TestDiffScan_EmptyKnownIsFullScan verifies that DiffScan with an empty known
// map produces the same track count as a full Scan.
func TestDiffScan_EmptyKnownIsFullScan(t *testing.T) {
	const n = 10
	dir := t.TempDir()
	makeAudioFiles(t, dir, n)

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, nil, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}

	var count int
	for range trackCh {
		count++
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty on first run", deleted)
	}
	if count != n {
		t.Errorf("tracks: got %d, want %d", count, n)
	}
}

// TestDiffScan_UnchangedFilesSkipped verifies that when all files match the
// known map, no tracks are emitted and the scan channel closes immediately.
func TestDiffScan_UnchangedFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 5)

	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, known, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}

	var count int
	for range trackCh {
		count++
	}

	if len(deleted) != 0 {
		t.Errorf("deleted: got %v, want empty", deleted)
	}
	if count != 0 {
		t.Errorf("tracks: got %d, want 0 (all unchanged)", count)
	}
}

// TestDiffScan_DeletedFilesReported verifies that files in known that no longer
// exist on disk are returned in the deleted slice.
func TestDiffScan_DeletedFilesReported(t *testing.T) {
	dir := t.TempDir()
	paths := makeAudioFiles(t, dir, 3)

	known := make(map[string]KnownFile, len(paths))
	for _, p := range paths {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}
	// Delete one file.
	if err := os.Remove(paths[2]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	fe := &fakeExtractor{}
	trackCh, deleted, err := DiffScan(context.Background(), []string{dir}, nil, fe, known, nil)
	if err != nil {
		t.Fatalf("DiffScan: %v", err)
	}
	for range trackCh {
	}

	if len(deleted) != 1 || deleted[0] != paths[2] {
		t.Errorf("deleted: got %v, want [%s]", deleted, paths[2])
	}
}

// TestFilteredWalk_HiddenFilesSkipped verifies hidden audio files are excluded.
func TestFilteredWalk_HiddenFilesSkipped(t *testing.T) {
	dir := t.TempDir()
	// Visible file.
	visible := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(visible, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Hidden file — should be ignored.
	hidden := filepath.Join(dir, ".hidden.mp3")
	if err := os.WriteFile(hidden, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)

	if len(changed) != 1 || changed[0] != visible {
		t.Errorf("changed: got %v, want [%s]", changed, visible)
	}
}

// TestFilteredWalk_NonAudioFilesIgnored verifies non-audio extensions are not included.
func TestFilteredWalk_NonAudioFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "track.flac")
	if err := os.WriteFile(audio, []byte("fLaC"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	nonAudio := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(nonAudio, []byte("text"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, nil, nil)

	if len(changed) != 1 || changed[0] != audio {
		t.Errorf("changed: got %v, want [%s]", changed, audio)
	}
}

// TestFilteredWalk_SizeMismatchTriggersRescan verifies that a size change alone
// marks the file as changed, even when mtime matches.
func TestFilteredWalk_SizeMismatchTriggersRescan(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(p, []byte("ID3"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, _ := os.Stat(p)
	// Pretend known has a different size.
	known := map[string]KnownFile{
		p: {ModTime: info.ModTime().Unix(), SizeBytes: info.Size() + 1},
	}

	changed, _, _ := FilteredWalk(context.Background(), []string{dir}, nil, known, nil)

	if len(changed) != 1 {
		t.Errorf("changed: got %v, want 1 file (size mismatch)", changed)
	}
}

// TestPathsToChan_ClosesWhenDone verifies the channel returned by pathsToChan
// is always closed after all paths have been sent, even when the slice is empty.
func TestPathsToChan_ClosesWhenDone(t *testing.T) {
	paths := []string{"/a.mp3", "/b.mp3", "/c.mp3"}

	ch := pathsToChan(context.Background(), paths)

	var got []string
	for p := range ch {
		got = append(got, p)
	}

	if len(got) != len(paths) {
		t.Errorf("got %d paths, want %d", len(got), len(paths))
	}
}

// TestPathsToChan_RespectsCtxCancellation verifies that pathsToChan stops
// sending and closes the channel when the context is cancelled.
func TestPathsToChan_RespectsCtxCancellation(t *testing.T) {
	// Use a large slice so the goroutine is mid-send when we cancel.
	paths := make([]string, 1000)
	for i := range paths {
		paths[i] = fmt.Sprintf("/track_%04d.mp3", i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := pathsToChan(ctx, paths)

	// Consume one value then cancel, simulating an aborted pipeline.
	<-ch
	cancel()

	// Drain the channel; it must close within a reasonable time.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel was not closed after context cancellation; possible goroutine leak")
	}
}

// TestFilteredWalk_MultipleRootsDeletedFileReported verifies that a path in
// the known map from one root and not the other is correctly reported as
// deleted when it no longer exists on disk.
func TestFilteredWalk_MultipleRootsDeletedFileReported(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	paths1 := makeAudioFiles(t, dir1, 2)
	paths2 := makeAudioFiles(t, dir2, 2)

	known := make(map[string]KnownFile)
	for _, p := range append(paths1, paths2...) {
		info, _ := os.Stat(p)
		known[p] = KnownFile{ModTime: info.ModTime().Unix(), SizeBytes: info.Size()}
	}

	// Delete one file from each root.
	if err := os.Remove(paths1[1]); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.Remove(paths2[0]); err != nil {
		t.Fatalf("remove: %v", err)
	}

	_, deleted, err := FilteredWalk(context.Background(), []string{dir1, dir2}, nil, known, nil)
	if err != nil {
		t.Fatalf("FilteredWalk: %v", err)
	}

	if len(deleted) != 2 {
		t.Errorf("deleted: got %d paths, want 2: %v", len(deleted), deleted)
	}
	deletedSet := make(map[string]bool, len(deleted))
	for _, p := range deleted {
		deletedSet[p] = true
	}
	if !deletedSet[paths1[1]] {
		t.Errorf("expected %q in deleted, got %v", paths1[1], deleted)
	}
	if !deletedSet[paths2[0]] {
		t.Errorf("expected %q in deleted, got %v", paths2[0], deleted)
	}
}
