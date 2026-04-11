package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
)

// fakeExtractor is a MetadataExtractor that returns one Track per HashedFile,
// echoing the path and hash with no real metadata. It records each batch it
// receives so tests can assert batching behaviour.
type fakeExtractor struct {
	batches [][]HashedFile
}

func (f *fakeExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	cp := make([]HashedFile, len(files))
	copy(cp, files)
	f.batches = append(f.batches, cp)

	tracks := make([]Track, len(files))
	for i, hf := range files {
		tracks[i] = Track{
			Path:  hf.Path,
			Hash:  hf.Hash,
			Title: filepath.Base(hf.Path),
		}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*fakeExtractor)(nil)

// poisonExtractor is a MetadataExtractor that returns an error whenever the
// batch contains the file at poisonIdx. For all other batches it echoes each
// file as a Track. It counts every Extract call so tests can assert O(log n)
// behaviour.
type poisonExtractor struct {
	poisonIdx int
	calls     int
}

func (p *poisonExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	p.calls++
	for _, f := range files {
		if f.Path == fmt.Sprintf("/music/track%02d.flac", p.poisonIdx) {
			return nil, errors.New("invalid JSON: corrupt file in batch")
		}
	}
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*poisonExtractor)(nil)

// capturingHandler is a slog.Handler that records all log records for inspection.
// It is safe for concurrent use from multiple goroutines.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

// alwaysFailExtractor always returns an error regardless of input.
type alwaysFailExtractor struct{}

func (a *alwaysFailExtractor) Extract(_ context.Context, _ []HashedFile) ([]Track, error) {
	return nil, errors.New("extraction failed")
}

var _ MetadataExtractor = (*alwaysFailExtractor)(nil)

// chainExtractor simulates the full fallback chain: any batch containing the
// corrupt path fails, and even the per-file invocation for that path fails,
// forcing extractWithBinaryRetry to produce filename-derived fallback metadata.
type chainExtractor struct {
	corruptPath  string
	perFileCalls int
}

func (c *chainExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	for _, f := range files {
		if f.Path == c.corruptPath {
			if len(files) == 1 {
				c.perFileCalls++
			}
			return nil, errors.New("invalid JSON: corrupt file in batch")
		}
	}
	if len(files) == 1 {
		c.perFileCalls++
	}
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*chainExtractor)(nil)

// warnExtractor returns valid tracks and an error simultaneously, simulating
// exiftool's exit-code-1-with-valid-output behaviour.
type warnExtractor struct {
	calls int
}

func (w *warnExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	w.calls++
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash, Title: "ok"}
	}
	return tracks, errors.New("exiftool exited with status 1")
}

var _ MetadataExtractor = (*warnExtractor)(nil)

// emptyFieldExtractor returns tracks with empty Title and Artist fields,
// simulating exiftool output for files with no embedded metadata tags.
type emptyFieldExtractor struct{}

func (e *emptyFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*emptyFieldExtractor)(nil)

// partialFieldExtractor returns tracks with a preset Artist but empty Title.
type partialFieldExtractor struct {
	artist string
}

func (p *partialFieldExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash, Artist: p.artist}
	}
	return tracks, nil
}

var _ MetadataExtractor = (*partialFieldExtractor)(nil)
