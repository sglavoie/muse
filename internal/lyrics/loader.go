package lyrics

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoLyrics reports that a song has no lyrics to show.
var ErrNoLyrics = errors.New("lyrics: none found")

// Loader finds the lyrics of the audio file at path.
type Loader interface {
	Load(ctx context.Context, path string) (*Lyrics, error)
}

// maxFileBytes bounds what is read from an .lrc file; real ones are a few
// kilobytes.
const maxFileBytes = 1 << 20

// FileLoader reads an .lrc file beside the audio file and falls back to the
// lyrics embedded in its tags.
type FileLoader struct {
	// Embedded returns the lyrics tag of the file at path, or "" when it has
	// none; nil skips embedded lyrics.
	Embedded func(ctx context.Context, path string) (string, error)
}

// Load returns the song's lyrics, or ErrNoLyrics.
func (l FileLoader) Load(ctx context.Context, path string) (*Lyrics, error) {
	if sidecar := SidecarPath(path); sidecar != "" {
		if lyr, err := readFile(sidecar); err == nil {
			lyr.Source = "sidecar"
			return lyr, nil
		}
	}
	if l.Embedded == nil {
		return nil, ErrNoLyrics
	}
	text, err := l.Embedded(ctx, path)
	if err != nil {
		return nil, err
	}
	lyr := ParseString(text)
	if lyr == nil {
		return nil, ErrNoLyrics
	}
	lyr.Source = "embedded"
	return lyr, nil
}

func readFile(path string) (*Lyrics, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes))
	if err != nil {
		return nil, err
	}
	lyr := Parse(data)
	if lyr == nil {
		return nil, ErrNoLyrics
	}
	return lyr, nil
}

// SidecarPath returns the .lrc file beside the audio file with the same
// name ("Song.flac" → "Song.lrc"), or "" when there is none. An exact match
// wins; otherwise the name and extension may differ in case, as they often
// do in folders copied between file systems.
func SidecarPath(audio string) string {
	dir := filepath.Dir(audio)
	stem := strings.TrimSuffix(filepath.Base(audio), filepath.Ext(audio))
	want := stem + ".lrc"
	if info, err := os.Stat(filepath.Join(dir, want)); err == nil && info.Mode().IsRegular() {
		return filepath.Join(dir, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), want) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
