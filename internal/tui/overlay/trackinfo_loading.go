package overlay

import (
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/text"
)

// NewLibraryTrackInfoOverlay is useful before, and independently of, a file probe.
func NewLibraryTrackInfoOverlay(track library.Track, tags []db.TagRow, request uint64, status string) TrackInfoOverlay {
	rows := libraryMetadataRows(track)
	rows = append(rows, Row{Label: "Duration:", Text: text.FormatPositionSecs(track.Duration)},
		Row{Label: "Play Count:", Text: strconv.Itoa(track.PlayCount)})
	if len(tags) > 0 {
		names := make([]string, len(tags))
		for i, tag := range tags {
			names[i] = tag.Name
		}
		rows = append(rows, Row{Label: "Tags:", Text: strings.Join(names, ", ")})
	}
	rows = append(rows, Row{Label: "Path:", Text: track.Path}, Row{Blank: true},
		Row{Text: "File metadata", Styled: true}, Row{Text: status})
	return TrackInfoOverlay{Path: track.Path, Request: request, track: track, userTags: tags, rows: rows}
}

// WithFileResult retains the library snapshot and scroll position on either
// outcome. Unavailable file tags must not be mistaken for absent tags.
func (t TrackInfoOverlay) WithFileResult(msg TrackInfoMsg) TrackInfoOverlay {
	var next TrackInfoOverlay
	if msg.Err != nil {
		next = NewLibraryTrackInfoOverlay(t.track, t.userTags, t.Request,
			"File details unavailable: "+msg.Err.Error())
	} else {
		next = NewLibraryTrackInfoOverlay(t.track, t.userTags, t.Request, "")
		next.rows = next.rows[:len(next.rows)-1] // replace the loading message
		file := NewTrackInfoOverlay(t.Path, msg.Tags, t.track.PlayCount, nil).withLibraryMetadata(t.track)
		next.rows = append(next.rows, fileDetailsOnly(file.rows)...)
	}
	next.offset = t.offset
	return next
}

func libraryMetadataRows(track library.Track) []Row {
	rows := []Row{
		{Text: "Library metadata", Styled: true},
		{Label: "Title:", Text: track.Title},
		{Label: "Artist:", Text: track.Artist},
		{Label: "Album:", Text: track.Album},
		{Label: "Album Artist:", Text: track.AlbumArtist},
		{Label: "Genre:", Text: track.Genre},
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Text == "" {
			rows[i].Text = "(blank)"
		}
	}
	return rows
}

// Keep the library section stable while appending the probe's file details.
func fileDetailsOnly(rows []Row) []Row {
	var result []Row
	inFile := false
	for _, row := range rows {
		if row.Text == "File metadata" {
			inFile = true
			continue
		}
		if !inFile {
			continue
		}
		switch row.Label {
		case "Path:", "Play Count:", "Tags:":
			continue
		}
		result = append(result, row)
	}
	return result
}
