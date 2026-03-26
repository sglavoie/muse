package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTrackInfoOverlayRender(t *testing.T) {
	overlay := TrackInfoOverlay{
		path: "/music/artist/song.flac",
		rows: []OverlayRow{
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
	if !strings.Contains(out, "y to copy path") {
		t.Error("expected copy-path hint")
	}
	if !strings.Contains(out, "i or esc to dismiss") {
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
	overlay := newTrackInfoOverlay("/music/test.flac", tags, 0)

	if overlay.path != "/music/test.flac" {
		t.Errorf("expected path '/music/test.flac', got %q", overlay.path)
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
	overlay := newTrackInfoOverlay("/music/empty.flac", map[string]any{}, 0)

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
	overlay := newTrackInfoOverlay("/music/sparse.flac", tags, 0)

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
	overlay := newTrackInfoOverlay("/music/alt.flac", tags, 0)

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
	overlay := newTrackInfoOverlay("/music/sep.flac", tags, 0)

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
