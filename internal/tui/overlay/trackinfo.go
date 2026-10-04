package overlay

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TrackInfoMsg carries the result of an async exiftool invocation.
type TrackInfoMsg struct {
	Path      string
	Hash      string
	Tags      map[string]any
	Err       error
	PlayCount int
}

// TrackInfoCmd returns a tea.Cmd that extracts all exiftool tags for path.
func TrackInfoCmd(ctx context.Context, path, hash string, playCount int) tea.Cmd {
	return func() tea.Msg {
		tags, err := library.ExtractAllTags(ctx, path)
		return TrackInfoMsg{Path: path, Hash: hash, Tags: tags, Err: err, PlayCount: playCount}
	}
}

// TrackInfoOverlay renders a centered overlay showing detailed file metadata.
type TrackInfoOverlay struct {
	Path   string
	rows   []Row
	offset int
}

// Render produces a centered overlay string for the track info.
func (t TrackInfoOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	rows := t.wrappedRows(termWidth)
	visible := max(1, termHeight-7)
	start := max(0, min(t.offset, len(rows)-visible))
	end := min(len(rows), start+visible)
	title := " ℹ Track Info "
	if len(rows) > visible {
		title = fmt.Sprintf(" ℹ Track Info · %d–%d/%d ", start+1, end, len(rows))
		if end < len(rows) {
			title += "↓ "
		}
	}
	box := Box{
		Title:         title,
		Rows:          rows[start:end],
		Hint:          "j/k scroll · y to copy path · i or esc to dismiss",
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

// NewTrackInfoOverlay builds a TrackInfoOverlay from exiftool tags.
func NewTrackInfoOverlay(path string, tags map[string]any, playCount int, userTags []db.TagRow) TrackInfoOverlay {
	rows := []Row{
		{Label: "Path:", Text: path},
		{Blank: true},
	}

	for gi, group := range tagGroups {
		groupHasRow := false
		for _, spec := range group {
			for _, key := range spec.keys {
				if val, ok := tags[key]; ok {
					rows = append(rows, Row{
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
			rows = append(rows, Row{Blank: true})
		}
	}

	rows = append(rows, Row{Blank: true})
	rows = append(rows, Row{Label: "Play Count:", Text: strconv.Itoa(playCount)})

	if len(userTags) > 0 {
		names := make([]string, len(userTags))
		for i, t := range userTags {
			names[i] = t.Name
		}
		rows = append(rows, Row{Label: "Tags:", Text: strings.Join(names, ", ")})
	}

	return TrackInfoOverlay{Path: path, rows: rows}
}
