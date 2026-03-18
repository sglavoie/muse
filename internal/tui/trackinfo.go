package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// trackInfoMsg carries the result of an async exiftool invocation.
type trackInfoMsg struct {
	path string
	tags map[string]any
	err  error
}

// trackInfoCmd returns a tea.Cmd that extracts all exiftool tags for path.
func trackInfoCmd(path string) tea.Cmd {
	return func() tea.Msg {
		tags, err := library.ExtractAllTags(path)
		return trackInfoMsg{path: path, tags: tags, err: err}
	}
}

// TrackInfoOverlay renders a centered overlay showing detailed file metadata.
type TrackInfoOverlay struct {
	path string
	rows []OverlayRow
}

// Render produces a centered overlay string for the track info.
func (t TrackInfoOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	box := OverlayBox{
		Title:         " ℹ Track Info ",
		Rows:          t.rows,
		Hint:          "y to copy path · i or esc to dismiss",
		MaxInnerWidth: termWidth - 10,
	}
	return box.Render(termWidth, termHeight, s)
}

// tagSpec describes a single metadata tag to display.
type tagSpec struct {
	keys  []string // exiftool key names to try in order
	label string   // display label
}

// tagGroups defines the ordered groups of tags shown in the overlay.
// Groups are separated by blank rows.
var tagGroups = [][]tagSpec{
	// Group 1: basic metadata
	{
		{[]string{"Title"}, "Title:"},
		{[]string{"Artist"}, "Artist:"},
		{[]string{"Album"}, "Album:"},
		{[]string{"AlbumArtist"}, "Album Artist:"},
		{[]string{"Genre"}, "Genre:"},
		{[]string{"Track"}, "Track:"},
		{[]string{"DiscNumber"}, "Disc:"},
		{[]string{"Year", "Date"}, "Year:"},
	},
	// Group 2: audio properties
	{
		{[]string{"Duration"}, "Duration:"},
		{[]string{"SampleRate"}, "Sample Rate:"},
		{[]string{"AudioBitrate", "AvgBitrate"}, "Bitrate:"},
		{[]string{"AudioChannels", "NumChannels"}, "Channels:"},
		{[]string{"BitsPerSample"}, "Bits/Sample:"},
	},
	// Group 3: file info
	{
		{[]string{"FileType"}, "File Type:"},
		{[]string{"MIMEType"}, "MIME Type:"},
		{[]string{"FileSize"}, "File Size:"},
	},
	// Group 4: encoder info
	{
		{[]string{"Encoder", "EncoderSettings"}, "Encoder:"},
		{[]string{"Comment"}, "Comment:"},
	},
}

// newTrackInfoOverlay builds a TrackInfoOverlay from exiftool tags.
func newTrackInfoOverlay(path string, tags map[string]any) TrackInfoOverlay {
	rows := []OverlayRow{
		{Label: "Path:", Text: path},
		{Blank: true},
	}

	for gi, group := range tagGroups {
		groupHasRow := false
		for _, spec := range group {
			for _, key := range spec.keys {
				if val, ok := tags[key]; ok {
					rows = append(rows, OverlayRow{
						Label: spec.label,
						Text:  fmt.Sprint(val),
					})
					groupHasRow = true
					break
				}
			}
		}
		// Add a blank separator after non-empty groups (except the last).
		if groupHasRow && gi < len(tagGroups)-1 {
			rows = append(rows, OverlayRow{Blank: true})
		}
	}

	return TrackInfoOverlay{path: path, rows: rows}
}
