package overlay

import "github.com/sglavoie/muse/internal/library"

// withLibraryMetadata presents the effective values first. File values remain
// visible when different, including absent tags and explicitly blank values.
func (t TrackInfoOverlay) withLibraryMetadata(track library.Track) TrackInfoOverlay {
	fields := []Row{
		{Label: "Title:", Text: track.Title},
		{Label: "Artist:", Text: track.Artist},
		{Label: "Album:", Text: track.Album},
		{Label: "Album Artist:", Text: track.AlbumArtist},
		{Label: "Genre:", Text: track.Genre},
	}
	rows := []Row{{Text: "Library metadata", Styled: true}}
	fileValues := make(map[string]string)
	for _, row := range t.rows {
		if row.Label != "" {
			fileValues[row.Label] = row.Text
		}
	}
	var originals []Row
	for _, field := range fields {
		if original, differs := originalMetadataRow(field, fileValues); differs {
			originals = append(originals, original)
		}
		if field.Text == "" {
			field.Text = "(blank)"
		}
		rows = append(rows, field)
	}
	rows = append(rows, Row{Blank: true}, Row{Text: "File metadata", Styled: true})
	rows = append(rows, originals...)
	for _, row := range t.rows {
		switch row.Label {
		case "Title:", "Artist:", "Album:", "Album Artist:", "Genre:":
			continue
		}
		rows = append(rows, row)
	}
	t.rows = rows
	return t
}

func originalMetadataRow(field Row, fileValues map[string]string) (Row, bool) {
	raw, exists := fileValues[field.Label]
	if raw == field.Text {
		return Row{}, false
	}
	original := Row{Label: "File " + field.Label, Text: raw}
	if !exists {
		original.Text = "(missing)"
	} else if raw == "" {
		original.Text = "(blank)"
	}
	return original, true
}
