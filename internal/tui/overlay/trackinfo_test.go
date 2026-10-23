package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
)

func TestTrackInfoOverlayRender(t *testing.T) {
	overlay := TrackInfoOverlay{
		Path: "/music/artist/song.flac",
		rows: []Row{
			{Label: "Path:", Text: "/music/artist/song.flac"},
			{Blank: true},
			{Label: "Title:", Text: "Song"},
			{Label: "Artist:", Text: "Artist"},
		},
	}
	out := ansi.Strip(overlay.Render(80, 24, newTestStyles()))

	if !strings.Contains(out, "Track Info") {
		t.Error("expected 'Track Info' header")
	}
	if !strings.Contains(out, "/music/artist/song.flac") {
		t.Error("expected path in output")
	}
	if !strings.Contains(out, "Song") {
		t.Error("expected title in output")
	}
	if !strings.Contains(out, "Artist") {
		t.Error("expected artist in output")
	}
	if !strings.Contains(out, "y copy path") {
		t.Error("expected copy-path hint")
	}
	if !strings.Contains(out, "esc close") {
		t.Error("expected dismiss hint")
	}
}

func TestNewTrackInfoOverlay(t *testing.T) {
	tags := map[string]any{
		"Title":         "Test Song",
		"Artist":        "Test Artist",
		"Album":         "Test Album",
		"Duration":      "3:45",
		"SampleRate":    44100,
		"AudioBitrate":  "320 kbps",
		"FileType":      "FLAC",
		"MIMEType":      "audio/flac",
		"FileSize":      "25 MB",
		"AudioChannels": 2,
	}
	overlay := NewTrackInfoOverlay("/music/test.flac", tags, 0, nil)

	if overlay.Path != "/music/test.flac" {
		t.Errorf("expected path '/music/test.flac', got %q", overlay.Path)
	}

	// First row must be the path.
	if overlay.rows[0].Label != "Path:" || overlay.rows[0].Text != "/music/test.flac" {
		t.Errorf("expected first row to be path, got %+v", overlay.rows[0])
	}

	// Second row must be blank separator.
	if !overlay.rows[1].Blank {
		t.Error("expected second row to be blank separator")
	}

	// Verify tag ordering: Title should come before Artist, which should
	// come before Duration, which should come before FileType.
	labelOrder := []string{}
	for _, row := range overlay.rows {
		if row.Label != "" {
			labelOrder = append(labelOrder, row.Label)
		}
	}

	indexOf := func(label string) int {
		for i, l := range labelOrder {
			if l == label {
				return i
			}
		}
		return -1
	}

	if idx := indexOf("Title:"); idx < 0 {
		t.Error("Title: not found in rows")
	}
	if indexOf("Title:") >= indexOf("Artist:") {
		t.Error("Title: should come before Artist:")
	}
	if indexOf("Artist:") >= indexOf("Duration:") {
		t.Error("Artist: should come before Duration:")
	}
	if indexOf("Duration:") >= indexOf("File Type:") {
		t.Error("Duration: should come before File Type:")
	}

	// Verify values are present.
	out := ansi.Strip(overlay.Render(80, 24, newTestStyles()))
	for _, want := range []string{"Test Song", "Test Artist", "Test Album", "3:45", "FLAC"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in rendered output", want)
		}
	}
}

func TestNewTrackInfoOverlay_EmptyTags(t *testing.T) {
	overlay := NewTrackInfoOverlay("/music/empty.flac", map[string]any{}, 0, nil)

	// Should have path row, blank separator, and play count row — but no tag rows.
	nonBlankNonPath := 0
	for _, row := range overlay.rows {
		if !row.Blank && row.Label != "Path:" {
			nonBlankNonPath++
		}
	}
	// Play Count is always appended (1 row).
	if nonBlankNonPath != 1 {
		t.Errorf("expected only path row and play count for empty tags, got %d extra rows", nonBlankNonPath)
	}
	if overlay.rows[0].Label != "Path:" || overlay.rows[0].Text != "/music/empty.flac" {
		t.Errorf("expected path row, got %+v", overlay.rows[0])
	}
}

func TestNewTrackInfoOverlay_MissingTagsSkipped(t *testing.T) {
	// Only provide a few tags — others should be skipped.
	tags := map[string]any{
		"Title":  "Only Title",
		"Artist": "Only Artist",
	}
	overlay := NewTrackInfoOverlay("/music/sparse.flac", tags, 0, nil)

	labels := map[string]bool{}
	for _, row := range overlay.rows {
		if row.Label != "" {
			labels[row.Label] = true
		}
	}
	if !labels["Title:"] {
		t.Error("expected Title: row")
	}
	if !labels["Artist:"] {
		t.Error("expected Artist: row")
	}
	// Tags not in the input should not appear.
	for _, absent := range []string{"Album:", "Duration:", "Sample Rate:", "File Type:"} {
		if labels[absent] {
			t.Errorf("expected %s to be absent for missing tag", absent)
		}
	}
}

func TestNewTrackInfoOverlay_AlternateKeys(t *testing.T) {
	// Test that alternate keys (e.g. AvgBitrate for AudioBitrate) work.
	tags := map[string]any{
		"AvgBitrate":  "256 kbps",
		"NumChannels": 2,
		"Date":        "2024",
	}
	overlay := NewTrackInfoOverlay("/music/alt.flac", tags, 0, nil)

	labels := map[string]string{}
	for _, row := range overlay.rows {
		if row.Label != "" {
			labels[row.Label] = row.Text
		}
	}
	if labels["Bitrate:"] != "256 kbps" {
		t.Errorf("expected Bitrate: '256 kbps' from AvgBitrate, got %q", labels["Bitrate:"])
	}
	if labels["Channels:"] != "2" {
		t.Errorf("expected Channels: '2' from NumChannels, got %q", labels["Channels:"])
	}
	if labels["Year:"] != "2024" {
		t.Errorf("expected Year: '2024' from Date, got %q", labels["Year:"])
	}
}

func TestNewTrackInfoOverlay_BlankSeparators(t *testing.T) {
	// Provide tags from two non-adjacent groups to verify blank separators.
	tags := map[string]any{
		"Title":    "Song",
		"FileType": "MP3",
	}
	overlay := NewTrackInfoOverlay("/music/sep.flac", tags, 0, nil)

	blankCount := 0
	for _, row := range overlay.rows {
		if row.Blank {
			blankCount++
		}
	}
	// At least the separator after the path row, plus one between the groups.
	if blankCount < 2 {
		t.Errorf("expected at least 2 blank separators, got %d", blankCount)
	}
}

func TestNewTrackInfoOverlay_WithUserTags(t *testing.T) {
	userTags := []db.TagRow{
		{ID: 1, Name: "chill"},
		{ID: 2, Name: "workout"},
	}
	overlay := NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "Song"}, 5, userTags)

	var tagsRow Row
	found := false
	for _, row := range overlay.rows {
		if row.Label == "Tags:" {
			tagsRow = row
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected Tags: row in overlay")
	}
	if tagsRow.Text != "chill, workout" {
		t.Errorf("expected 'chill, workout', got %q", tagsRow.Text)
	}
}

func TestNewTrackInfoOverlay_NoUserTags(t *testing.T) {
	overlay := NewTrackInfoOverlay("/music/song.flac", map[string]any{"Title": "Song"}, 0, nil)

	for _, row := range overlay.rows {
		if row.Label == "Tags:" {
			t.Fatal("Tags: row should not appear when there are no user tags")
		}
	}
}

// TestNewTrackInfoOverlay_FFProbeKeys pins the contract between the ffprobe
// half of library.ExtractAllTags and this overlay: the key names that backend
// emits must fill every row tagGroups defines. If a key is renamed on either
// side, the corresponding row silently disappears in the app — here it fails.
func TestNewTrackInfoOverlay_FFProbeKeys(t *testing.T) {
	// Exactly the keys internal/library/alltags.go emits, with values of the
	// types it emits them as (ints for the counted fields).
	tags := map[string]any{
		"SourceFile":    "/music/probe.flac",
		"Title":         "Probe Song",
		"Artist":        "Probe Artist",
		"Album":         "Probe Album",
		"AlbumArtist":   "Probe Album Artist",
		"Genre":         "Ambient",
		"Comment":       "Probe Comment",
		"Encoder":       "Lavf62.12.102",
		"Date":          "2024",
		"Track":         3,
		"DiscNumber":    1,
		"SampleRate":    44100,
		"AudioChannels": 2,
		"BitsPerSample": 16,
		"AudioBitrate":  "800 kbps",
		"Duration":      "0:04:16",
		"FileSize":      "26 MB",
		"FileType":      "FLAC",
		"MIMEType":      "audio/flac",
	}
	overlay := NewTrackInfoOverlay("/music/probe.flac", tags, 7, nil)

	got := map[string]string{}
	for _, row := range overlay.rows {
		if row.Label != "" {
			got[row.Label] = row.Text
		}
	}
	want := map[string]string{
		"Title:":        "Probe Song",
		"Artist:":       "Probe Artist",
		"Album:":        "Probe Album",
		"Album Artist:": "Probe Album Artist",
		"Genre:":        "Ambient",
		"Track:":        "3",
		"Disc:":         "1",
		"Year:":         "2024",
		"Duration:":     "0:04:16",
		"Sample Rate:":  "44100",
		"Bitrate:":      "800 kbps",
		"Channels:":     "2",
		"Bits/Sample:":  "16",
		"File Type:":    "FLAC",
		"MIME Type:":    "audio/flac",
		"Encoder:":      "Lavf62.12.102",
		"Comment:":      "Probe Comment",
	}
	for label, wantText := range want {
		if got[label] != wantText {
			t.Errorf("row %q = %q, want %q", label, got[label], wantText)
		}
	}

	out := ansi.Strip(overlay.Render(80, 40, newTestStyles()))
	for _, want := range []string{"Probe Song", "44100", "audio/flac", "0:04:16"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in rendered output", want)
		}
	}
}

// TestNewTrackInfoOverlay_FFProbeOmissions covers the accepted fallback: when
// ffprobe reports nothing beyond the container basics (a raw stream with no
// tags), the overlay drops every missing row and still renders a well-formed
// panel rather than panicking or emitting blank labels.
func TestNewTrackInfoOverlay_FFProbeOmissions(t *testing.T) {
	tags := map[string]any{
		"SourceFile": "/music/raw.dts",
		"FileType":   "DTS",
		"SampleRate": 48000,
	}
	overlay := NewTrackInfoOverlay("/music/raw.dts", tags, 0, nil)

	for _, row := range overlay.rows {
		if row.Blank {
			continue
		}
		if row.Label == "" || row.Text == "" {
			t.Errorf("half-empty row rendered: %+v", row)
		}
	}

	out := ansi.Strip(overlay.Render(80, 24, newTestStyles()))
	if !strings.Contains(out, "Track Info") || !strings.Contains(out, "DTS") {
		t.Errorf("expected a well-formed panel, got:\n%s", out)
	}
	for _, absent := range []string{"MIME Type:", "Encoder:", "Bitrate:", "Comment:"} {
		if strings.Contains(out, absent) {
			t.Errorf("expected %q to be omitted when ffprobe reports no value", absent)
		}
	}
}
