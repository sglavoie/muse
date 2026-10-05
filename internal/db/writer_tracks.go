package db

import (
	"database/sql"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// PopulateTrackGenres refreshes (track_hash, genre) rows in track_genres for
// each track. Stale rows for the same hash are deleted first so that genre
// changes (rescan after the file's metadata was edited externally) are not
// shadowed by leftover rows; this keeps ReadTopGenres, ReadHiddenGemsMix,
// ReadNeverPlayedMix and ReadHourGenreAffinities consistent with the canonical
// tracks.genre value. Tracks whose hash is empty are skipped (no junction row
// can reference them via FK).
func (w *Writer) PopulateTrackGenres(tracks []library.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		for _, t := range tracks {
			if t.Hash == "" {
				continue
			}
			genre := t.Genre
			var storedGenre string
			var userEdited int
			if err := tx.QueryRow(
				`SELECT genre, user_edited FROM tracks WHERE hash = ?`,
				t.Hash,
			).Scan(&storedGenre, &userEdited); err != nil {
				return err
			}
			if userEdited != 0 {
				genre = storedGenre
			}
			if _, err := tx.Exec(
				`DELETE FROM track_genres WHERE track_hash = ?`,
				t.Hash,
			); err != nil {
				return err
			}
			genres := library.SplitGenres(genre)
			for _, g := range genres {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`,
					t.Hash, g,
				); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// MarkTrackMissing sets missing=1 on the tracks row identified by hash and
// updates its updated_at timestamp. It is a no-op if no row matches.
func (w *Writer) MarkTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=1, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
	})
}

// ClearTrackMissing sets missing=0 on the tracks row identified by hash and
// updates its updated_at timestamp. Call this during rescan when a file with a
// matching BLAKE3 hash is located again.
func (w *Writer) ClearTrackMissing(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET missing=0, updated_at=? WHERE hash=?`,
			time.Now().Unix(), hash,
		)
		return err
	})
}

// UpdateTrackMetadata updates the title, artist, album, and genre for the track
// identified by hash and sets user_edited=1 so that future rescans preserve
// the manually edited metadata. The track_genres junction table is refreshed
// in the same transaction so that genre-driven recommendation queries
// (ReadTopGenres, ReadHiddenGemsMix, ReadNeverPlayedMix,
// ReadHourGenreAffinities) reflect the edit immediately instead of mixing the
// edited genre with stale entries from before the edit.
func (w *Writer) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)
		oldAlbumKey, err := prepareAlbumFavoriteUpdate(tx, hash)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(
			`UPDATE tracks SET title=?, artist=?, album=?, genre=?, user_edited=1, updated_at=? WHERE hash=?`,
			title, artist, album, genre, time.Now().Unix(), hash,
		); err != nil {
			return err
		}
		if err := finishAlbumFavoriteUpdate(tx, oldAlbumKey, hash); err != nil {
			return err
		}
		if hash != "" {
			if _, err := tx.Exec(
				`DELETE FROM track_genres WHERE track_hash = ?`, hash,
			); err != nil {
				return err
			}
			for _, g := range library.SplitGenres(genre) {
				if _, err := tx.Exec(
					`INSERT OR IGNORE INTO track_genres (track_hash, genre) VALUES (?, ?)`,
					hash, g,
				); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// UpdateTrackPosition updates the track and disc number for the track
// identified by hash and sets user_edited=1 so that the UpsertBatch guards on
// track_number/disc_number keep a rescan from restoring the file's own tags.
// Unlike UpdateTrackMetadata it touches no genre columns, so no transaction is
// needed. It is a no-op if no row matches.
func (w *Writer) UpdateTrackPosition(hash string, trackNumber, discNumber int) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET track_number=?, disc_number=?, user_edited=1, updated_at=? WHERE hash=?`,
			trackNumber, discNumber, time.Now().Unix(), hash,
		)
		return err
	})
}

// UpdateTrackDuration updates the duration_ms for the track identified by hash.
// It is a no-op if no row matches.
func (w *Writer) UpdateTrackDuration(hash string, durationMs int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_ms=?, updated_at=? WHERE hash=?`,
			durationMs, time.Now().Unix(), hash,
		)
		return err
	})
}

// UpsertBatch writes tracks into the tracks table inside a single transaction
// using an INSERT … ON CONFLICT(hash) DO UPDATE so that existing rows are
// updated in place without cascading deletes on their FK dependants (queue,
// playlist_tracks). created_at is preserved on conflict; all other fields are
// updated to the values supplied by the scanner. Duration is preserved when
// the scanner returns 0 (e.g. tag-less files). Duration is converted from
// seconds (float64) to milliseconds (int64).
//
// Because the upsert clears tracks.missing for every located file, it also
// clears queue.missing for the same hash inside the same transaction. A
// recovered move/rename that reaches the writer must not leave durable queue
// rows grayed out while the tracks row reports the file as present; committing
// both flag clears together keeps the two tables consistent even if the
// process is interrupted between them.
// UpsertBatch is a no-op when tracks is empty.
func (w *Writer) UpsertBatch(tracks []library.Track) error {
	if len(tracks) == 0 {
		return nil
	}
	return w.Exec(func(db *sql.DB) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer rollback(tx)

		now := time.Now().Unix()
		for _, t := range tracks {
			oldAlbumKey, err := prepareAlbumFavoriteUpdate(tx, t.Hash)
			if err != nil {
				return err
			}
			durationMs := int64(t.Duration * 1000)
			createdAt := t.CreatedAt
			if createdAt == 0 {
				createdAt = now
			}
			_, err = tx.Exec(`
INSERT INTO tracks (hash, path, title, artist, album, genre, duration_ms, size_bytes, mod_time, created_at, updated_at, track_number, disc_number, album_artist)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(hash) DO UPDATE SET
path         = excluded.path,
album_artist = excluded.album_artist,
title        = CASE WHEN tracks.user_edited = 0 THEN excluded.title ELSE tracks.title END,
artist       = CASE WHEN tracks.user_edited = 0 THEN excluded.artist ELSE tracks.artist END,
album        = CASE WHEN tracks.user_edited = 0 THEN excluded.album ELSE tracks.album END,
genre        = CASE WHEN tracks.user_edited = 0 THEN excluded.genre ELSE tracks.genre END,
track_number = CASE WHEN tracks.user_edited = 0 THEN excluded.track_number ELSE tracks.track_number END,
disc_number  = CASE WHEN tracks.user_edited = 0 THEN excluded.disc_number ELSE tracks.disc_number END,
duration_ms  = CASE WHEN excluded.duration_ms > 0 THEN excluded.duration_ms ELSE tracks.duration_ms END,
size_bytes   = excluded.size_bytes,
mod_time     = excluded.mod_time,
missing            = 0,
duration_verified  = 0,
updated_at         = excluded.updated_at`,
				t.Hash, t.Path, t.Title, t.Artist, t.Album, t.Genre, durationMs, t.SizeBytes, t.ModTime, createdAt, now, t.TrackNumber, t.DiscNumber, t.AlbumArtist,
			)
			if err != nil {
				return err
			}
			if err := finishAlbumFavoriteUpdate(tx, oldAlbumKey, t.Hash); err != nil {
				return err
			}
			// Clear the durable queue.missing flag for every queued instance
			// of this hash in the same transaction that cleared tracks.missing
			// above, so a recovered move/rename can never leave queue rows
			// grayed out after a partially-applied recovery.
			if t.Hash != "" {
				if _, err := tx.Exec(
					`UPDATE queue SET missing=0 WHERE track_hash=? AND missing=1`,
					t.Hash,
				); err != nil {
					return err
				}
			}
		}
		return tx.Commit()
	})
}

// VerifyTrackDuration updates duration_ms to the given value and sets
// duration_verified=1 for the track identified by hash. Used when ffprobe
// reports a duration that differs materially from the stored value.
func (w *Writer) VerifyTrackDuration(hash string, durationMs int64) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_ms = ?, duration_verified = 1 WHERE hash = ?`,
			durationMs, hash,
		)
		return err
	})
}

// MarkDurationVerified sets duration_verified=1 for the track identified by
// hash without changing the stored duration. Used when ffprobe confirms the
// existing duration is accurate.
func (w *Writer) MarkDurationVerified(hash string) error {
	return w.Exec(func(db *sql.DB) error {
		_, err := db.Exec(
			`UPDATE tracks SET duration_verified = 1 WHERE hash = ?`,
			hash,
		)
		return err
	})
}
