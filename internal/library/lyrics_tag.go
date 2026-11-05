package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// extractLyricsTimeout bounds one lyrics lookup; the Now Playing view shows
// the song without lyrics rather than wait on a stuck tool.
const extractLyricsTimeout = 5 * time.Second

// ExtractLyrics returns the lyrics embedded in the file at path, or "" when
// it carries none. It reads them through the same backend as the library
// scan: exiftool's lyrics tags (Lyrics for Vorbis LYRICS and MP4 ©lyr,
// Lyrics or Uslt for an ID3 USLT frame depending on who wrote it, and
// UnsyncedLyrics), or ffprobe's lyrics tags. The scan does not store lyrics,
// so this is one small probe of the playing song.
func ExtractLyrics(ctx context.Context, path string) (string, error) {
	_, backend, err := SelectExtractor(ctx)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, extractLyricsTimeout)
	defer cancel()
	if backend == BackendExifTool {
		return exifToolLyrics(ctx, path)
	}
	return ffprobeLyrics(ctx, path)
}

func exifToolLyrics(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "exiftool", "-json", "-*Lyrics*", "-Uslt", path).Output()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("exiftool lyrics %s: %w", path, err)
	}
	var records []map[string]json.RawMessage
	if err := json.Unmarshal(out, &records); err != nil || len(records) == 0 {
		return "", nil
	}
	return lyricsFromTags(records[0]), nil
}

func ffprobeLyrics(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "quiet", "-print_format", "json",
		"-show_entries", "format_tags:stream_tags", path).Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe lyrics %s: %w", path, err)
	}
	var probe struct {
		Streams []struct {
			Tags map[string]json.RawMessage `json:"tags"`
		} `json:"streams"`
		Format struct {
			Tags map[string]json.RawMessage `json:"tags"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return "", nil
	}
	if s := lyricsFromTags(probe.Format.Tags); s != "" {
		return s, nil
	}
	for _, stream := range probe.Streams {
		if s := lyricsFromTags(stream.Tags); s != "" {
			return s, nil
		}
	}
	return "", nil
}

// lyricsFromTags picks the lyrics out of a tag map. Names vary by format and
// tool ("Lyrics", "LYRICS", "lyrics-eng", "UNSYNCEDLYRICS"), so any key that
// starts with lyrics, unsyncedlyrics or uslt counts; a plain Lyrics tag wins, and
// the rest are tried in name order so the choice is stable.
func lyricsFromTags(tags map[string]json.RawMessage) string {
	var keys []string
	for k := range tags {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "lyrics") || strings.HasPrefix(lower, "unsyncedlyrics") || strings.HasPrefix(lower, "uslt") {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		li, lj := strings.EqualFold(keys[i], "lyrics"), strings.EqualFold(keys[j], "lyrics")
		if li != lj {
			return li
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		if s := strings.TrimSpace(coerceString(tags[k])); s != "" {
			return s
		}
	}
	return ""
}
