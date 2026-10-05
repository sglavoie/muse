package db

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
)

// FavoriteRow holds a single row from the favorites table.
type FavoriteRow struct {
	ID        int64
	Type      string
	Value     string
	CreatedAt int64
}

// ReadFavoritesByType returns all favorites of the given type ordered by
// created_at DESC. Album values are release identities; legacy title favorites
// expand to matching releases. Returns an empty slice when none exist.
func (d *DB) ReadFavoritesByType(typ string) ([]FavoriteRow, error) {
	rows, err := d.Query(`
		SELECT id, type, value, created_at
		FROM favorites WHERE type = ?
		ORDER BY created_at DESC
	`, typ)
	if err != nil {
		return nil, fmt.Errorf("db: read favorites by type: %w", err)
	}
	defer rows.Close()

	var result []FavoriteRow
	for rows.Next() {
		var row FavoriteRow
		if err := rows.Scan(&row.ID, &row.Type, &row.Value, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: read favorite row: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Release the query before resolving legacy album titles.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if typ == "album" {
		return expandAlbumFavorites(d, result)
	}
	return result, nil
}

// ReadFavoriteCounts returns the number of favorites per type.
func (d *DB) ReadFavoriteCounts() (artists, albums, tracks int, err error) {
	rows, err := d.Query(`SELECT type, COUNT(*) FROM favorites GROUP BY type`)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("db: read favorite counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var typ string
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			return 0, 0, 0, fmt.Errorf("db: read favorite count row: %w", err)
		}
		switch typ {
		case "artist":
			artists = count
		case "album":
			albums = count
		case "track":
			tracks = count
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, 0, 0, err
	}
	albumRows, err := d.ReadFavoritesByType("album")
	return artists, len(albumRows), tracks, err
}

// IsFavorite reports whether a favorite with the given type and value exists.
func (d *DB) IsFavorite(typ, value string) (bool, error) {
	if _, ok := library.ParseAlbumKey(value); typ == "album" && ok {
		rows, err := d.ReadFavoritesByType(typ)
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			if row.Value == value {
				return true, nil
			}
		}
		return false, nil
	}
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM favorites WHERE type = ? AND value = ?`, typ, value).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("db: is favorite: %w", err)
	}
	return count > 0, nil
}
