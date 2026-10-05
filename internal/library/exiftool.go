package library

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/safego"
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
	SourceFile  string
	Artist      string
	Title       string
	Album       string
	AlbumArtist string
	Genre       string
	Duration    float64
	Track       string
	DiscNumber  string
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
	r.AlbumArtist = exifAlbumArtist(raw)
	r.Genre = coerceString(raw["Genre"])
	r.Duration = coerceFloat(raw["Duration"])
	r.Track = coerceString(raw["Track"])
	r.DiscNumber = coerceString(raw["DiscNumber"])
	return nil
}

// coerceString extracts a string from a JSON value that may be a string,
// a number, or an array of strings/numbers. Arrays are joined with "; ".
// Heterogeneous arrays (e.g. ["Rock", 42, "Pop"]) are decoded element-by-element
// so a single odd element does not discard the rest of the values.
func coerceString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string first (most common).
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	// Try array; coerce each element recursively so mixed-type arrays don't
	// lose data (exiftool occasionally emits ["a", null, 1] for messy tags).
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if v := coerceString(el); v != "" {
				parts = append(parts, v)
			}
		}
		return strings.Join(parts, "; ")
	}
	// Try number (rare for text fields but possible). Keep the literal JSON
	// text: a float round-trip would turn a tag of 1000000 into "1e+06" and
	// 0.10 into "0.1".
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	// Try bool as a final fallback (exiftool will occasionally emit booleans
	// for tags like "Lossless"; we don't want to drop them on the floor).
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		if b {
			return "true"
		}
		return "false"
	}
	return ""
}

// coerceFloat extracts a float64 from a JSON value that may be a number or
// a duration string. Supported string formats: "HH:MM:SS", "MM:SS", and
// plain numeric strings like "123.45". Negative, NaN, and Inf values are
// normalised to 0 so they cannot poison downstream duration arithmetic.
func coerceFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	// Try number first (most common with -Duration#).
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return sanitizeDuration(f)
	}
	// Try string: could be "HH:MM:SS", "MM:SS", or a numeric string.
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return 0
	}
	return sanitizeDuration(parseDurationString(s))
}

// sanitizeDuration coerces non-finite or negative durations to 0.
func sanitizeDuration(f float64) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return f
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

// filenameStem returns the base name of path with the audio extension stripped.
// Unlike a naive base[:len(base)-len(filepath.Ext(base))] this returns the
// full base name when the extension consumes the entire name (e.g. ".mp3"
// or ".flac"), since callers use the stem as a user-visible Title fallback
// and an empty string is worse than the dot-prefixed name.
func filenameStem(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if stem == "" {
		return base
	}
	return stem
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
	args = append(args, "-json", "-Artist", "-Title", "-Album", "-AlbumArtist", "-Band", "-Genre", "-Duration#", "-Track", "-DiscNumber")
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
			AlbumArtist: r.AlbumArtist,
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

// extractAllTagsExifTool runs exiftool -json on a single file and returns the
// full set of tags as a map. It is the exiftool half of ExtractAllTags (see
// alltags.go), which is what the track-info overlay calls.
func extractAllTagsExifTool(ctx context.Context, path string) (map[string]any, error) {
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
	if len(files) == 0 {
		return nil
	}
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
				tracks[i].Title = filenameStem(tracks[i].Path)
			}
			if tracks[i].Artist == "" {
				tracks[i].Artist = UnknownArtist
			}
		}
		return tracks
	}
	if ctx.Err() != nil {
		// Scan was cancelled: bail without generating filename fallbacks for
		// the remaining files. The caller is shutting down and any partial
		// results would just be discarded.
		return nil
	}
	if len(files) == 1 {
		// Corrupt single file: fall back to filename-derived metadata so the
		// track is still included in the library.
		slog.Warn("metadata extraction failed, using filename fallback",
			"path", files[0].Path,
			"error", err,
		)
		return []Track{{
			Path:      files[0].Path,
			Hash:      files[0].Hash,
			Artist:    UnknownArtist,
			Title:     filenameStem(files[0].Path),
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
		// The extractor shells out and parses third-party JSON, so this is the
		// likeliest panic site in the scan pipeline. close(out) stays deferred:
		// a recovered producer that left out open would block the scan bridge
		// forever, which is worse than the crash it replaces.
		defer safego.Recover("library.ExtractBatches")
		defer close(out)
		batch := make([]HashedFile, 0, exiftoolBatchSize)
		cancelled := false
		flush := func() {
			if len(batch) == 0 {
				return
			}
			slog.Debug("processing metadata batch", "size", len(batch))
			tracks := extractWithBinaryRetry(ctx, extractor, batch)
			slog.Info("scan batch complete", "tracks", len(tracks))
			batch = batch[:0]
			for _, t := range tracks {
				if cancelled {
					return
				}
				select {
				case out <- t:
				case <-ctx.Done():
					// Stop trying to emit tracks, but keep draining `in` so
					// the upstream hash-worker goroutines blocked on
					// `out <- HashedFile{...}` can exit and close `in`.
					// Returning early here without draining would leak the
					// entire pipeline (hash workers + walker).
					cancelled = true
					return
				}
			}
		}
		for hf := range in {
			if cancelled {
				continue // drain `in` so upstream goroutines can exit
			}
			batch = append(batch, hf)
			if len(batch) >= exiftoolBatchSize {
				flush()
			}
		}
		if !cancelled {
			flush()
		}
	}()
	return out
}
