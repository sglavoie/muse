package library

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// requireFFProbe skips the test when the ffprobe binary is not on PATH,
// mirroring the LookPath-and-skip guard used by the exiftool tests.
func requireFFProbe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found on PATH: " + err.Error())
	}
}

// requireFFmpeg skips the test when the ffmpeg binary is not on PATH. ffmpeg is
// what synthesises the tagged mp3/flac/opus/m4a fixtures: the repository ships
// no audio testdata, and a hand-built minimal WAV carries no tags at all, so it
// could not exercise artist/title/album/genre agreement.
func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found on PATH: " + err.Error())
	}
}

// requireExifTool skips the test when the exiftool binary is not on PATH.
func requireExifTool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not found on PATH: " + err.Error())
	}
}

// Tag values written into every generated fixture. Genre uses the "; " form the
// exiftool path produces when it joins an array-valued tag, so both backends
// must emit a byte-identical string.
const (
	fixtureArtist      = "Differential Artist"
	fixtureTitle       = "Differential Title"
	fixtureAlbum       = "Differential Album"
	fixtureAlbumArtist = "Various Artists"
	fixtureGenre       = "Rock; Pop"
	fixtureTrack       = 3
	fixtureDisc        = 1
	fixtureDuration    = 2.0
)

// ffFixture describes one generated audio file and how strictly its duration
// may be compared against the exiftool reading of the same file.
type ffFixture struct {
	ext string
	// exifDurationTolerance is the allowed absolute difference in seconds
	// between the exiftool and ffprobe durations. exiftool derives mp3 duration
	// from the bitrate rather than the container, so its answer for a VBR mp3
	// runs well over a second long on a 2-second file; the container formats
	// agree to the millisecond.
	exifDurationTolerance float64
	// exifReportsDuration is false for containers where exiftool returns no
	// Duration tag at all (opus), leaving nothing to compare.
	exifReportsDuration bool
}

var ffFixtures = []ffFixture{
	{ext: "mp3", exifDurationTolerance: 1.5, exifReportsDuration: true},
	{ext: "flac", exifDurationTolerance: 0.05, exifReportsDuration: true},
	{ext: "opus", exifDurationTolerance: 0, exifReportsDuration: false},
	{ext: "m4a", exifDurationTolerance: 0.05, exifReportsDuration: true},
}

// writeTaggedFixture synthesises a short tagged audio file at path using ffmpeg
// and returns the HashedFile describing it.
func writeTaggedFixture(t *testing.T, path string) HashedFile {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-f", "lavfi",
		"-i", fmt.Sprintf("sine=frequency=440:duration=%g", fixtureDuration),
		"-metadata", "artist="+fixtureArtist,
		"-metadata", "title="+fixtureTitle,
		"-metadata", "album="+fixtureAlbum,
		"-metadata", "album_artist="+fixtureAlbumArtist,
		"-metadata", "genre="+fixtureGenre,
		"-metadata", fmt.Sprintf("track=%d/12", fixtureTrack),
		"-metadata", fmt.Sprintf("disc=%d/2", fixtureDisc),
		"-y", path,
	).CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg could not encode %s (missing encoder?): %v: %s", filepath.Base(path), err, out)
	}
	return statFixture(t, path)
}

// statFixture builds a HashedFile for an already-written fixture, filling
// ModTime and SizeBytes the way HashWorkers would.
func statFixture(t *testing.T, path string) HashedFile {
	t.Helper()
	hash, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile(%s): %v", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat(%s): %v", path, err)
	}
	return HashedFile{
		Path:      path,
		Hash:      hash,
		ModTime:   info.ModTime().Unix(),
		SizeBytes: info.Size(),
	}
}

// TestFFProbeExtractor_Differential encodes one tagged fixture per container
// and asserts that FFProbeExtractor and ExifToolExtractor read back the same
// metadata for each of them.
func TestFFProbeExtractor_Differential(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)
	requireExifTool(t)

	dir := t.TempDir()
	files := make([]HashedFile, 0, len(ffFixtures))
	for _, f := range ffFixtures {
		files = append(files, writeTaggedFixture(t, filepath.Join(dir, "fixture."+f.ext)))
	}

	ffTracks, err := FFProbeExtractor{}.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("FFProbeExtractor.Extract: %v", err)
	}
	if len(ffTracks) != len(files) {
		t.Fatalf("FFProbeExtractor returned %d tracks, want %d", len(ffTracks), len(files))
	}

	exifTracks, err := ExifToolExtractor{}.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("ExifToolExtractor.Extract: %v", err)
	}
	exifByPath := make(map[string]Track, len(exifTracks))
	for _, tr := range exifTracks {
		exifByPath[tr.Path] = tr
	}

	for i, fx := range ffFixtures {
		t.Run(fx.ext, func(t *testing.T) {
			got := ffTracks[i]
			if got.Path != files[i].Path {
				t.Fatalf("result %d out of order: got path %q, want %q", i, got.Path, files[i].Path)
			}
			if got.Hash != files[i].Hash {
				t.Errorf("Hash = %q, want %q", got.Hash, files[i].Hash)
			}
			if got.ModTime != files[i].ModTime || got.SizeBytes != files[i].SizeBytes {
				t.Errorf("stat fields = (%d, %d), want (%d, %d)",
					got.ModTime, got.SizeBytes, files[i].ModTime, files[i].SizeBytes)
			}

			// ffprobe must read back exactly what ffmpeg wrote.
			if got.Artist != fixtureArtist {
				t.Errorf("Artist = %q, want %q", got.Artist, fixtureArtist)
			}
			if got.Title != fixtureTitle {
				t.Errorf("Title = %q, want %q", got.Title, fixtureTitle)
			}
			if got.Album != fixtureAlbum {
				t.Errorf("Album = %q, want %q", got.Album, fixtureAlbum)
			}
			if got.AlbumArtist != fixtureAlbumArtist {
				t.Errorf("AlbumArtist = %q, want %q", got.AlbumArtist, fixtureAlbumArtist)
			}
			if got.Genre != fixtureGenre {
				t.Errorf("Genre = %q, want %q", got.Genre, fixtureGenre)
			}
			if got.TrackNumber != fixtureTrack {
				t.Errorf("TrackNumber = %d, want %d", got.TrackNumber, fixtureTrack)
			}
			if got.DiscNumber != fixtureDisc {
				t.Errorf("DiscNumber = %d, want %d", got.DiscNumber, fixtureDisc)
			}
			if math.Abs(got.Duration-fixtureDuration) > 0.05 {
				t.Errorf("Duration = %v, want %v (±0.05)", got.Duration, fixtureDuration)
			}

			// And it must agree with what exiftool reads from the same file.
			want, ok := exifByPath[files[i].Path]
			if !ok {
				t.Fatalf("exiftool returned no record for %s", files[i].Path)
			}
			if got.Artist != want.Artist {
				t.Errorf("Artist: ffprobe %q, exiftool %q", got.Artist, want.Artist)
			}
			if got.Title != want.Title {
				t.Errorf("Title: ffprobe %q, exiftool %q", got.Title, want.Title)
			}
			if got.Album != want.Album {
				t.Errorf("Album: ffprobe %q, exiftool %q", got.Album, want.Album)
			}
			if got.AlbumArtist != want.AlbumArtist {
				t.Errorf("AlbumArtist: ffprobe %q, exiftool %q", got.AlbumArtist, want.AlbumArtist)
			}
			if got.Genre != want.Genre {
				t.Errorf("Genre: ffprobe %q, exiftool %q", got.Genre, want.Genre)
			}
			// Each container spells the ordinal tags differently (ID3 Track and
			// PartOfSet, Vorbis TrackNumber and Discnumber, QuickTime "3 of 12"),
			// so both extractors must agree on every fixture.
			if got.TrackNumber != want.TrackNumber {
				t.Errorf("TrackNumber: ffprobe %d, exiftool %d", got.TrackNumber, want.TrackNumber)
			}
			if got.DiscNumber != want.DiscNumber {
				t.Errorf("DiscNumber: ffprobe %d, exiftool %d", got.DiscNumber, want.DiscNumber)
			}
			if fx.exifReportsDuration {
				if want.Duration == 0 {
					t.Errorf("exiftool reported no duration for .%s; update ffFixtures", fx.ext)
				} else if math.Abs(got.Duration-want.Duration) > fx.exifDurationTolerance {
					t.Errorf("Duration: ffprobe %v, exiftool %v (tolerance %v)",
						got.Duration, want.Duration, fx.exifDurationTolerance)
				}
			}
		})
	}
}

// TestFFProbeExtractor_NoTags verifies that a probeable file carrying no
// artist/title/album/genre yields empty metadata fields rather than an error or
// a filename fallback — the file was read successfully, it simply has no tags.
// extractWithBinaryRetry is what fills in the display defaults downstream.
func TestFFProbeExtractor_NoTags(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "untagged.flac")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error",
		"-f", "lavfi",
		"-i", fmt.Sprintf("sine=frequency=440:duration=%g", fixtureDuration),
		"-map_metadata", "-1",
		"-y", path,
	).CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg could not encode untagged fixture: %v: %s", err, out)
	}

	files := []HashedFile{statFixture(t, path)}
	tracks, err := FFProbeExtractor{}.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("got %d tracks, want 1", len(tracks))
	}
	got := tracks[0]
	if got.Artist != "" || got.Title != "" || got.Album != "" || got.Genre != "" {
		t.Errorf("expected empty tag fields for an untagged file, got %+v", got)
	}
	if got.TrackNumber != 0 || got.DiscNumber != 0 {
		t.Errorf("expected zero track/disc numbers, got %d/%d", got.TrackNumber, got.DiscNumber)
	}
	if math.Abs(got.Duration-fixtureDuration) > 0.05 {
		t.Errorf("Duration = %v, want %v (±0.05)", got.Duration, fixtureDuration)
	}
	if got.Hash != files[0].Hash {
		t.Errorf("Hash = %q, want %q", got.Hash, files[0].Hash)
	}
}

// TestFFProbeExtractor_UnprobeableFilesFallBack covers the two ways a file can
// fail to probe — it does not exist, or it is not audio despite its extension.
// Neither may drop the track: both must yield the same filename-derived
// fallback the exiftool path produces.
func TestFFProbeExtractor_UnprobeableFilesFallBack(t *testing.T) {
	requireFFProbe(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "does_not_exist.mp3")
	bogus := filepath.Join(dir, "not_really_audio.mp3")
	writeFile(t, bogus, []byte("this is plain text, not an mp3 frame\n"))

	files := []HashedFile{
		{Path: missing, Hash: "missing-hash", ModTime: 111, SizeBytes: 222},
		{Path: bogus, Hash: "bogus-hash", ModTime: 333, SizeBytes: 444},
	}

	tracks, err := FFProbeExtractor{}.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("Extract must not fail on unprobeable files: %v", err)
	}
	if len(tracks) != len(files) {
		t.Fatalf("got %d tracks, want %d — unprobeable files must not be dropped", len(tracks), len(files))
	}
	for i, got := range tracks {
		want := Track{
			Path:      files[i].Path,
			Hash:      files[i].Hash,
			Artist:    "Unknown Artist",
			Title:     filenameStem(files[i].Path),
			ModTime:   files[i].ModTime,
			SizeBytes: files[i].SizeBytes,
		}
		if got != want {
			t.Errorf("fallback track %d:\n got %+v\nwant %+v", i, got, want)
		}
	}
}

// TestFFProbeExtractor_PreservesInputOrder mixes probeable and unprobeable
// files so the index-based correlation is exercised: every result must land at
// its input position carrying that input's hash and stat data.
func TestFFProbeExtractor_PreservesInputOrder(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)

	dir := t.TempDir()
	var files []HashedFile
	for i := range 6 {
		if i%2 == 0 {
			files = append(files, writeTaggedFixture(t, filepath.Join(dir, fmt.Sprintf("good%02d.flac", i))))
			continue
		}
		files = append(files, HashedFile{
			Path:      filepath.Join(dir, fmt.Sprintf("bad%02d.mp3", i)),
			Hash:      fmt.Sprintf("bad-hash-%02d", i),
			ModTime:   int64(i),
			SizeBytes: int64(i * 10),
		})
	}

	tracks, err := FFProbeExtractor{}.Extract(context.Background(), files)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(tracks) != len(files) {
		t.Fatalf("got %d tracks, want %d", len(tracks), len(files))
	}
	for i, got := range tracks {
		if got.Path != files[i].Path || got.Hash != files[i].Hash {
			t.Errorf("result %d: got (%q, %q), want (%q, %q)",
				i, got.Path, got.Hash, files[i].Path, files[i].Hash)
		}
		if got.ModTime != files[i].ModTime || got.SizeBytes != files[i].SizeBytes {
			t.Errorf("result %d: stat fields (%d, %d), want (%d, %d)",
				i, got.ModTime, got.SizeBytes, files[i].ModTime, files[i].SizeBytes)
		}
	}
}

// TestFFProbeExtractor_EmptyInput verifies the zero-file short circuit.
func TestFFProbeExtractor_EmptyInput(t *testing.T) {
	tracks, err := FFProbeExtractor{}.Extract(context.Background(), nil)
	if err != nil {
		t.Fatalf("Extract(nil): %v", err)
	}
	if tracks != nil {
		t.Errorf("Extract(nil) = %v, want nil", tracks)
	}
}

// TestFFProbeExtractor_CancelledBeforeStart verifies that an already-cancelled
// context short-circuits without producing filename fallbacks for files that
// were never probed.
func TestFFProbeExtractor_CancelledBeforeStart(t *testing.T) {
	requireFFProbe(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	files := []HashedFile{{Path: "/nonexistent/a.mp3", Hash: "a"}, {Path: "/nonexistent/b.mp3", Hash: "b"}}
	tracks, err := FFProbeExtractor{}.Extract(ctx, files)
	if err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	if !isContextCancelled(err) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if tracks != nil {
		t.Errorf("expected no tracks on cancellation, got %d", len(tracks))
	}
}

// TestFFProbeExtractor_CancelMidBatch cancels while ffprobe processes are
// actually in flight. Each fixture is a FIFO with no writer, so ffprobe blocks
// in open(2) and can only be unblocked by the context: this proves both that
// the running probes honour cancellation and that the worker pool drains
// instead of leaking. More FIFOs than workers are queued so the feeder's own
// cancellation path is exercised too.
func TestFFProbeExtractor_CancelMidBatch(t *testing.T) {
	requireFFProbe(t)
	defer goleak.VerifyNone(t)

	dir := t.TempDir()
	n := runtime.NumCPU() * 2
	files := make([]HashedFile, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("blocking%02d.wav", i))
		if err := syscall.Mkfifo(p, 0o600); err != nil {
			t.Skipf("cannot create FIFO: %v", err)
		}
		files[i] = HashedFile{Path: p, Hash: fmt.Sprintf("h%02d", i)}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Every worker is parked in ffprobe's blocking open by now, and would
		// stay there until ffprobeExtractTimeout without this cancel.
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	start := time.Now()
	tracks, err := FFProbeExtractor{}.Extract(ctx, files)
	elapsed := time.Since(start)
	<-done

	if err == nil {
		t.Fatal("expected an error after mid-batch cancellation")
	}
	if !isContextCancelled(err) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if tracks != nil {
		t.Errorf("expected no tracks on cancellation, got %d", len(tracks))
	}
	if elapsed >= ffprobeExtractTimeout {
		t.Errorf("Extract took %v — cancellation did not interrupt the running probes", elapsed)
	}
}

// isContextCancelled reports whether err is (or wraps) context.Canceled.
func isContextCancelled(err error) bool {
	return errors.Is(err, context.Canceled)
}
