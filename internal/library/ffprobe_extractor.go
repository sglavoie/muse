package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/safego"
)

// ffprobeExtractTimeout is the maximum time allowed for a single ffprobe
// invocation. Unlike exiftool, ffprobe has no batch mode: this budget is
// per file, not per batch.
const ffprobeExtractTimeout = 10 * time.Second

// FFProbeExtractor implements MetadataExtractor using the ffprobe binary.
//
// ffprobe accepts exactly one input file per invocation, so where
// ExifToolExtractor amortises a single process over exiftoolBatchSize (500)
// files, this extractor spawns one process per file. To keep that from
// serialising a scan it fans the batch out over a bounded worker pool sized to
// runtime.NumCPU(), mirroring HashWorkers. Scans are still materially slower
// than the exiftool path — process startup dominates, and it is paid per track
// rather than per batch.
type FFProbeExtractor struct{}

// Compile-time check that FFProbeExtractor implements MetadataExtractor.
var _ MetadataExtractor = (*FFProbeExtractor)(nil)

// ffprobeOutput mirrors the JSON emitted by
// `ffprobe -print_format json -show_entries format=duration:format_tags:stream_tags`.
//
// Tags are decoded as raw messages so coerceString can apply the same
// normalisation the exiftool path uses (arrays joined with "; ", numbers
// stringified). ffprobe writes tag values as strings today, but decoding
// leniently means one oddly-typed tag cannot fail the whole file and demote it
// to the filename fallback.
type ffprobeOutput struct {
	Format struct {
		Duration json.RawMessage            `json:"duration"`
		Tags     map[string]json.RawMessage `json:"tags"`
	} `json:"format"`
	Streams []struct {
		Tags map[string]json.RawMessage `json:"tags"`
	} `json:"streams"`
}

// Extract probes each file with ffprobe and returns one Track per input file,
// in input order. Results are correlated to their HashedFile by slice index
// rather than by a path returned from the tool, since each worker already knows
// which input it was handed.
//
// A file that fails to probe is not dropped: it yields a Track carrying its
// Path/Hash/ModTime/SizeBytes plus "Unknown Artist" and the filename stem as
// Title, matching the fallback the exiftool path produces in
// extractWithBinaryRetry. Only context cancellation produces an error.
func (e FFProbeExtractor) Extract(ctx context.Context, files []HashedFile) ([]Track, error) {
	if len(files) == 0 {
		return nil, nil
	}

	tracks := make([]Track, len(files))
	indexes := make(chan int)

	workers := min(runtime.NumCPU(), len(files))
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			// Recovery is per item (see probeFileRecovered), so a malformed
			// response cannot abandon the rest of this worker's queue. This
			// worker-level recover is only a backstop for a panic raised by the
			// loop itself; the fill pass after wg.Wait repairs any slot it left
			// unwritten, so a lost worker can never emit a zero-value Track.
			defer safego.Recover("library.ffprobeWorker")
			// Each worker writes only to tracks[i] for the indexes it pulls,
			// so the shared slice needs no lock.
			for i := range indexes {
				tracks[i] = probeFileRecovered(ctx, files[i])
			}
		})
	}

feed:
	for i := range files {
		select {
		case indexes <- i:
		case <-ctx.Done():
			// Stop handing out work; the loop below still closes the channel
			// and waits so no worker is left blocked.
			break feed
		}
	}
	close(indexes)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		// Scan was cancelled. Discard the partial results rather than reporting
		// filename fallbacks for files that were never probed — the caller is
		// shutting down and extractWithBinaryRetry drops cancelled batches.
		return nil, err
	}

	// Every index was handed out (the feed loop only breaks early on
	// cancellation, which returned above), so any slot still zero-valued means a
	// worker died before writing it. Emitting those as-is would insert tracks
	// with an empty path and hash into the library, so they are repaired with
	// the same filename fallback an unprobeable file gets.
	repaired := 0
	for i := range tracks {
		if tracks[i].Path == "" {
			tracks[i] = fallbackTrack(files[i])
			repaired++
		}
	}
	if repaired > 0 {
		slog.Error("ffprobe: worker panicked mid-batch, metadata degraded to filename fallback",
			"files", repaired,
			"batch", len(files),
		)
	}
	return tracks, nil
}

// probeFileRecovered probes one file, converting a panic in the probe into the
// same filename fallback an unprobeable file produces. Recovery is per item
// rather than per worker so a single bad file degrades only itself: the worker
// keeps draining indexes, and no accepted file is left as a zero-value Track.
func probeFileRecovered(ctx context.Context, f HashedFile) (t Track) {
	defer safego.RecoverTo("library.ffprobeProbeFile", func(error) {
		t = fallbackTrack(f)
	})
	return probeFileFn(ctx, f)
}

// probeFileFn is the per-file probe, indirected so tests can inject a panicking
// probe without spawning a real ffprobe.
var probeFileFn = probeFile

// fallbackTrack returns the filename-derived Track used when metadata cannot be
// read, matching the fallback extractWithBinaryRetry produces on the exiftool
// path. It always sets Path, which is what makes an unwritten slot detectable.
func fallbackTrack(f HashedFile) Track {
	return Track{
		Path:      f.Path,
		Hash:      f.Hash,
		Artist:    "Unknown Artist",
		Title:     filenameStem(f.Path),
		ModTime:   f.ModTime,
		SizeBytes: f.SizeBytes,
	}
}

// probeFile runs ffprobe on a single file and converts the result to a Track.
// It never returns an error: an unprobeable file (missing, corrupt, or not
// actually audio) falls back to filename-derived metadata so the track still
// reaches the library.
func probeFile(ctx context.Context, f HashedFile) Track {
	out, err := runFFProbe(ctx, f.Path)
	if err != nil {
		slog.Warn("ffprobe: metadata extraction failed, using filename fallback",
			"path", f.Path,
			"error", err,
		)
		return fallbackTrack(f)
	}

	tags := mergeFFProbeTags(out)
	return Track{
		Path:        f.Path,
		Hash:        f.Hash,
		Artist:      lookupTag(tags, "artist"),
		Title:       lookupTag(tags, "title"),
		Album:       lookupTag(tags, "album"),
		Genre:       lookupTag(tags, "genre"),
		Duration:    sanitizeDuration(parseDurationString(coerceString(out.Format.Duration))),
		ModTime:     f.ModTime,
		SizeBytes:   f.SizeBytes,
		TrackNumber: parseTrackNumber(lookupTag(tags, "track", "tracknumber", "track_number")),
		DiscNumber:  parseTrackNumber(lookupTag(tags, "disc", "discnumber", "disc_number", "part_of_set")),
	}
}

// runFFProbe invokes ffprobe on one file and decodes its JSON output.
func runFFProbe(ctx context.Context, path string) (ffprobeOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, ffprobeExtractTimeout)
	defer cancel()

	// Tags live at format level for mp3/flac/mp4 but at stream level for Ogg
	// containers (opus/vorbis), so both sections are requested and merged.
	raw, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_entries", "format=duration:format_tags:stream_tags",
		path,
	).Output()
	if err != nil {
		return ffprobeOutput{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var out ffprobeOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return ffprobeOutput{}, fmt.Errorf("ffprobe %s: parse output: %w", path, err)
	}
	return out, nil
}

// mergeFFProbeTags flattens format-level and stream-level tags into a single
// lowercase-keyed map. Tag key casing varies by container ("TRACK" in Vorbis
// comments, "track" in MP4), so every key is folded before lookup. Format-level
// tags win over stream-level ones when both carry the same key.
func mergeFFProbeTags(out ffprobeOutput) map[string]json.RawMessage {
	tags := make(map[string]json.RawMessage, len(out.Format.Tags))
	for k, v := range out.Format.Tags {
		tags[strings.ToLower(k)] = v
	}
	for _, s := range out.Streams {
		for k, v := range s.Tags {
			k = strings.ToLower(k)
			if _, ok := tags[k]; !ok {
				tags[k] = v
			}
		}
	}
	return tags
}

// lookupTag returns the first non-empty value among the given lowercase tag
// keys. The aliases exist because containers disagree on spelling
// ("tracknumber" vs "track", "discnumber" vs "disc" vs "part_of_set").
func lookupTag(tags map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if v, ok := tags[k]; ok {
			if s := coerceString(v); s != "" {
				return s
			}
		}
	}
	return ""
}
