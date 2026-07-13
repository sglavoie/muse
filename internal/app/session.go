package app

import (
	"log/slog"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

// SessionPlayer is the subset of player.Manager methods used by
// ApplyRestoredSession to synchronize restored state into mpv.
type SessionPlayer interface {
	SetQueue(entries []player.QueueTrack)
	SetRepeat(mode string) error
	SetShuffle(enabled bool) error
	RestoreShuffleAnchor(index int)
	SetVolumeAbsolute(volume float64) error
}

// PlaylistReader reads playlists from the database.
type PlaylistReader interface {
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// ApplyRestoredSession configures the TUI model and player with state from the
// previous session: queue, library tracks, playlists, repeat mode, shuffle,
// volume, and current track.
//
// Eviction of files that were deleted from disk since the last session is no
// longer handled here. Those deletions now arrive asynchronously as watch
// deletion events on LibraryResult.TuiWatchCh (interleaved with live watcher
// events), so the background library pipeline owns all reconciliation.
func ApplyRestoredSession(
	model *tui.Model,
	mgr SessionPlayer,
	restored *RestoredSession,
	playlistReader PlaylistReader,
	libraryTracks []library.Track,
	defaultVolume int,
) {
	if restored == nil {
		restored = &RestoredSession{Volume: -1}
	}

	// Sync queue between player and TUI.
	mgr.SetQueue(restored.PlayerTracks)
	tui.WithQueueEntries(restored.QueueEntries)(model)

	// Pre-populate the library with cached tracks for instant display.
	tui.WithLibraryTracks(libraryTracks)(model)

	if playlistReader != nil {
		if playlists, err := playlistReader.ReadPlaylists(); err == nil {
			tui.WithPlaylists(playlists)(model)
		} else {
			slog.Warn("startup: read playlists", "err", err)
		}
	}

	// Apply restored session state to the model so the first render reflects
	// the correct repeat/shuffle/playback state from the previous session.
	tui.WithRepeatMode(restored.RepeatMode)(model)

	// Shuffle order is Go-owned by the player; the DB persists only the boolean
	// flag, so there is nothing to regenerate here. The player's shuffle cycle is
	// (re)anchored around the restored current track further below via
	// RestoreShuffleAnchor, after the shuffle flag has been applied.
	tui.WithShuffle(restored.Shuffle)(model)
	tui.WithRadioMode(restored.RadioMode)(model)
	vol := restored.Volume
	if vol < 0 {
		vol = defaultVolume
	}
	tui.WithVolume(vol)(model)

	// Propagate the restored volume to mpv so the actual playback volume
	// matches the TUI display from the first interaction. SetVolumeAbsolute
	// enqueues the command for replay once the IPC socket connects, so it
	// is safe to call before the socket is ready.
	if err := mgr.SetVolumeAbsolute(float64(vol)); err != nil {
		slog.Debug("startup: set volume", "err", err)
	}

	// Propagate restored repeat/shuffle modes into mpv so its behaviour
	// matches the status bar indicators from the first user interaction.
	if err := mgr.SetRepeat(restored.RepeatMode.String()); err != nil {
		slog.Debug("startup: set repeat", "err", err)
	}
	if err := mgr.SetShuffle(restored.Shuffle); err != nil {
		slog.Debug("startup: set shuffle", "err", err)
	}

	if restored.CurrentHash != "" {
		// Only mark the TUI as having a current track if the hash maps to a
		// real queue entry. Otherwise the TUI carries a "ghost" hash that no
		// queue index points to, causing status bar/now-playing desync.
		foundIdx := -1
		for i, pt := range restored.PlayerTracks {
			if pt.Hash == restored.CurrentHash {
				foundIdx = i
				break
			}
		}
		if foundIdx >= 0 {
			if foundIdx >= len(restored.QueueEntries) || restored.QueueEntries[foundIdx].Missing {
				slog.Warn("startup: restored current track is unavailable; ignoring",
					"hash", restored.CurrentHash)
				return
			}
			tui.WithCurrentTrackHash(restored.CurrentHash)(model)
			// Anchor the player at the restored track. When shuffle is enabled
			// this rebuilds the cycle around foundIdx (fresh permutation of the
			// rest) so the first Next does not replay the restored track; the
			// shuffle flag was already applied by SetShuffle above.
			mgr.RestoreShuffleAnchor(foundIdx)
			tui.WithCurrentIndex(foundIdx)(model)
			// Populate status bar from the queue entry so the first render
			// shows track metadata instead of an empty bar.
			e := restored.QueueEntries[foundIdx]
			source := e.SourcePlaylist
			if source == "" {
				source = "Queue"
			}
			tui.WithStatusBarTrack(e.Artist, e.Title, source)(model)
		} else {
			slog.Warn("startup: restored current_hash does not match any queue entry; ignoring",
				"hash", restored.CurrentHash)
		}
	}
}
