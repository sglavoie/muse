package library

import (
	"encoding/json"
	"os/exec"
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

	hashByPath := make(map[string]string, len(files))
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
		hashByPath[f.Path] = f.Hash
	}

	args := make([]string, 0, 5+len(paths))
	args = append(args, "-json", "-Artist", "-Title", "-Album", "-Duration#")
	args = append(args, paths...)

	out, err := exec.Command("exiftool", args...).Output()
	if err != nil {
		return nil, err
	}

	var records []exifRecord
	if err := json.Unmarshal(out, &records); err != nil {
		return nil, err
	}

	tracks := make([]Track, 0, len(records))
	for _, r := range records {
		tracks = append(tracks, Track{
			Path:     r.SourceFile,
			Hash:     hashByPath[r.SourceFile],
			Artist:   r.Artist,
			Title:    r.Title,
			Album:    r.Album,
			Duration: r.Duration,
		})
	}
	return tracks, nil
}

// ExtractBatches reads HashedFile values from in, groups them into batches of
// batchSize (~500), and calls extractor.Extract on each batch. Track values
// are sent to the returned channel, which is closed once all batches have been
// processed. Batches that return an error are silently dropped so that one
// failed exiftool invocation does not stop the pipeline.
func ExtractBatches(extractor MetadataExtractor, in <-chan HashedFile) <-chan Track {
	out := make(chan Track, runtime.NumCPU())
	go func() {
		defer close(out)
		batch := make([]HashedFile, 0, batchSize)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			tracks, err := extractor.Extract(batch)
			if err == nil {
				for _, t := range tracks {
					out <- t
				}
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
