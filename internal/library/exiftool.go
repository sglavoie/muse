package library

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
)

// batchSize is the number of files per exiftool invocation.
const batchSize = 500

// MetadataExtractor extracts audio metadata from a batch of hashed files.
type MetadataExtractor interface {
	Extract(files []HashedFile) ([]Track, error)
}

// ExifToolExtractor implements MetadataExtractor using the exiftool binary.
type ExifToolExtractor struct{}

// exifRecord mirrors the per-file JSON object emitted by exiftool -json.
type exifRecord struct {
	SourceFile string  `json:"SourceFile"`
	Artist     string  `json:"Artist"`
	Title      string  `json:"Title"`
	Album      string  `json:"Album"`
	Duration   float64 `json:"Duration"`
}

// Extract invokes exiftool -json on the provided files and returns one Track
// per file. Each Track carries the hash from its corresponding HashedFile.
// The -Duration# flag requests numeric (seconds) duration output.
func (e ExifToolExtractor) Extract(files []HashedFile) ([]Track, error) {
	if len(files) == 0 {
		return nil, nil
	}

	fileByPath := make(map[string]HashedFile, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
		fileByPath[f.Path] = f
	}

	args := make([]string, 0, 5+len(paths))
	args = append(args, "-json", "-Artist", "-Title", "-Album", "-Duration#")
	args = append(args, paths...)

	out, err := exec.Command("exiftool", args...).Output()
	if err != nil {
		// exiftool exits non-zero for warnings (unknown tags, minor file
		// issues) but still writes valid JSON to stdout. Use the output
		// when available so a single problematic file does not discard
		// the entire batch and trigger expensive binary-retry subdivision.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || len(out) == 0 {
			return nil, err
		}
		// Fall through: we have stdout from exiftool despite the non-zero exit.
	}

	var records []exifRecord
	if err := json.Unmarshal(out, &records); err != nil {
		return nil, err
	}

	tracks := make([]Track, 0, len(records))
	for _, r := range records {
		hf, ok := fileByPath[r.SourceFile]
		if !ok {
			slog.Warn("exiftool: SourceFile not in input set, skipping",
				"source_file", r.SourceFile)
			continue
		}
		tracks = append(tracks, Track{
			Path:      r.SourceFile,
			Hash:      hf.Hash,
			Artist:    r.Artist,
			Title:     r.Title,
			Album:     r.Album,
			Duration:  r.Duration,
			ModTime:   hf.ModTime,
			SizeBytes: hf.SizeBytes,
		})
	}
	return tracks, nil
}

// ExtractAllTags runs exiftool -json on a single file and returns the full
// set of tags as a map. This is used by the track-info overlay to display
// detailed metadata without being limited to the fields in exifRecord.
func ExtractAllTags(path string) (map[string]any, error) {
	out, err := exec.Command("exiftool", "-json", path).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || len(out) == 0 {
			return nil, err
		}
	}
	var records []map[string]any
	if err := json.Unmarshal(out, &records); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("exiftool returned no records")
	}
	return records[0], nil
}

// extractWithBinaryRetry calls extractor.Extract for the given files. If the
// call fails (e.g. invalid JSON caused by a corrupt file poisoning the batch),
// the batch is split in half and each half is retried recursively. When a
// single-file batch fails, that file is skipped so at most one track loses
// metadata per corrupt file.
func extractWithBinaryRetry(extractor MetadataExtractor, files []HashedFile) []Track {
	tracks, err := extractor.Extract(files)
	if err == nil {
		return tracks
	}
	if len(files) == 1 {
		// Corrupt single file: fall back to filename-derived metadata so the
		// track is still included in the library.
		slog.Warn("metadata extraction failed, using filename fallback",
			"path", files[0].Path,
			"error", err,
		)
		base := filepath.Base(files[0].Path)
		ext := filepath.Ext(base)
		stem := base[:len(base)-len(ext)]
		return []Track{{
			Path:      files[0].Path,
			Hash:      files[0].Hash,
			Artist:    "Unknown Artist",
			Title:     stem,
			ModTime:   files[0].ModTime,
			SizeBytes: files[0].SizeBytes,
		}}
	}
	mid := len(files) / 2
	left := extractWithBinaryRetry(extractor, files[:mid])
	right := extractWithBinaryRetry(extractor, files[mid:])
	return append(left, right...)
}

// ExtractBatches reads HashedFile values from in, groups them into batches of
// batchSize (~500), and calls extractor.Extract on each batch via binary-search
// retry. Track values are sent to the returned channel, which is closed once
// all batches have been processed. If a batch produces invalid JSON due to a
// corrupt file, binary-search subdivision isolates the corrupt file so that at
// most one track loses metadata per corrupt file.
func ExtractBatches(extractor MetadataExtractor, in <-chan HashedFile) <-chan Track {
	out := make(chan Track, runtime.NumCPU())
	go func() {
		defer close(out)
		batch := make([]HashedFile, 0, batchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			slog.Debug("processing metadata batch", "size", len(batch))
			tracks := extractWithBinaryRetry(extractor, batch)
			slog.Info("scan batch complete", "tracks", len(tracks))
			for _, t := range tracks {
				out <- t
			}
			batch = batch[:0]
		}
		for hf := range in {
			batch = append(batch, hf)
			if len(batch) >= batchSize {
				flush()
			}
		}
		flush()
	}()
	return out
}
