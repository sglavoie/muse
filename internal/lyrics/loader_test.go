package lyrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Song.flac"), "")
	writeFile(t, filepath.Join(dir, "Song.lrc"), "[00:01.00]exact")
	writeFile(t, filepath.Join(dir, "Other.mp3"), "")
	writeFile(t, filepath.Join(dir, "other.LRC"), "[00:01.00]folded")
	writeFile(t, filepath.Join(dir, "Lonely.m4a"), "")
	writeFile(t, filepath.Join(dir, "Lonely.flac.lrc"), "[00:01.00]wrong name")
	if err := os.Mkdir(filepath.Join(dir, "Dir.lrc"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "Dir.ogg"), "")

	tests := map[string]string{
		"Song.flac":  "Song.lrc",
		"Other.mp3":  "other.LRC",
		"Lonely.m4a": "",
		"Dir.ogg":    "",
	}
	for audio, want := range tests {
		got := SidecarPath(filepath.Join(dir, audio))
		if want == "" {
			if got != "" {
				t.Errorf("SidecarPath(%s) = %q, want none", audio, got)
			}
			continue
		}
		// A case-insensitive file system finds the file under either name,
		// so compare the files rather than the spelling.
		gotInfo, err1 := os.Stat(got)
		wantInfo, err2 := os.Stat(filepath.Join(dir, want))
		if err1 != nil || err2 != nil || !os.SameFile(gotInfo, wantInfo) {
			t.Errorf("SidecarPath(%s) = %q, want %s", audio, got, want)
		}
	}
	if got := SidecarPath(filepath.Join(dir, "missing", "x.flac")); got != "" {
		t.Errorf("missing folder: %q", got)
	}
}

func TestFileLoaderPrefersSidecarThenEmbedded(t *testing.T) {
	dir := t.TempDir()
	withLRC := filepath.Join(dir, "a.flac")
	writeFile(t, filepath.Join(dir, "a.lrc"), "[00:01.00]from the file")
	plain := filepath.Join(dir, "b.flac")
	calls := 0
	loader := FileLoader{Embedded: func(_ context.Context, path string) (string, error) {
		calls++
		if path == plain {
			return "Embedded words\nsecond line", nil
		}
		return "", nil
	}}

	got, err := loader.Load(context.Background(), withLRC)
	if err != nil || got.Source != "sidecar" || got.Lines[0].Text != "from the file" || calls != 0 {
		t.Fatalf("sidecar: %+v %v (embedded calls %d)", got, err, calls)
	}
	got, err = loader.Load(context.Background(), plain)
	if err != nil || got.Source != "embedded" || got.Synced || len(got.Lines) != 2 {
		t.Fatalf("embedded: %+v %v", got, err)
	}
	if _, err := loader.Load(context.Background(), filepath.Join(dir, "c.flac")); !errors.Is(err, ErrNoLyrics) {
		t.Fatalf("no lyrics: %v", err)
	}
	if _, err := (FileLoader{}).Load(context.Background(), plain); !errors.Is(err, ErrNoLyrics) {
		t.Fatalf("without an embedded reader: %v", err)
	}
}

func TestFileLoaderSkipsEmptySidecar(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.lrc"), "[ar:Nobody]\n")
	loader := FileLoader{Embedded: func(context.Context, string) (string, error) {
		return "[00:02.00]tagged", nil
	}}
	got, err := loader.Load(context.Background(), filepath.Join(dir, "a.flac"))
	if err != nil || got.Source != "embedded" {
		t.Fatalf("an empty .lrc should fall back to the tag: %+v %v", got, err)
	}
}
