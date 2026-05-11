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
	SetCurrentIndex(index int)
	SetVolumeAbsolute(volume float64) error
}

// ShuffleWriter can regenerate shuffle positions in the database.
type ShuffleWriter interface {
	WriteShuffle(enabled bool) error
	MarkTrackMissing(hash string) error
}

// PlaylistReader reads playlists from the database.
type PlaylistReader interface {
	ReadPlaylists() ([]db.PlaylistRow, error)
}

// ApplyRestoredSession configures the TUI model and player with state from the
// previous session: queue, library tracks, playlists, repeat mode, shuffle,
// volume, current track, and evicts deleted files from the library index.
func ApplyRestoredSession(
	model *tui.Model,
	mgr SessionPlayer,
	restored *RestoredSession,
	shuffleWriter ShuffleWriter,
	playlistReader PlaylistReader,
	libraryTracks []library.Track,
	deletedPaths []string,
	hashByPath map[string]string,
	defaultVolume int,
) {
	// Sync queue between player and TUI.
	mgr.SetQueue(restored.PlayerTracks)
	tui.WithQueueEntries(restored.QueueEntries)(model)

	// Pre-populate the library with cached tracks for instant display.
	tui.WithLibraryTracks(libraryTracks)(model)

	// Evict deleted files from the library index before the TUI starts.
	for _, path := range deletedPaths {
		model.RemoveLibraryTrackByPath(path)
		if hash, ok := hashByPath[path]; ok {
			if err := shuffleWriter.MarkTrackMissing(hash); err != nil {
				slog.Warn("startup: mark deleted track missing", "path", path, "err", err)
			}
		}
	}

	if playlists, err := playlistReader.ReadPlaylists(); err == nil {
		tui.WithPlaylists(playlists)(model)
	} else {
		slog.Warn("startup: read playlists", "err", err)
	}

	// Apply restored session state to the model so the first render reflects
	// the correct repeat/shuffle/playback state from the previous session.
	tui.WithRepeatMode(restored.RepeatMode)(model)

	// Regenerate shuffle_position for all queue rows when shuffle is on.
	if restored.Shuffle && len(restored.QueueEntries) > 0 {
		if err := shuffleWriter.WriteShuffle(true); err != nil {
			slog.Warn("startup: regenerate shuffle positions", "err", err)
		}
	}
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
		tui.WithCurrentTrackHash(restored.CurrentHash)(model)
		for i, pt := range restored.PlayerTracks {
			if pt.Hash == restored.CurrentHash {
				mgr.SetCurrentIndex(i)
				tui.WithCurrentIndex(i)(model)
				// Populate status bar from the queue entry so the first
				// render shows track metadata instead of an empty bar.
				if i < len(restored.QueueEntries) {
					e := restored.QueueEntries[i]
					source := e.SourcePlaylist
					if source == "" {
						source = "Queue"
					}
					tui.WithStatusBarTrack(e.Artist, e.Title, source)(model)
				}
				break
			}
		}
	}
}
