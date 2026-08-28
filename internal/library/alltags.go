package library

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ExtractAllTags returns the full metadata of a single file as a map keyed by
// the display names the track-info overlay renders (see tagGroups in
// internal/tui/trackinfo.go).
//
// The backend is chosen by SelectExtractor, so the overlay always reads a file
// through the same tool the library scan used: exiftool when it is usable,
// ffprobe otherwise. Dispatching here rather than at the call site keeps the
// (ctx, path) signature the TUI depends on and leaves MetadataExtractor — which
// this function is deliberately not part of, since it is per file and returns
// raw display tags rather than a Track — untouched.
//
// Neither backend produces every key: an absent key is simply omitted, and the
// overlay drops the corresponding row (a tagSpec whose keys are all missing
// renders nothing at all).
func ExtractAllTags(ctx context.Context, path string) (map[string]any, error) {
	// The extractor value itself is for batch scans; only the backend name
	// matters here, but going through SelectExtractor is what guarantees the
	// overlay and the scan can never disagree about which tool is in use.
	_, backend, err := SelectExtractor(ctx)
	if err != nil {
		return nil, err
	}
	if backend == BackendExifTool {
		return extractAllTagsExifTool(ctx, path)
	}
	return extractAllTagsFFProbe(ctx, path)
}

// ffprobeAllTagsOutput mirrors the JSON emitted by
// `ffprobe -print_format json -show_streams -show_format`.
//
// Numeric-looking fields are typed as ffprobe actually emits them: sample_rate,
// bit_rate, size, duration and bits_per_raw_sample are JSON strings, while
// channels and bits_per_sample are JSON numbers.
type ffprobeAllTagsOutput struct {
	Streams []ffprobeStream `json:"streams"`
	Format  struct {
		FormatName string                     `json:"format_name"`
		Duration   string                     `json:"duration"`
		Size       string                     `json:"size"`
		BitRate    string                     `json:"bit_rate"`
		Tags       map[string]json.RawMessage `json:"tags"`
	} `json:"format"`
}

// ffprobeStream is one entry of ffprobe's `streams` array. Only the audio
// stream's fields are used; the others exist because an audio file routinely
// carries an embedded cover image as a second (video) stream.
type ffprobeStream struct {
	CodecType        string                     `json:"codec_type"`
	CodecName        string                     `json:"codec_name"`
	SampleRate       string                     `json:"sample_rate"`
	Channels         int                        `json:"channels"`
	BitsPerRawSample string                     `json:"bits_per_raw_sample"`
	BitsPerSample    int                        `json:"bits_per_sample"`
	BitRate          string                     `json:"bit_rate"`
	Tags             map[string]json.RawMessage `json:"tags"`
}

// extractAllTagsFFProbe probes a single file with ffprobe and maps the result
// onto the overlay's key names. Unlike the exiftool path it cannot fail on a
// per-tag basis: anything ffprobe does not report is left out of the map.
func extractAllTagsFFProbe(ctx context.Context, path string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, extractAllTagsTimeout)
	defer cancel()

	raw, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_streams",
		"-show_format",
		path,
	).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var out ffprobeAllTagsOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ffprobe %s: parse output: %w", path, err)
	}
	return ffprobeDisplayTags(path, out), nil
}

// ffprobeDisplayTags converts decoded ffprobe output into the overlay's key
// space. Split from the exec call so the mapping is testable without a
// subprocess.
func ffprobeDisplayTags(path string, out ffprobeAllTagsOutput) map[string]any {
	// Index of the first audio stream; -1 when the file has none (ffprobe will
	// happily describe a cover-art-only or non-audio file).
	audio := -1
	for i, s := range out.Streams {
		if s.CodecType == "audio" {
			audio = i
			break
		}
	}

	// Only the audio stream's tags are merged with the container's. Merging
	// every stream, as the scan extractor does, would let an embedded cover
	// image's `comment: "Cover (front)"` masquerade as the track's comment.
	tags := make(map[string]json.RawMessage)
	if audio >= 0 {
		for k, v := range out.Streams[audio].Tags {
			tags[strings.ToLower(k)] = v
		}
	}
	for k, v := range out.Format.Tags {
		tags[strings.ToLower(k)] = v
	}

	m := map[string]any{"SourceFile": path}
	set := func(key, val string) {
		if val != "" {
			m[key] = val
		}
	}

	set("Title", lookupTag(tags, "title"))
	set("Artist", lookupTag(tags, "artist"))
	set("Album", lookupTag(tags, "album"))
	set("AlbumArtist", lookupTag(tags, "album_artist", "albumartist", "album artist"))
	set("Genre", lookupTag(tags, "genre"))
	set("Comment", lookupTag(tags, "comment", "description"))
	set("Encoder", lookupTag(tags, "encoder", "encoded_by"))
	// The overlay reads Year first and falls back to Date; ffprobe's `date` tag
	// may be a bare year or a full ISO date, so it is published as Date and the
	// overlay renders whatever the file actually carries.
	set("Date", lookupTag(tags, "date", "year", "originalyear"))

	// Track/disc arrive as "3" or "3/12" depending on the container; the parsed
	// ordinal is what the overlay should show, matching exiftool's numeric Track.
	if n := parseTrackNumber(lookupTag(tags, "track", "tracknumber", "track_number")); n > 0 {
		m["Track"] = n
	}
	if n := parseTrackNumber(lookupTag(tags, "disc", "discnumber", "disc_number", "part_of_set")); n > 0 {
		m["DiscNumber"] = n
	}

	if audio >= 0 {
		s := out.Streams[audio]
		if hz, err := strconv.Atoi(s.SampleRate); err == nil && hz > 0 {
			m["SampleRate"] = hz
		}
		if s.Channels > 0 {
			m["AudioChannels"] = s.Channels
		}
		// bits_per_raw_sample is the meaningful depth for lossless codecs;
		// bits_per_sample is 0 for every compressed codec, so it is only a
		// fallback for raw PCM containers (wav/aiff/caf).
		if bits, err := strconv.Atoi(s.BitsPerRawSample); err == nil && bits > 0 {
			m["BitsPerSample"] = bits
		} else if s.BitsPerSample > 0 {
			m["BitsPerSample"] = s.BitsPerSample
		}
		// Prefer the stream's own bitrate; the container figure includes tag
		// and cover-art overhead. Many demuxers report only the latter.
		if br := formatBitrateTag(s.BitRate); br != "" {
			m["AudioBitrate"] = br
		} else if br := formatBitrateTag(out.Format.BitRate); br != "" {
			m["AudioBitrate"] = br
		}
	}

	if d, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil {
		if d = sanitizeDuration(d); d > 0 {
			m["Duration"] = formatDurationTag(d)
		}
	}
	if size, err := strconv.ParseInt(out.Format.Size, 10, 64); err == nil && size >= 0 {
		m["FileSize"] = formatFileSizeTag(size)
	}

	codec := ""
	if audio >= 0 {
		codec = out.Streams[audio].CodecName
	}
	fileType, mimeType := ffprobeFileKind(out.Format.FormatName, codec, path)
	set("FileType", fileType)
	set("MIMEType", mimeType)

	return m
}

// ffprobeContainer maps an ffprobe format_name onto the container's display
// name and MIME type. format_name is the demuxer identifier, so it is shared by
// whole container families ("mov,mp4,m4a,3gp,3g2,mj2" covers every MP4 flavour)
// and never format-specific in the way exiftool's FileType is.
var ffprobeContainer = map[string]struct{ fileType, mimeType string }{
	"mp3":                     {"MP3", "audio/mpeg"},
	"flac":                    {"FLAC", "audio/flac"},
	"ogg":                     {"OGG", "audio/ogg"},
	"mov,mp4,m4a,3gp,3g2,mj2": {"M4A", "audio/mp4"},
	"wav":                     {"WAV", "audio/x-wav"},
	"w64":                     {"W64", "audio/x-wav"},
	"asf":                     {"WMA", "audio/x-ms-wma"},
	"aac":                     {"AAC", "audio/aac"},
	"ac3":                     {"AC3", "audio/ac3"},
	"dts":                     {"DTS", "audio/vnd.dts"},
	"aiff":                    {"AIFF", "audio/x-aiff"},
	"ape":                     {"APE", "audio/x-ape"},
	"au":                      {"AU", "audio/basic"},
	"caf":                     {"CAF", "audio/x-caf"},
	"matroska,webm":           {"MKA", "audio/x-matroska"},
	"wv":                      {"WV", "audio/x-wavpack"},
}

// ffprobeFileKind derives the FileType and MIMEType rows from ffprobe's
// container and codec names. An unrecognised container degrades to the
// uppercased file extension with no MIME type rather than guessing, and the
// overlay then simply omits the MIME row.
func ffprobeFileKind(formatName, codecName, path string) (string, string) {
	kind, ok := ffprobeContainer[formatName]
	if !ok {
		return strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")), ""
	}
	// Ogg is the one container whose display name depends on the codec: both
	// .ogg (vorbis) and .opus demux as "ogg", and exiftool reports them apart.
	if formatName == "ogg" && codecName == "opus" {
		return "OPUS", kind.mimeType
	}
	return kind.fileType, kind.mimeType
}

// formatBitrateTag renders an ffprobe bits-per-second string the way exiftool
// renders AudioBitrate ("320 kbps"). It returns "" for absent or unusable input.
func formatBitrateTag(bps string) string {
	n, err := strconv.ParseFloat(bps, 64)
	if err != nil || n <= 0 || math.IsInf(n, 0) {
		return ""
	}
	return fmt.Sprintf("%.0f kbps", n/1000)
}

// formatDurationTag renders seconds the way exiftool's Duration tag reads:
// "3.00 s" below a minute, "H:MM:SS" above it.
func formatDurationTag(sec float64) string {
	if sec < 60 {
		return fmt.Sprintf("%.2f s", sec)
	}
	total := int(sec)
	return fmt.Sprintf("%d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}

// formatFileSizeTag renders a byte count in exiftool's FileSize style: decimal
// (not binary) units, one decimal place below 10 of a unit and none above.
func formatFileSizeTag(b int64) string {
	if b < 1000 {
		return fmt.Sprintf("%d bytes", b)
	}
	v := float64(b)
	units := []string{"kB", "MB", "GB", "TB", "PB"}
	unit := units[len(units)-1]
	for _, u := range units {
		v /= 1000
		if v < 999.95 {
			unit = u
			break
		}
	}
	if v < 10 {
		return fmt.Sprintf("%.1f %s", v, unit)
	}
	return fmt.Sprintf("%.0f %s", v, unit)
}
