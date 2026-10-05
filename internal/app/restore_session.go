package app

import (
	"fmt"
	"log/slog"
	"math"
	"os"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// RestoreSession loads the persisted queue and session state from the
// database, validates each queue track against disk, and marks any entry
// whose file cannot be stat-ed as missing. When shuffle is enabled a fresh
// random shuffle ordering is regenerated so that each session starts with a
// new permutation. Returns a non-nil *RestoredSession even when no prior
// session exists (all fields at zero values).
func RestoreSession(database *db.DB, writer *db.Writer) (*RestoredSession, error) {
	rows, err := database.ReadQueue()
	if err != nil {
		return nil, fmt.Errorf("restore session: read queue: %w", err)
	}

	queueEntries, playerTracks := restoreQueue(rows, writer)

	sess, err := database.ReadSession()
	if err != nil {
		return nil, fmt.Errorf("restore session: read session: %w", err)
	}

	// Radio's recent-history avoidance is in-memory only, so without this it
	// starts empty on every launch and tracks heard in the previous session
	// get no penalty. A failure here is not fatal: radio simply falls back to
	// the pre-existing behaviour.
	recentHashes, err := database.ReadRecentPlayedHashes(tui.RadioRecentCap)
	if err != nil {
		slog.Warn("restore session: read recent played hashes", "err", err)
		recentHashes = nil
	}

	restored := buildRestoredSession(queueEntries, playerTracks, sess)
	restored.RecentHashes = recentHashes
	restored.DJ, restored.DJStateErr = restoreDJSession(database)
	return restored, nil
}

// restoreDJSession loads the persisted Auto-DJ session.
//
// A read failure is deliberately not fatal to startup and is deliberately not
// repaired into defaults either. ReadDJSession already refuses to hand back a
// half-usable session — substituting stock settings would trade the user's
// restrictive filters for an unrestricted DJ — so the zero session is carried
// forward with the error beside it. The DJ then starts stopped, its saved state
// is left on disk for repair, and the rest of the session (queue, transport,
// volume) restores exactly as it would have.
func restoreDJSession(database *db.DB) (db.DJSession, error) {
	session, err := database.ReadDJSession()
	if err != nil {
		slog.Warn("restore session: read dj session", "err", err)
		return db.DJSession{}, err
	}
	return session, nil
}

func restoreQueue(rows []db.QueueRow, writer *db.Writer) ([]queue.Entry, []player.QueueTrack) {
	queueEntries := make([]queue.Entry, 0, len(rows))
	playerTracks := make([]player.QueueTrack, 0, len(rows))
	for _, row := range rows {
		missing := resolveQueueEntryMissing(row, writer)
		queueEntries = append(queueEntries, queueEntryFromRow(row, missing))
		playerTracks = append(playerTracks, playerTrackFromRow(row))
	}
	return queueEntries, playerTracks
}

func resolveQueueEntryMissing(row db.QueueRow, writer *db.Writer) bool {
	if !row.Missing {
		if playableFileExists(row.Path) {
			return false
		}

		if err := writer.MarkQueueEntryMissing(row.ID); err != nil {
			slog.Warn("mark queue entry missing", "id", row.ID, "err", err)
		}
		if err := writer.MarkTrackMissing(row.Hash); err != nil {
			slog.Warn("mark track missing", "hash", row.Hash, "err", err)
		}
		return true
	}

	// Re-stat previously-missing entries: if the file has reappeared,
	// clear the stale missing flags so the player restore can proceed.
	if !playableFileExists(row.Path) {
		if err := writer.MarkTrackMissing(row.Hash); err != nil {
			slog.Warn("mark track missing", "hash", row.Hash, "err", err)
		}
		return true
	}

	if err := writer.ClearTrackMissing(row.Hash); err != nil {
		slog.Warn("clear track missing", "hash", row.Hash, "err", err)
	}
	if err := writer.ClearQueueEntriesMissing(row.Hash); err != nil {
		slog.Warn("clear queue entries missing", "hash", row.Hash, "err", err)
	}
	return false
}

func queueEntryFromRow(row db.QueueRow, missing bool) queue.Entry {
	return queue.Entry{
		ID:             row.ID,
		Path:           row.Path,
		Hash:           row.Hash,
		Display:        buildDisplay(row.Artist, row.Title, row.Path),
		Missing:        missing,
		Artist:         row.Artist,
		Title:          row.Title,
		Album:          row.Album,
		AlbumArtist:    row.AlbumArtist,
		DurationMs:     row.DurationMs,
		Position:       row.Position,
		SourcePlaylist: row.SourcePlaylist,
		TrackNumber:    row.TrackNumber,
		DiscNumber:     row.DiscNumber,
		// Ownership and the planner's explanation are restored with the row.
		// Without them every DJ-owned entry would read back as a manual one
		// after a restart, and the first "apply while active" refresh would
		// find nothing of its own to replace.
		DJOwned:   row.DJOwned,
		DJReasons: row.DJReasons,
	}
}

func playerTrackFromRow(row db.QueueRow) player.QueueTrack {
	return player.QueueTrack{
		Path:    row.Path,
		Hash:    row.Hash,
		QueueID: row.ID,
		Artist:  row.Artist,
		Title:   row.Title,
		Source:  row.SourcePlaylist,
	}
}

func buildRestoredSession(
	queueEntries []queue.Entry,
	playerTracks []player.QueueTrack,
	sess *db.SessionRow,
) *RestoredSession {
	repeatMode, shuffle, radioMode, currentHash, positionMs, volume := restoreSessionState(sess)
	return &RestoredSession{
		QueueEntries: queueEntries,
		PlayerTracks: playerTracks,
		RepeatMode:   repeatMode,
		Shuffle:      shuffle,
		RadioMode:    radioMode,
		CurrentHash:  currentHash,
		PositionMs:   positionMs,
		Volume:       volume,
	}
}

func restoreSessionState(sess *db.SessionRow) (playback.RepeatMode, bool, bool, string, int64, int) {
	if sess == nil {
		return playback.RepeatOff, false, false, "", 0, -1
	}

	positionMs := sess.PositionMs
	if positionMs < 0 {
		positionMs = 0
	}

	return parseRepeatMode(sess.RepeatMode),
		sess.Shuffle,
		sess.RadioMode,
		sess.CurrentHash,
		positionMs,
		restoreVolume(sess.Volume)
}

func restoreVolume(volume float64) int {
	// Guard against NaN/Inf in the persisted volume column: int(NaN) is 0
	// on amd64 (silently muting) and int(Inf) is implementation-defined.
	// Treat such values as "no volume persisted" so callers fall back to
	// the configured default rather than mutating the player into garbage.
	if math.IsNaN(volume) || math.IsInf(volume, 0) {
		return -1
	}
	return max(0, min(int(math.Round(volume*100)), 100))
}

// buildDisplay returns a human-readable label for a queue entry, falling back
// to the file path when both artist and title are empty.
func buildDisplay(artist, title, path string) string {
	if artist == "" && title == "" {
		return path
	}
	if artist == "" {
		return title
	}
	if title == "" {
		return artist
	}
	return artist + " – " + title
}

func playableFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// parseRepeatMode converts a DB repeat_mode string to a tui.RepeatMode.
// Unrecognised values are treated as RepeatOff.
func parseRepeatMode(s string) playback.RepeatMode {
	switch s {
	case "all":
		return playback.RepeatAll
	case "one":
		return playback.RepeatOne
	default:
		return playback.RepeatOff
	}
}
