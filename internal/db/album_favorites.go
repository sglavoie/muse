package db

import (
	"database/sql"
	"errors"

	"github.com/sglavoie/muse/internal/library"
)

type albumQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func albumTracks(q albumQueryer, title string) ([]library.Track, error) {
	rows, err := q.Query(`SELECT hash, path, artist, album, album_artist FROM tracks WHERE album = ? OR (? = '' AND album = '(unknown album)')`, title, title)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tracks []library.Track
	for rows.Next() {
		var t library.Track
		if err := rows.Scan(&t.Hash, &t.Path, &t.Artist, &t.Album, &t.AlbumArtist); err != nil {
			return nil, err
		}
		tracks = append(tracks, t)
	}
	return tracks, rows.Err()
}

// Old favorites named only an album title. Resolve them to every matching
// release without changing the database during a read. Unavailable titles
// remain visible so an unplugged library never silently loses favorites.
func expandAlbumFavorites(q albumQueryer, favorites []FavoriteRow) ([]FavoriteRow, error) {
	var result []FavoriteRow
	seen := make(map[string]bool)
	for _, row := range favorites {
		values := []string{row.Value}
		if _, canonical := library.ParseAlbumKey(row.Value); !canonical {
			tracks, err := albumTracks(q, library.AlbumTitle(row.Value))
			if err != nil {
				return nil, err
			}
			if keys := library.AlbumKeys(tracks); len(keys) > 0 {
				values = keys
			}
		}
		for _, value := range values {
			if !seen[value] {
				copy := row
				copy.Value = value
				result = append(result, copy)
				seen[value] = true
			}
		}
	}
	return result, nil
}

// Materialize before changing a favorite or its tracks. Splitting one old
// title favorite must not unfavorite other releases with the same title.
func materializeAlbumFavorite(tx *sql.Tx, key string) error {
	a, ok := library.ParseAlbumKey(key)
	if !ok {
		return nil
	}
	if err := materializeAlbumTitle(tx, a.Title, a.Title); err != nil {
		return err
	}
	if a.Title == "" {
		return materializeAlbumTitle(tx, a.Title, "(unknown album)")
	}
	return nil
}

func materializeAlbumTitle(tx *sql.Tx, title, legacy string) error {
	var created int64
	err := tx.QueryRow(`SELECT created_at FROM favorites WHERE type='album' AND value=?`, legacy).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	tracks, err := albumTracks(tx, title)
	if err != nil || len(tracks) == 0 {
		return err
	}
	for _, value := range library.AlbumKeys(tracks) {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO favorites(type,value,created_at) VALUES('album',?,?)`, value, created); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM favorites WHERE type='album' AND value=?`, legacy)
	return err
}

func albumKeyByHash(tx *sql.Tx, hash string) (string, error) {
	var t library.Track
	err := tx.QueryRow(`SELECT path, artist, album, album_artist FROM tracks WHERE hash=?`, hash).
		Scan(&t.Path, &t.Artist, &t.Album, &t.AlbumArtist)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return library.AlbumKey(t), nil
}

func prepareAlbumFavoriteUpdate(tx *sql.Tx, hash string) (string, error) {
	key, err := albumKeyByHash(tx, hash)
	if err == nil && key != "" {
		err = materializeAlbumFavorite(tx, key)
	}
	return key, err
}

// Keep favorites attached when a rescan supplies Album Artist, a file moves,
// or a metadata edit/undo renames an album. A split keeps both resulting
// releases favorited; a merge collapses naturally through the unique index.
func finishAlbumFavoriteUpdate(tx *sql.Tx, oldKey, hash string) error {
	if oldKey == "" {
		return nil
	}
	newKey, err := albumKeyByHash(tx, hash)
	if err != nil || oldKey == newKey || newKey == "" {
		return err
	}
	var created int64
	err = tx.QueryRow(`SELECT created_at FROM favorites WHERE type='album' AND value=?`, oldKey).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO favorites(type,value,created_at) VALUES('album',?,?)`, newKey, created); err != nil {
		return err
	}
	return removeUnusedAlbumFavorite(tx, oldKey)
}

func removeUnusedAlbumFavorite(tx *sql.Tx, oldKey string) error {
	tracks, err := albumTracks(tx, library.AlbumTitle(oldKey))
	if err != nil {
		return err
	}
	for _, t := range tracks {
		if library.AlbumKey(t) == oldKey {
			return nil
		}
	}
	_, err = tx.Exec(`DELETE FROM favorites WHERE type='album' AND value=?`, oldKey)
	return err
}
