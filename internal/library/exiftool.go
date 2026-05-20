package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// exiftoolBatchSize is the number of files per exiftool invocation.
// Independent from the DB upsert batch size in main.go.
const exiftoolBatchSize = 500

// MetadataExtractor extracts audio metadata from a batch of hashed files.
type MetadataExtractor interface {
	Extract(ctx context.Context, files []HashedFile) ([]Track, error)
}

// ExifToolExtractor implements MetadataExtractor using the exiftool binary.
type ExifToolExtractor struct{}

// Compile-time check that ExifToolExtractor implements MetadataExtractor.
var _ MetadataExtractor = (*ExifToolExtractor)(nil)

// exifRecord mirrors the per-file JSON object emitted by exiftool -json.
// A custom UnmarshalJSON handles type mismatches that exiftool can produce
// (e.g. Duration as a string like "0:03:45", or Artist/Genre as arrays)
// so that a single unusual file does not fail json.Unmarshal for the entire
// batch and trigger expensive binary-retry subdivision.
type exifRecord struct {
	SourceFile string
	Artist     string
	Title      string
	Album      string
	Genre      string
	Duration   float64
	Track      string
	DiscNumber string
}

// UnmarshalJSON decodes an exiftool JSON record, coercing fields that may
// arrive as unexpected types:
//   - string fields (Artist, Title, Album, Genre) accept arrays by joining
//     elements with "; ".
//   - Duration accepts both numeric values and "HH:MM:SS" / "MM:SS" strings.
func (r *exifRecord) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.SourceFile = coerceString(raw["SourceFile"])
	r.Artist = coerceString(raw["Artist"])
	r.Title = coerceString(raw["Title"])
	r.Album = coerceString(raw["Album"])
	r.Genre = coerceString(raw["Genre"])
	r.Duration = coerceFloat(raw["Duration"])
	r.Track = coerceString(raw["Track"])
	r.DiscNumber = coerceString(raw["DiscNumber"])
	return nil
}

// coerceString extracts a string from a JSON value that may be a string,
// a number, or an array of strings. Arrays are joined with "; ".
func coerceString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first (most common).
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Try array of strings (e.g. multi-value Genre tags).
	var arr []string
	if json.Unmarshal(raw, &arr) == nil {
		return strings.Join(arr, "; ")
	}
	// Try number (rare for text fields but possible).
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return fmt.Sprintf("%g", f)
	}
	return ""
}

// coerceFloat extracts a float64 from a JSON value that may be a number or
// a duration string. Supported string formats: "HH:MM:SS", "MM:SS", and
// plain numeric strings like "123.45".
func coerceFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	// Try number first (most common with -Duration#).
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	// Try string: could be "HH:MM:SS", "MM:SS", or a numeric string.
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0
	}
	return parseDurationString(s)
}

// parseDurationString converts a duration string to seconds. It handles:
//   - "HH:MM:SS" or "HH:MM:SS.mmm"
//   - "MM:SS" or "MM:SS.mmm"
//   - plain numeric strings like "123.45"
func parseDurationString(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Try colon-separated format.
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3: // HH:MM:SS
		h, err1 := strconv.ParseFloat(parts[0], 64)
		m, err2 := strconv.ParseFloat(parts[1], 64)
		sec, err3 := strconv.ParseFloat(parts[2], 64)
		if err1 == nil && err2 == nil && err3 == nil {
			return h*3600 + m*60 + sec
		}
	case 2: // MM:SS
		m, err1 := strconv.ParseFloat(parts[0], 64)
		sec, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil {
			return m*60 + sec
		}
	}
	// Try plain numeric.
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return 0
}

// parseTrackNumber parses a track/disc number string from exiftool, which may
// be a plain integer ("3"), a fraction ("3/12"), or empty. Returns 0 on failure.
func parseTrackNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		// Negative track/disc numbers are nonsensical; return the 0 sentinel
		// (unknown) so downstream sorting/display does not surface them.
		return 0
	}
	return n
}

// Extract invokes exiftool -json on the provided files and returns one Track
// per file. Each Track carries the hash from its corresponding HashedFile.
// The -Duration# flag requests numeric (seconds) duration output.
// extractTimeout is the maximum time allowed for a single exiftool invocation.
const extractTimeout = 30 * time.Second

func (e ExifToolExtractor) Extract(ctx context.Context, files []HashedFile) ([]Track, error) {
	if len(files) == 0 {
		return nil, nil
	}

	fileByPath := make(map[string]HashedFile, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
		fileByPath[f.Path] = f
	}

	args := make([]string, 0, 8+len(paths))
	args = append(args, "-json", "-Artist", "-Title", "-Album", "-Genre", "-Duration#", "-Track", "-DiscNumber")
	args = append(args, paths...)

	ctx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "exiftool", args...).Output()
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
			Path:        r.SourceFile,
			Hash:        hf.Hash,
			Artist:      r.Artist,
			Title:       r.Title,
			Album:       r.Album,
			Genre:       r.Genre,
			Duration:    r.Duration,
			ModTime:     hf.ModTime,
			SizeBytes:   hf.SizeBytes,
			TrackNumber: parseTrackNumber(r.Track),
			DiscNumber:  parseTrackNumber(r.DiscNumber),
		})
	}
	return tracks, nil
}

// extractAllTagsTimeout is the maximum time allowed for a single-file tag extraction.
const extractAllTagsTimeout = 10 * time.Second

// ExtractAllTags runs exiftool -json on a single file and returns the full
// set of tags as a map. This is used by the track-info overlay to display
// detailed metadata without being limited to the fields in exifRecord.
func ExtractAllTags(ctx context.Context, path string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, extractAllTagsTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "exiftool", "-json", path).Output()
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
func extractWithBinaryRetry(ctx context.Context, extractor MetadataExtractor, files []HashedFile) []Track {
	tracks, err := extractor.Extract(ctx, files)
	if err == nil {
		for i := range tracks {
			// Trim whitespace-only metadata so it's treated the same as
			// missing tags (broken taggers occasionally write " " or "\t").
			tracks[i].Title = strings.TrimSpace(tracks[i].Title)
			tracks[i].Artist = strings.TrimSpace(tracks[i].Artist)
			tracks[i].Album = strings.TrimSpace(tracks[i].Album)
			tracks[i].Genre = strings.TrimSpace(tracks[i].Genre)
			if tracks[i].Title == "" {
				base := filepath.Base(tracks[i].Path)
				tracks[i].Title = base[:len(base)-len(filepath.Ext(base))]
			}
			if tracks[i].Artist == "" {
				tracks[i].Artist = "Unknown Artist"
			}
		}
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
	left := extractWithBinaryRetry(ctx, extractor, files[:mid])
	right := extractWithBinaryRetry(ctx, extractor, files[mid:])
	return append(left, right...)
}

// ExtractBatches reads HashedFile values from in, groups them into batches of
// exiftoolBatchSize (~500), and calls extractor.Extract on each batch via binary-search
// retry. Track values are sent to the returned channel, which is closed once
// all batches have been processed. If a batch produces invalid JSON due to a
// corrupt file, binary-search subdivision isolates the corrupt file so that at
// most one track loses metadata per corrupt file.
func ExtractBatches(ctx context.Context, extractor MetadataExtractor, in <-chan HashedFile) <-chan Track {
	out := make(chan Track, runtime.NumCPU())
	go func() {
		defer close(out)
		batch := make([]HashedFile, 0, exiftoolBatchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			slog.Debug("processing metadata batch", "size", len(batch))
			tracks := extractWithBinaryRetry(ctx, extractor, batch)
			slog.Info("scan batch complete", "tracks", len(tracks))
			for _, t := range tracks {
				out <- t
			}
			batch = batch[:0]
		}
		for hf := range in {
			batch = append(batch, hf)
			if len(batch) >= exiftoolBatchSize {
				flush()
			}
		}
		flush()
	}()
	return out
}
