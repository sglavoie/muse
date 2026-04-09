package library

import (
	"context"
	"log/slog"
	"testing"
)

// TestFallback_SingleFileFailureInsertsFallback verifies that when ExifTool
// fails to extract metadata for a single file, the track is still returned with
// Artist="Unknown Artist" and Title set to the filename stem. It also verifies
// that a slog.Warn is emitted and the track is present in the returned slice.
func TestFallback_SingleFileFailureInsertsFallback(t *testing.T) {
	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	files := []HashedFile{{Path: "/music/artist/some song.flac", Hash: "deadbeef"}}
	tracks := extractWithBinaryRetry(context.Background(), &alwaysFailExtractor{}, files)

	if len(tracks) != 1 {
		t.Fatalf("expected 1 fallback track, got %d", len(tracks))
	}
	tr := tracks[0]

	if tr.Artist != "Unknown Artist" {
		t.Errorf("artist: want %q, got %q", "Unknown Artist", tr.Artist)
	}
	if tr.Title != "some song" {
		t.Errorf("title: want %q (filename stem), got %q", "some song", tr.Title)
	}
	if tr.Path != files[0].Path {
		t.Errorf("path: want %q, got %q", files[0].Path, tr.Path)
	}
	if tr.Hash != files[0].Hash {
		t.Errorf("hash: want %q, got %q", files[0].Hash, tr.Hash)
	}

	warnCount := 0
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			warnCount++
		}
	}
	if warnCount == 0 {
		t.Error("expected at least one slog.Warn for failed extraction, got none")
	}
}

// TestBinaryRetry_EmptyMetadataFallback verifies that when an extractor
// succeeds but returns tracks with empty Title and Artist, extractWithBinaryRetry
// applies filename-derived fallback metadata.
func TestBinaryRetry_EmptyMetadataFallback(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/Estas Tonne/INTERNAL FLIGHT (REMASTERED).opus", Hash: "abc123"},
		{Path: "/music/artist/album/song.flac", Hash: "def456"},
	}

	tracks := extractWithBinaryRetry(context.Background(), &emptyFieldExtractor{}, files)

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if tracks[0].Title != "INTERNAL FLIGHT (REMASTERED)" {
		t.Errorf("track[0] title: want %q, got %q", "INTERNAL FLIGHT (REMASTERED)", tracks[0].Title)
	}
	if tracks[0].Artist != "Unknown Artist" {
		t.Errorf("track[0] artist: want %q, got %q", "Unknown Artist", tracks[0].Artist)
	}
	if tracks[1].Title != "song" {
		t.Errorf("track[1] title: want %q, got %q", "song", tracks[1].Title)
	}
	if tracks[1].Artist != "Unknown Artist" {
		t.Errorf("track[1] artist: want %q, got %q", "Unknown Artist", tracks[1].Artist)
	}
}

// TestBinaryRetry_PartialMetadataPreserved verifies that when an extractor
// returns a track with a valid Artist but empty Title, only the missing field
// gets the fallback while the existing Artist is preserved.
func TestBinaryRetry_PartialMetadataPreserved(t *testing.T) {
	ext := &partialFieldExtractor{artist: "Estas Tonne"}
	files := []HashedFile{{Path: "/music/INTERNAL FLIGHT.opus", Hash: "aaa"}}

	tracks := extractWithBinaryRetry(context.Background(), ext, files)

	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Artist != "Estas Tonne" {
		t.Errorf("artist: want %q, got %q", "Estas Tonne", tracks[0].Artist)
	}
	if tracks[0].Title != "INTERNAL FLIGHT" {
		t.Errorf("title: want %q (filename stem), got %q", "INTERNAL FLIGHT", tracks[0].Title)
	}
}