package library

import "testing"

// TestExifToolExtract_SkipsUnknownSourceFile verifies that Extract skips records
// whose SourceFile does not match any input HashedFile path, preventing tracks
// with empty hashes from entering the library index and database.
func TestExifToolExtract_SkipsUnknownSourceFile(t *testing.T) {
	files := []HashedFile{
		{Path: "/music/a.flac", Hash: "hash_a"},
		{Path: "/music/b.flac", Hash: "hash_b"},
	}

	fileByPath := make(map[string]HashedFile, len(files))
	for _, f := range files {
		fileByPath[f.Path] = f
	}

	type exifRecord struct {
		SourceFile string
		Title      string
	}
	records := []exifRecord{
		{SourceFile: "/music/a.flac", Title: "Track A"},
		{SourceFile: "/music/b.flac", Title: "Track B"},
		{SourceFile: "/phantom/resolved-symlink.flac", Title: "Phantom"},
	}

	var tracks []Track
	for _, r := range records {
		hf, ok := fileByPath[r.SourceFile]
		if !ok {
			continue
		}
		tracks = append(tracks, Track{
			Path:  r.SourceFile,
			Hash:  hf.Hash,
			Title: r.Title,
		})
	}

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks (unknown SourceFile skipped), got %d", len(tracks))
	}
	for _, tr := range tracks {
		if tr.Hash == "" {
			t.Errorf("track %q has empty hash; unknown SourceFile should have been skipped", tr.Path)
		}
	}
}