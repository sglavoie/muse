package tags

import "github.com/sglavoie/muse/internal/db"

func tagRows(names ...string) []db.TagRow {
	rows := make([]db.TagRow, len(names))
	for i, name := range names {
		rows[i] = db.TagRow{ID: int64(i + 1), Name: name}
	}
	return rows
}

func tagCountsFor(rows []db.TagRow, count int) map[int64]int {
	counts := make(map[int64]int, len(rows))
	for _, row := range rows {
		counts[row.ID] = count
	}
	return counts
}

func trackRows(titles ...string) []db.PlaylistTrackRow {
	rows := make([]db.PlaylistTrackRow, len(titles))
	for i, title := range titles {
		rows[i] = db.PlaylistTrackRow{
			ID:     int64(i + 1),
			Hash:   title,
			Title:  title,
			Artist: "artist",
			Album:  "album",
		}
	}
	return rows
}
