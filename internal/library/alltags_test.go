package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allTagsFixtures are the containers the ffprobe track-info mapping is checked
// against. They mirror ffFixtures (the scan-path differential set) so both
// halves of the ffprobe backend are exercised on the same encoders, plus the
// FileType/MIMEType pair each container is expected to report.
var allTagsFixtures = []struct {
	ext      string
	fileType string
	mimeType string
	// lossless is true for containers that report a real sample depth;
	// compressed codecs leave bits_per_raw_sample unset.
	lossless bool
}{
	{ext: "mp3", fileType: "MP3", mimeType: "audio/mpeg"},
	{ext: "flac", fileType: "FLAC", mimeType: "audio/flac", lossless: true},
	{ext: "opus", fileType: "OPUS", mimeType: "audio/ogg"},
	{ext: "m4a", fileType: "M4A", mimeType: "audio/mp4"},
}

// tagValue returns the first of keys present in tags, rendered the way the
// overlay renders it (fmt.Sprint), plus whether any key was present. It is the
// test-side mirror of the tagSpec key-alternates fallback in
// internal/tui/trackinfo.go.
func tagValue(tags map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := tags[k]; ok {
			return fmt.Sprint(v), true
		}
	}
	return "", false
}

// TestExtractAllTagsFFProbe_Keys asserts that the ffprobe backend emits the
// technical and tag keys the track-info overlay reads, with values of the
// expected shape, for every supported container family.
func TestExtractAllTagsFFProbe_Keys(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)

	dir := t.TempDir()
	for _, fx := range allTagsFixtures {
		t.Run(fx.ext, func(t *testing.T) {
			path := filepath.Join(dir, "alltags."+fx.ext)
			writeTaggedFixture(t, path)

			tags, err := extractAllTagsFFProbe(context.Background(), path)
			if err != nil {
				t.Fatalf("extractAllTagsFFProbe: %v", err)
			}

			if got, _ := tagValue(tags, "SourceFile"); got != path {
				t.Errorf("SourceFile = %q, want %q", got, path)
			}
			for key, want := range map[string]string{
				"Title":    fixtureTitle,
				"Artist":   fixtureArtist,
				"Album":    fixtureAlbum,
				"Genre":    fixtureGenre,
				"FileType": fx.fileType,
				"MIMEType": fx.mimeType,
			} {
				got, ok := tagValue(tags, key)
				if !ok {
					t.Errorf("%s missing", key)
					continue
				}
				if got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}

			// Track and disc are stored as parsed ordinals, not "3/12".
			if n, ok := tags["Track"].(int); !ok || n != fixtureTrack {
				t.Errorf("Track = %#v, want int %d", tags["Track"], fixtureTrack)
			}
			if n, ok := tags["DiscNumber"].(int); !ok || n != fixtureDisc {
				t.Errorf("DiscNumber = %#v, want int %d", tags["DiscNumber"], fixtureDisc)
			}

			// Technical rows: present, typed, and plausible.
			hz, ok := tags["SampleRate"].(int)
			if !ok || hz < 8000 || hz > 192000 {
				t.Errorf("SampleRate = %#v, want a plausible int rate", tags["SampleRate"])
			}
			if ch, ok := tags["AudioChannels"].(int); !ok || ch < 1 {
				t.Errorf("AudioChannels = %#v, want a positive int", tags["AudioChannels"])
			}
			if _, ok := tags["BitsPerSample"]; ok != fx.lossless {
				t.Errorf("BitsPerSample present = %v, want %v", ok, fx.lossless)
			}
			for _, key := range []string{"AudioBitrate", "Duration", "FileSize"} {
				v, ok := tags[key].(string)
				if !ok || v == "" {
					t.Errorf("%s = %#v, want a non-empty string", key, tags[key])
				}
			}
			// The rendered duration must parse back to the encoded length;
			// the opus encoder pads by a few milliseconds.
			got, _ := tagValue(tags, "Duration")
			secs, err := strconv.ParseFloat(strings.TrimSuffix(got, " s"), 64)
			if err != nil || secs < fixtureDuration-0.1 || secs > fixtureDuration+0.1 {
				t.Errorf("Duration = %q, want ~%.2f s", got, fixtureDuration)
			}
		})
	}
}

// TestExtractAllTags_DispatchesToFFProbe is the end-to-end check that the
// overlay's entry point works with exiftool off $PATH: only a real ffprobe is
// reachable, and ExtractAllTags must still return the overlay's keys rather
// than failing the way the exiftool-only implementation did.
func TestExtractAllTags_DispatchesToFFProbe(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)

	// Encode the fixture while ffmpeg is still reachable.
	path := filepath.Join(t.TempDir(), "dispatch.flac")
	writeTaggedFixture(t, path)

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatalf("LookPath(ffprobe): %v", err)
	}
	pathDir := t.TempDir()
	if err := os.Symlink(ffprobePath, filepath.Join(pathDir, "ffprobe")); err != nil {
		t.Fatalf("symlink ffprobe: %v", err)
	}
	t.Setenv("PATH", pathDir)

	if _, err := exec.LookPath("exiftool"); err == nil {
		t.Fatal("exiftool still resolvable after isolating PATH")
	}

	tags, err := ExtractAllTags(context.Background(), path)
	if err != nil {
		t.Fatalf("ExtractAllTags with exiftool off PATH: %v", err)
	}
	if got, _ := tagValue(tags, "Title"); got != fixtureTitle {
		t.Errorf("Title = %q, want %q", got, fixtureTitle)
	}
	if got, _ := tagValue(tags, "FileType"); got != "FLAC" {
		t.Errorf("FileType = %q, want %q", got, "FLAC")
	}
}

// TestExtractAllTags_NoBackend verifies that ExtractAllTags surfaces
// SelectExtractor's install instructions when neither binary is available,
// rather than reporting a bare "executable file not found".
func TestExtractAllTags_NoBackend(t *testing.T) {
	stubPATH(t, nil)

	_, err := ExtractAllTags(context.Background(), "/nonexistent/file.flac")
	if err == nil {
		t.Fatal("expected an error with no metadata backend on PATH")
	}
	if got := err.Error(); !strings.Contains(got, "no metadata backend available") {
		t.Errorf("error = %q, want SelectExtractor's no-backend message", got)
	}
}

// allTagsComparable lists the overlay keys where the two backends are expected
// to agree, with the alternate spellings the overlay itself tries.
//
// Deliberately excluded, with reasons:
//   - AudioBitrate/AvgBitrate: exiftool derives mp3 bitrate from the frame
//     header while ffprobe measures the muxed stream; on a 2s fixture they
//     differ by more than 10%.
//   - Duration: exiftool approximates mp3 duration from the bitrate and emits
//     none at all for opus (see ffFixtures).
//   - FileSize: both are human-rounded strings from different rounding rules.
//   - Encoder/EncoderSettings: exiftool files the same Lavf string under
//     different keys per container.
//   - AlbumArtist: exiftool spells it Albumartist or Band depending on the
//     container, neither of which the overlay reads, so there is nothing on
//     the exiftool side to compare against.
var allTagsComparable = []struct {
	keys    []string
	numeric bool
	// ordinal keys are compared through parseTrackNumber: exiftool passes
	// through the container's raw "3/12" for mp3 while the ffprobe mapping
	// stores the parsed position.
	ordinal bool
}{
	{keys: []string{"Title"}},
	{keys: []string{"Artist"}},
	{keys: []string{"Album"}},
	{keys: []string{"Genre"}},
	{keys: []string{"Track"}, ordinal: true},
	{keys: []string{"DiscNumber"}, ordinal: true},
	{keys: []string{"SampleRate"}, numeric: true},
	{keys: []string{"AudioChannels", "NumChannels"}, numeric: true},
	{keys: []string{"BitsPerSample"}, numeric: true},
	{keys: []string{"FileType"}},
	{keys: []string{"MIMEType"}},
}

// TestExtractAllTags_Differential asserts that the exiftool and ffprobe
// track-info backends read the same values from the same file for every key
// both of them report. Keys only one backend reports are skipped: coverage is
// uneven in both directions (exiftool omits channels for mp3 and flac; ffprobe
// omits nothing but has no equivalent of exiftool's derived tags).
func TestExtractAllTags_Differential(t *testing.T) {
	requireFFProbe(t)
	requireFFmpeg(t)
	requireExifTool(t)

	dir := t.TempDir()
	for _, fx := range allTagsFixtures {
		t.Run(fx.ext, func(t *testing.T) {
			path := filepath.Join(dir, "diff."+fx.ext)
			writeTaggedFixture(t, path)

			ffTags, err := extractAllTagsFFProbe(context.Background(), path)
			if err != nil {
				t.Fatalf("extractAllTagsFFProbe: %v", err)
			}
			exifTags, err := extractAllTagsExifTool(context.Background(), path)
			if err != nil {
				t.Fatalf("extractAllTagsExifTool: %v", err)
			}

			compared := 0
			for _, spec := range allTagsComparable {
				want, okExif := tagValue(exifTags, spec.keys...)
				got, okFF := tagValue(ffTags, spec.keys...)
				if !okExif || !okFF {
					continue
				}
				compared++
				if spec.ordinal {
					if parseTrackNumber(want) != parseTrackNumber(got) {
						t.Errorf("%v: exiftool=%q ffprobe=%q", spec.keys, want, got)
					}
					continue
				}
				if spec.numeric {
					wantN, err1 := strconv.ParseFloat(want, 64)
					gotN, err2 := strconv.ParseFloat(got, 64)
					if err1 != nil || err2 != nil {
						t.Errorf("%v: unparseable numbers exiftool=%q ffprobe=%q", spec.keys, want, got)
						continue
					}
					if wantN != gotN {
						t.Errorf("%v: exiftool=%v ffprobe=%v", spec.keys, wantN, gotN)
					}
					continue
				}
				if got != want {
					t.Errorf("%v: exiftool=%q ffprobe=%q", spec.keys, want, got)
				}
			}
			// Guard against the comparison silently degrading to a no-op if a
			// future exiftool or ffmpeg stops reporting these keys.
			if compared < 5 {
				t.Errorf("only %d keys were comparable, want at least 5", compared)
			}
		})
	}
}

// rawTags builds the json.RawMessage tag map ffprobe would decode into.
func rawTags(kv map[string]string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(kv))
	for k, v := range kv {
		b, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		out[k] = b
	}
	return out
}

// TestFFProbeDisplayTags_CoverArtCommentIgnored pins the reason the track-info
// mapping merges only the audio stream's tags: a FLAC with embedded cover art
// carries a video stream tagged `comment: "Cover (front)"`, which must not be
// shown as the track's comment.
func TestFFProbeDisplayTags_CoverArtCommentIgnored(t *testing.T) {
	var out ffprobeAllTagsOutput
	out.Format.FormatName = "flac"
	out.Format.Tags = rawTags(map[string]string{"TITLE": "Song"})
	out.Streams = []ffprobeStream{
		{CodecType: "audio", CodecName: "flac", SampleRate: "44100", Channels: 2, BitsPerRawSample: "16"},
		{CodecType: "video", CodecName: "mjpeg", Tags: rawTags(map[string]string{"comment": "Cover (front)"})},
	}

	tags := ffprobeDisplayTags("/music/x.flac", out)
	if v, ok := tags["Comment"]; ok {
		t.Errorf("Comment = %v, want the cover-art stream's comment to be ignored", v)
	}
	if tags["SampleRate"] != 44100 || tags["BitsPerSample"] != 16 {
		t.Errorf("audio stream fields not read: %#v", tags)
	}
}

func TestFFProbeFileKind(t *testing.T) {
	tests := []struct {
		format, codec, path string
		wantType, wantMIME  string
	}{
		{"ogg", "vorbis", "/a/x.ogg", "OGG", "audio/ogg"},
		{"ogg", "opus", "/a/x.opus", "OPUS", "audio/ogg"},
		{"mov,mp4,m4a,3gp,3g2,mj2", "aac", "/a/x.m4b", "M4A", "audio/mp4"},
		{"mp3", "mp2", "/a/x.mp2", "MP3", "audio/mpeg"},
		// Unknown container: fall back to the extension, no MIME row.
		{"nut", "pcm_s16le", "/a/x.nut", "NUT", ""},
		{"nut", "pcm_s16le", "/a/noext", "", ""},
	}
	for _, tc := range tests {
		gotType, gotMIME := ffprobeFileKind(tc.format, tc.codec, tc.path)
		if gotType != tc.wantType || gotMIME != tc.wantMIME {
			t.Errorf("ffprobeFileKind(%q, %q, %q) = (%q, %q), want (%q, %q)",
				tc.format, tc.codec, tc.path, gotType, gotMIME, tc.wantType, tc.wantMIME)
		}
	}
}

func TestFormatDurationTag(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0.5, "0.50 s"},
		{2, "2.00 s"},
		{59.99, "59.99 s"},
		{60, "0:01:00"},
		{201, "0:03:21"},
		{3661, "1:01:01"},
	}
	for _, tc := range tests {
		if got := formatDurationTag(tc.in); got != tc.want {
			t.Errorf("formatDurationTag(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatFileSizeTag(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 bytes"},
		{999, "999 bytes"},
		{8617, "8.6 kB"},
		{264644, "265 kB"},
		{8_200_000, "8.2 MB"},
		{25_591_745, "26 MB"},
		{3_000_000_000, "3.0 GB"},
	}
	for _, tc := range tests {
		if got := formatFileSizeTag(tc.in); got != tc.want {
			t.Errorf("formatFileSizeTag(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatBitrateTag(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"320000", "320 kbps"},
		{"799742", "800 kbps"},
		{"", ""},
		{"0", ""},
		{"N/A", ""},
	}
	for _, tc := range tests {
		if got := formatBitrateTag(tc.in); got != tc.want {
			t.Errorf("formatBitrateTag(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
