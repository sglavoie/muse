//go:build integration

package library

import (
	"context"
	"encoding/binary"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeMinimalWAV returns the bytes of a valid, minimal PCM WAV file with
// 0 data samples (mono, 8-bit, 8000 Hz). Exiftool parses this without error.
func makeMinimalWAV() []byte {
	const (
		audioFormatPCM uint16 = 1
		numChannels    uint16 = 1
		sampleRate     uint32 = 8000
		bitsPerSample  uint16 = 8
	)
	byteRate := uint32(sampleRate) * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8

	buf := make([]byte, 44) // RIFF(12) + fmt(24) + data(8)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], 36) // chunk size = total - 8
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16) // fmt sub-chunk size
	binary.LittleEndian.PutUint16(buf[20:], audioFormatPCM)
	binary.LittleEndian.PutUint16(buf[22:], numChannels)
	binary.LittleEndian.PutUint32(buf[24:], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:], byteRate)
	binary.LittleEndian.PutUint16(buf[32:], blockAlign)
	binary.LittleEndian.PutUint16(buf[34:], bitsPerSample)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], 0) // 0 data bytes
	return buf
}

// TestExifToolExtractor_RealInvocation creates a small batch of minimal WAV
// files and calls ExifToolExtractor.Extract, asserting that the real exiftool
// binary returns parseable JSON for every file in the batch.
func TestExifToolExtractor_RealInvocation(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}

	dir := t.TempDir()
	const n = 5
	files := make([]HashedFile, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("track%02d.wav", i))
		writeFile(t, p, makeMinimalWAV())
		files[i] = HashedFile{Path: p, Hash: fmt.Sprintf("testhash%02d", i)}
	}

	ext := ExifToolExtractor{}
	tracks, err := ext.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("ExifToolExtractor.Extract returned error: %v", err)
	}
	if len(tracks) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(tracks))
	}
	// Build an expected path→hash map from the inputs.
	wantHash := make(map[string]string, n)
	for _, f := range files {
		wantHash[f.Path] = f.Hash
	}
	for _, tr := range tracks {
		if tr.Path == "" {
			t.Error("Track.Path must not be empty")
			continue
		}
		if got, ok := wantHash[tr.Path]; !ok {
			t.Errorf("unexpected Track.Path %q", tr.Path)
		} else if tr.Hash != got {
			t.Errorf("Track %q: want hash %q, got %q", tr.Path, got, tr.Hash)
		}
	}
}

// TestExifToolExtractor_ExitErrorWithValidOutput verifies that Extract
// succeeds when exiftool exits with a non-zero status (because some input
// files don't exist) but still writes valid JSON to stdout for the files it
// could process. Before the fix, the entire batch was discarded; after the
// fix, valid tracks from stdout are returned.
func TestExifToolExtractor_ExitErrorWithValidOutput(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}

	dir := t.TempDir()
	// One real file and one non-existent file. exiftool will exit non-zero
	// because of the missing file, but still output JSON for the real one.
	realPath := filepath.Join(dir, "real.wav")
	writeFile(t, realPath, makeMinimalWAV())

	fakePath := filepath.Join(dir, "does_not_exist.wav")

	files := []HashedFile{
		{Path: realPath, Hash: "real-hash"},
		{Path: fakePath, Hash: "fake-hash"},
	}

	ext := ExifToolExtractor{}
	tracks, err := ext.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("Extract should succeed when exiftool exits non-zero but produces valid stdout, got error: %v", err)
	}
	// At minimum the real file should be in the results.
	foundReal := false
	for _, tr := range tracks {
		if tr.Path == realPath {
			foundReal = true
			if tr.Hash != "real-hash" {
				t.Errorf("real track hash: want %q, got %q", "real-hash", tr.Hash)
			}
		}
	}
	if !foundReal {
		t.Errorf("expected real file %q in results, got %d tracks: %v", realPath, len(tracks), tracks)
	}
}

// TestExtractAllTags_RealInvocation verifies that ExtractAllTags returns a
// non-empty map of tags for a valid audio file.
func TestExtractAllTags_RealInvocation(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "test.wav")
	writeFile(t, p, makeMinimalWAV())

	tags, err := ExtractAllTags(context.Background(), p)
	if err != nil {
		t.Fatalf("ExtractAllTags returned error: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("expected non-empty tags map")
	}
	// A WAV file should have at least FileType and MIMEType.
	if _, ok := tags["FileType"]; !ok {
		t.Error("expected FileType tag in result")
	}
	if _, ok := tags["MIMEType"]; !ok {
		t.Error("expected MIMEType tag in result")
	}
	// SourceFile should match the input path.
	if sf, ok := tags["SourceFile"]; ok {
		if sf != p {
			t.Errorf("SourceFile: want %q, got %q", p, sf)
		}
	}
}

// TestExtractAllTags_NonexistentFile verifies that ExtractAllTags returns an
// error for a file that does not exist.
func TestExtractAllTags_NonexistentFile(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}

	_, err := ExtractAllTags(context.Background(), "/nonexistent/path/file.wav")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// TestExifToolExtractor_BatchSize verifies that ExtractBatches correctly
// dispatches a real exiftool batch when exiftoolBatchSize files accumulate.
func TestExifToolExtractor_BatchSize(t *testing.T) {
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}

	dir := t.TempDir()
	const n = exiftoolBatchSize + 2
	in := make(chan HashedFile, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("batch%04d.wav", i))
		writeFile(t, p, makeMinimalWAV())
		in <- HashedFile{Path: p, Hash: fmt.Sprintf("bh%04d", i)}
	}
	close(in)

	ext := ExifToolExtractor{}
	var got []Track
	for tr := range ExtractBatches(context.Background(), ext, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks from full batch run, got %d", n, len(got))
	}
}
