package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ListeningStats holds aggregated listening data for a given time window.
type ListeningStats struct {
	TotalListenedSecs int
	TotalPlays        int
	TopArtists        []StatEntry // top 5 by play count
	TopAlbums         []StatEntry
	TopGenres         []StatEntry
	TopTracks         []StatEntry
	CurrentStreak     int // consecutive days with ≥1 play
}

// StatEntry holds an aggregated name/play-count/listening-time tuple.
type StatEntry struct {
	Name         string
	PlayCount    int
	ListenedSecs int
}

// ReadListeningStats returns aggregated listening statistics for plays recorded
// at or after sinceUnix. Pass sinceUnix = 0 for all-time stats.
func (d *DB) ReadListeningStats(sinceUnix int64) (ListeningStats, error) {
	var stats ListeningStats

	// 1. Totals
	if err := d.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(listened_secs), 0) FROM play_history WHERE played_at >= ?`,
		sinceUnix,
	).Scan(&stats.TotalPlays, &stats.TotalListenedSecs); err != nil {
		return stats, fmt.Errorf("db: read listening stats totals: %w", err)
	}

	// 2. Top 5 artists
	var err error
	stats.TopArtists, err = readTopEntries(d,
		`SELECT t.artist, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.artist ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top artists: %w", err)
	}

	// 3. Top 5 albums
	stats.TopAlbums, err = readTopEntries(d,
		`SELECT t.album, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.album ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top albums: %w", err)
	}

	// 4. Top 5 genres
	stats.TopGenres, err = readTopEntries(d,
		`SELECT t.genre, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY t.genre ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top genres: %w", err)
	}

	// 5. Top 5 tracks
	stats.TopTracks, err = readTopEntries(d,
		`SELECT t.artist || ' — ' || t.title, COUNT(*), SUM(ph.listened_secs)
		 FROM play_history ph JOIN tracks t ON ph.track_hash = t.hash
		 WHERE ph.played_at >= ? GROUP BY ph.track_hash ORDER BY COUNT(*) DESC LIMIT 5`,
		sinceUnix)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats top tracks: %w", err)
	}

	// 6. Current streak: count consecutive days (including today) that have ≥1 play
	stats.CurrentStreak, err = readCurrentStreak(d)
	if err != nil {
		return stats, fmt.Errorf("db: read listening stats streak: %w", err)
	}

	return stats, nil
}

// readTopEntries runs a query that returns (name, play_count, listened_secs)
// rows and collects them into a []StatEntry.
func readTopEntries(d *DB, query string, arg int64) ([]StatEntry, error) {
	rows, err := d.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []StatEntry
	for rows.Next() {
		var e StatEntry
		if err := rows.Scan(&e.Name, &e.PlayCount, &e.ListenedSecs); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// readCurrentStreak counts consecutive days ending today (in local time) that
// have at least one play_history row.
func readCurrentStreak(d *DB) (int, error) {
	rows, err := d.Query(
		`SELECT DISTINCT date(played_at, 'unixepoch', 'localtime') AS day
		 FROM play_history ORDER BY day DESC`,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	today := time.Now().Format("2006-01-02")
	streak := 0
	expected := today
	for rows.Next() {
		var day string
		if err := rows.Scan(&day); err != nil {
			return 0, err
		}
		if day != expected {
			break
		}
		streak++
		t, err := time.ParseInLocation("2006-01-02", day, time.Local)
		if err != nil {
			break
		}
		expected = t.AddDate(0, 0, -1).Format("2006-01-02")
	}
	return streak, rows.Err()
}

// TrackRow holds a single row from the tracks table. Used by ReadTracks to
// return all cached tracks for pre-populating the library on startup.
type TrackRow struct {
	Hash         string
	Path         string
	Title        string
	Artist       string
	Album        string
	Genre        string
	DurationMs   int64
	CreatedAt    int64
	SizeBytes    int64
	ModTime      int64
	PlayCount    int
	LastPlayedAt int64
}

// ReadTracks returns all non-missing tracks from the tracks table. Returns an
// empty slice (not an error) when the table is empty.
func (d *DB) ReadTracks() ([]TrackRow, error) {
	rows, err := d.Query(`
		SELECT hash, path, title, artist, album, genre, duration_ms, created_at, size_bytes, mod_time, play_count, last_played_at
		FROM tracks WHERE missing = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read tracks: %w", err)
	}
	defer rows.Close()

	var result []TrackRow
	for rows.Next() {
		var r TrackRow
		if err := rows.Scan(&r.Hash, &r.Path, &r.Title, &r.Artist, &r.Album, &r.Genre, &r.DurationMs, &r.CreatedAt, &r.SizeBytes, &r.ModTime, &r.PlayCount, &r.LastPlayedAt); err != nil {
			return nil, fmt.Errorf("db: read track row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UserEditedRow holds the user-edited metadata for a single track.
type UserEditedRow struct {
	Hash   string
	Title  string
	Artist string
	Album  string
	Genre  string
}

// ReadUserEditedTracks returns metadata for all non-missing tracks whose
// metadata was manually edited by the user (user_edited = 1).
func (d *DB) ReadUserEditedTracks() ([]UserEditedRow, error) {
	rows, err := d.Query(`
		SELECT hash, title, artist, album, genre
		FROM tracks WHERE user_edited = 1 AND missing = 0
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read user-edited tracks: %w", err)
	}
	defer rows.Close()

	var result []UserEditedRow
	for rows.Next() {
		var r UserEditedRow
		if err := rows.Scan(&r.Hash, &r.Title, &r.Artist, &r.Album, &r.Genre); err != nil {
			return nil, fmt.Errorf("db: read user-edited row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// QueueRow holds a single queue entry joined with its track metadata.
// Used by ReadQueue to return typed results that callers convert into
// player.QueueTrack and tui.QueueEntry slices.
type QueueRow struct {
	ID             int64
	Hash           string
	Path           string
	Artist         string
	Title          string
	Album          string
	Missing        bool
	DurationMs     int64
	Position       int64
	SourcePlaylist string
}

// SessionRow holds the persisted session state from the single session row.
// CurrentHash is empty when no track was current at the time of the last save.
type SessionRow struct {
	RepeatMode  string
	Shuffle     bool
	CurrentHash string
	PositionMs  int64
	Volume      float64
}

// DB wraps sql.DB with muse-specific setup.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path and configures it for
// WAL mode and safe concurrent use. The caller is responsible for calling
// Close when done. Open creates the parent directory if it does not exist.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("db: create directory %s: %w", dir, err)
		}
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}

	if err := configure(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}

	return &DB{sqlDB}, nil
}

// ReadQueue returns all active (non-deleted) queue entries ordered by
// position, joined with track metadata. Returns an empty slice (not an error)
// when the queue is empty.
//
// Ordering is always by position (original insertion order). shuffle_position
// is a persistence-only field regenerated by WriteShuffle; in-session playback
// randomisation is delegated to mpv's native shuffle property.
func (d *DB) ReadQueue() ([]QueueRow, error) {
	rows, err := d.Query(`
		SELECT q.id, t.hash, t.path, t.artist, t.title, t.album, q.missing,
		       t.duration_ms, q.position, q.source_playlist
		FROM queue q
		JOIN tracks t ON q.track_hash = t.hash
		WHERE q.deleted_at IS NULL
		ORDER BY q.position
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read queue: %w", err)
	}
	defer rows.Close()

	var result []QueueRow
	for rows.Next() {
		var r QueueRow
		var missing int
		if err := rows.Scan(&r.ID, &r.Hash, &r.Path, &r.Artist, &r.Title, &r.Album, &missing, &r.DurationMs, &r.Position, &r.SourcePlaylist); err != nil {
			return nil, fmt.Errorf("db: read queue row: %w", err)
		}
		r.Missing = missing != 0
		result = append(result, r)
	}
	return result, rows.Err()
}

// ReadSession reads the single session row and returns the persisted state.
// Returns nil (no error) when no session row exists.
func (d *DB) ReadSession() (*SessionRow, error) {
	var r SessionRow
	var currentHash sql.NullString
	var shuffle int
	err := d.QueryRow(`
		SELECT repeat_mode, shuffle, current_hash, position_ms, volume
		FROM session WHERE id = 1
	`).Scan(&r.RepeatMode, &shuffle, &currentHash, &r.PositionMs, &r.Volume)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: read session: %w", err)
	}
	r.Shuffle = shuffle != 0
	if currentHash.Valid {
		r.CurrentHash = currentHash.String
	}
	return &r, nil
}

// PlaylistRow holds a single playlist entry with its active track count.
// Used by ReadPlaylists to return typed results for the playlist list view.
// IsSmart is true when the playlist is defined by a query (query IS NOT NULL).
// When IsSmart is true, TrackCount is -1 (tracks are computed on demand) and
// Query holds the raw JSON query string.
type PlaylistRow struct {
	ID         int64
	Name       string
	TrackCount int
	IsSmart    bool
	Query      string
}

// ReadPlaylists returns all active (non-deleted) playlists ordered
// alphabetically by name (case-insensitive), each with its active track count.
// For smart playlists (query IS NOT NULL), TrackCount is -1 and the raw JSON
// query string is populated in the Query field.
// Returns an empty slice (not an error) when no playlists exist.
func (d *DB) ReadPlaylists() ([]PlaylistRow, error) {
	rows, err := d.Query(`
		SELECT p.id, p.name,
		       CASE WHEN p.query IS NOT NULL THEN -1 ELSE COUNT(pt.id) END,
		       p.query IS NOT NULL,
		       COALESCE(p.query, '')
		FROM playlists p
		LEFT JOIN playlist_tracks pt ON pt.playlist_id = p.id AND pt.deleted_at IS NULL
		WHERE p.deleted_at IS NULL
		GROUP BY p.id
		ORDER BY p.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, fmt.Errorf("db: read playlists: %w", err)
	}
	defer rows.Close()

	var result []PlaylistRow
	for rows.Next() {
		var r PlaylistRow
		var isSmart int
		if err := rows.Scan(&r.ID, &r.Name, &r.TrackCount, &isSmart, &r.Query); err != nil {
			return nil, fmt.Errorf("db: read playlist row: %w", err)
		}
		r.IsSmart = isSmart != 0
		result = append(result, r)
	}
	return result, rows.Err()
}

// PlaylistTrackRow holds a single playlist track entry with its position and
// track metadata. Used by ReadPlaylistTracks to return typed results for the
// playlist track-level view.
type PlaylistTrackRow struct {
	ID         int64 // playlist_tracks row ID; required for soft-delete operations
	Position   int64
	Title      string
	Artist     string
	Album      string
	DurationMs int64
	Hash       string // BLAKE3 content hash; required for enqueue operations
	Path       string // file path; required for enqueue operations
}

// ReadPlaylistTracks returns all tracks for the given playlist ID ordered by
// ascending position, joined with track metadata. Returns an empty slice (not
// an error) when the playlist has no tracks.
func (d *DB) ReadPlaylistTracks(playlistID int64) ([]PlaylistTrackRow, error) {
	rows, err := d.Query(`
		SELECT pt.id, pt.position, t.title, t.artist, t.album, t.duration_ms, t.hash, t.path
		FROM playlist_tracks pt
		JOIN tracks t ON pt.track_hash = t.hash
		WHERE pt.playlist_id = ? AND pt.deleted_at IS NULL
		ORDER BY pt.position
	`, playlistID)
	if err != nil {
		return nil, fmt.Errorf("db: read playlist tracks: %w", err)
	}
	defer rows.Close()

	var result []PlaylistTrackRow
	for rows.Next() {
		var r PlaylistTrackRow
		if err := rows.Scan(&r.ID, &r.Position, &r.Title, &r.Artist, &r.Album, &r.DurationMs, &r.Hash, &r.Path); err != nil {
			return nil, fmt.Errorf("db: read playlist track row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// FavoriteRow holds a single row from the favorites table.
type FavoriteRow struct {
	ID        int64
	Type      string
	Value     string
	CreatedAt int64
}

// ReadFavoritesByType returns all favorites of the given type ordered by
// created_at DESC. Returns an empty slice (not an error) when none exist.
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
		var r FavoriteRow
		if err := rows.Scan(&r.ID, &r.Type, &r.Value, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: read favorite row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
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
	return artists, albums, tracks, rows.Err()
}

// IsFavorite reports whether a favorite with the given type and value exists.
func (d *DB) IsFavorite(typ, value string) (bool, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM favorites WHERE type = ? AND value = ?`, typ, value).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("db: is favorite: %w", err)
	}
	return count > 0, nil
}

// configure applies the required PRAGMA settings.
func configure(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("db: %s: %w", p, err)
		}
	}

	return nil
}
