package tui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// New creates a root Model. All dependencies are injected via functional
// options (With* functions). Only ctx is required as a positional parameter.
//
// Example:
//
//	model := tui.New(ctx,
//	    tui.WithConfigPath(path, true),
//	    tui.WithScanChannel(scanCh),
//	    tui.WithLibraryRoots(roots),
//	    tui.WithPlayer(mgr),
//	)
func New(ctx context.Context, opts ...Option) Model {
	initialStyles := styles.NewStyles(true)
	m := Model{
		appCtx:          ctx,
		ActiveView:      viewLibrary,
		cfg:             config.Defaults(),
		library:         libtui.View{Fuzzy: &library.CachedFuzzySearcher{}},
		playback:        playbackInfo{currentIndex: -1, requests: &playRequestGate{}},
		registry:        keys.NewRegistry(),
		styles:          &initialStyles,
		terminalHasDark: true,
		insightsPending: &insightsPendingWrites{},
		metadataWrites:  metadata.NewWriteQueue(),
		// Stock DJ settings before anything is restored. The zero
		// radio.Settings has an empty anchor mode and fails validation, so a
		// Model built without WithDJSession would otherwise carry settings no
		// plan could be built from.
		dj: djSessionState{settings: defaultDJSettings()},
	}
	for _, opt := range opts {
		opt(&m)
	}
	m = m.applyTheme()
	// Initialize the palette usage store from the resolved data directory.
	// Load errors are swallowed (best-effort persistence).
	m.paletteUsage = palette.NewUsageStore(m.dataDir)
	_ = m.paletteUsage.Load()
	var historyErr error
	m.filterHistory, historyErr = filter.LoadHistory(m.dataDir)
	if historyErr != nil {
		slog.Warn("filter history: load failed, starting fresh", "err", historyErr)
	}
	// Derive scanning state from scanCh after options are applied.
	m.scanning = m.scanCh != nil
	m = m.initFirstRun()
	// Pre-load the set of favorited track hashes for row highlighting.
	m.favoriteHashes = loadFavoriteSet(m.favoriteReader, "track")
	m.favoriteArtists = loadFavoriteSet(m.favoriteReader, "artist")
	m.favoriteAlbums = loadFavoriteSet(m.favoriteReader, "album")
	// Pre-load dismissed track hashes so mixes filter them on first load.
	m.dismissedHashes = loadDismissedSet(m.dismissalReader)
	if m.freshLibrary {
		m.library = m.library.SwitchMode(libtui.LibraryModeRecent)
	}
	m = m.syncRecentlyAddedFromLibrary()
	m = m.withAlbumGrid()
	m = m.loadSortPreferences().restoreSortPreference().loadBrowsingSession()
	// Footer hint strip default is taken from config (defaults to true).
	m.showFooterHints = m.cfg.ShowFooterHints
	// Queue follow-now-playing default is taken from config (defaults to true).
	m.followNowPlaying = m.cfg.QueueFollowNowPlaying
	// On-track-change notifications are opt-in (default false).
	m.notifyOnTrackChange = m.cfg.NotifyOnTrackChange
	// Sync statusBar fields from model state.
	m.statusBar.TransientDur = time.Duration(m.cfg.TransientDurationSecs) * time.Second
	m.statusBar.ConfigPath = m.configPath
	m.statusBar.ConfigCreated = m.configCreated
	if len(m.configWarnings) > 0 {
		m.statusBar.PushMsg("Config: "+strings.Join(m.configWarnings, "; "), 10*time.Second, statusbar.NotifNormal)
	}
	if m.sessionCount > 0 && m.sessionCount <= 3 {
		m.statusBar.PushMsg("Tip: press : to open command palette", 10*time.Second, statusbar.NotifInfo)
	}
	return m
}

// Volume returns the current volume level as an integer percentage.
func (m Model) Volume() int { return m.playback.volume }

// PlaybackCurrentIndex returns the current queue index for the playing track.
func (m Model) PlaybackCurrentIndex() int { return m.playback.currentIndex }

// PlaybackStateValue returns the current playback state.
func (m Model) PlaybackStateValue() playback.State { return m.playback.state }

// StatusBarArtist returns the current status bar artist string.
func (m Model) StatusBarArtist() string { return m.statusBar.Artist }

// StatusBarTitle returns the current status bar title string.
func (m Model) StatusBarTitle() string { return m.statusBar.Title }

// StatusBarPlaylistSource returns the current status bar playlist source string.
func (m Model) StatusBarPlaylistSource() string { return m.statusBar.PlaylistSource }

// SessionTracksPlayed returns the number of tracks counted as played this session.
func (m Model) SessionTracksPlayed() int { return m.sessionTracksPlayed }

// SessionListenSecs returns the accumulated listening time in seconds this session.
func (m Model) SessionListenSecs() float64 { return m.sessionListenSecs }

// SessionTopArtist returns the artist with the most plays this session, or "" when none.
// Ties are broken by lexicographically smaller artist name to keep the output stable.
func (m Model) SessionTopArtist() string {
	var top string
	var topCount int
	for artist, count := range m.sessionArtistCounts {
		if artist == "" {
			continue
		}
		if count > topCount || (count == topCount && artist < top) {
			top = artist
			topCount = count
		}
	}
	return top
}

// RemoveLibraryTrackByPath removes a track from the in-memory library index
// by its file path. Call this before starting the TUI to evict deleted files
// discovered during the diff scan setup phase.
func (m *Model) RemoveLibraryTrackByPath(path string) {
	m.library.Index.RemoveByPath(path)
	*m = m.syncRecentlyAddedFromLibrary()
}
