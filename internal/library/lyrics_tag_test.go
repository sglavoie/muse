package library

import (
	"encoding/json"
	"testing"
)

func TestLyricsFromTags(t *testing.T) {
	raw := func(m map[string]string) map[string]json.RawMessage {
		out := map[string]json.RawMessage{}
		for k, v := range m {
			b, _ := json.Marshal(v)
			out[k] = b
		}
		return out
	}
	tests := []struct {
		name string
		tags map[string]string
		want string
	}{
		{"exiftool Lyrics", map[string]string{"SourceFile": "a.mp3", "Lyrics": "[00:01.00]hi\n"}, "[00:01.00]hi"},
		{"vorbis upper case", map[string]string{"LYRICS": "words"}, "words"},
		{"exiftool Uslt frame", map[string]string{"Uslt": "from ffmpeg"}, "from ffmpeg"},
		{"unsynced only", map[string]string{"UNSYNCEDLYRICS": "plain"}, "plain"},
		{"plain Lyrics wins", map[string]string{"lyrics-eng": "lang", "Lyrics": "main", "UnsyncedLyrics": "other"}, "main"},
		{"blank skipped", map[string]string{"Lyrics": "  ", "lyrics-eng": "lang"}, "lang"},
		{"none", map[string]string{"Title": "Song", "Comment": "lyrics later"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lyricsFromTags(raw(tt.tags)); got != tt.want {
				t.Fatalf("lyricsFromTags = %q, want %q", got, tt.want)
			}
		})
	}
}
