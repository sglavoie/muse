package overlay

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TrackInfoMsg carries the result of an asynchronous file metadata probe.
type TrackInfoMsg struct {
	Request uint64
	Path    string
	Tags    map[string]any
	Err     error
}

// TrackInfoCmd extracts file tags through the configured metadata backend.
func TrackInfoCmd(ctx context.Context, path string, request uint64) tea.Cmd {
	return func() tea.Msg {
		tags, err := library.ExtractAllTags(ctx, path)
		return TrackInfoMsg{Request: request, Path: path, Tags: tags, Err: trackInfoError(err)}
	}
}

// TrackInfoOverlay renders stored metadata with asynchronously loaded file details.
type TrackInfoOverlay struct {
	Request  uint64
	track    library.Track
	userTags []db.TagRow
	Path     string
	rows     []Row
	offset   int
}

// Render produces a top-anchored, scrollable track information panel.
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
		Hint:          "esc close · space play/pause · j/k scroll · y copy path",
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

// Preserve the extractor's explanation rather than showing only an exit code.
func trackInfoError(err error) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if detail := strings.TrimSpace(string(exitErr.Stderr)); detail != "" {
			return fmt.Errorf("%s: %w", detail, err)
		}
	}
	return err
}
