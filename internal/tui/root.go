package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/filter"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// overlayInfo groups all overlay and modal state.
type overlayInfo struct {
	preview               *PreviewOverlay
	previewPlayer         player.Previewer
	previewWasPlaying     bool
	previewDurationFilled bool
	nowPlaying            *NowPlayingOverlay
	queueSummary          *QueueSummaryOverlay
	trackInfo             *TrackInfoOverlay
	metadataEditor        *MetadataEditor
	destPicker            *pltui.DestinationPickerModal
	destPickerTracks      []library.Track
	goTo                  *GoToModal
	settings              *SettingsOverlay
	smartRuleEditor       *SmartRuleEditor
	commandPalette        *CommandPalette
	tagPicker             *TagPicker
	tagMergePicker        *TagMergePicker
	tagFilterPicker       *TagFilterPicker
}

// undoInfo groups the fields that track pending destructive-action confirmations
// and the reversible undo slot.
type undoInfo struct {
	slot                undoSlot
	clearQueuePending   bool
	clearQueuePendingAt time.Time
	bulkRemovePending   bool
	bulkRemovePendingAt time.Time
}

// playbackInfo groups the fields that describe the current playback state.
type playbackInfo struct {
	state            PlaybackState
	repeatMode       RepeatMode
	shuffle          bool
	volume           int
	currentHash      string
	currentIndex     int // index into queue entries; -1 = nothing playing
	elapsedSecs      float64
	totalSecs        float64
	stopAfterCurrent  bool // true when "stop after current track" is armed
	durationCorrected bool // true once mpv duration has been compared/corrected for the current track
}

// Model is the root Bubble Tea v2 model. It manages view switching between
// Library, Playlists, Queue, and Artists panels, and renders a persistent status bar.
type Model struct {
	appCtx         context.Context // application-level context for subprocess lifecycle
	activeView     viewID
	cfg            config.Config
	configPath     string
	configCreated  bool
	dataDir        string // XDG data directory (contains muse.db); set via WithDataDir
	configWarnings []string

	library       LibraryView
	recentlyAdded LibraryView
	history       LibraryView
	playlists     PlaylistsView
	queue         QueueView
	artists       ArtistsView
	albums        AlbumsView
	favorites     FavoritesView
	genres        GenresView
	tagsView      TagsView

	// scanCh is the channel returned by library.Scan. It is nil when no scan
	// is in progress or after the scan has completed.
	scanCh <-chan library.Track
	// scanErrCh carries non-fatal walk errors from the initial diff scan.
	// It is already closed when Init() runs; drained once on startup.
	scanErrCh <-chan library.ScanErrorMsg
	scanning  bool
	scanned   int
	scanTotal int // estimated total from the parallel counting goroutine

	// userEditedReader queries the DB for user-edited tracks so that
	// rescan and watcher flows can preserve edited metadata in-memory.
	userEditedReader userEditedReader
	// userEditedOverlay caches user-edited metadata keyed by hash.
	// Built at startup and before rescan; consulted when inserting
	// scanned/watched tracks into the in-memory index.
	userEditedOverlay map[string]db.UserEditedRow
	// rescanSnapshot caches created_at and duration for every track in the
	// library index just before a full rescan clears it. Consulted by
	// preserveTrackMetadata so in-memory values survive the rescan.
	rescanSnapshot map[string]rescanEntry

	// watchCh is the channel returned by library.Watch. It emits WatchEvents
	// (track additions/updates and deletion notifications) and is nil when
	// watching is disabled.
	watchCh <-chan library.WatchEvent

	// auditCh receives DurationFix values from the background duration audit
	// goroutine. It is nil when ffprobe is unavailable or the audit is done.
	auditCh <-chan library.DurationFix

	// watchDegraded is true when the filesystem watcher could not be started
	// (e.g. fd exhaustion). The status bar shows a notice so the user knows
	// to use manual rescan.
	watchDegraded bool

	// libraryRoots and extractor are required for manual rescan. When
	// libraryRoots is empty the rescan hotkey is a no-op.
	libraryRoots []string
	extractor    library.MetadataExtractor

	statusBar        StatusBar
	termWidth        int
	termHeight       int
	registry         Registry
	helpVisible      bool           // true while the help overlay is shown
	helpViewport     viewport.Model // scrollable viewport for the help overlay
	helpFiltered     bool           // true when showing only current-view keybindings
	helpSearchFilter filter.State   // search filter state for the help overlay
	prevActiveView   viewID         // the view that was active before help was opened

	debugVisible  bool           // true while the debug panel is shown
	debugViewport viewport.Model // scrollable viewport for the debug panel
	debugPrevView viewID         // the view that was active before debug was opened

	// player is the playback backend. It may be nil when playback is not
	// available (e.g. in tests that do not exercise playback).
	player              playPauser
	queueWriter         libtui.QueueAppender   // may be nil; set via WithQueueWriter
	reorderer           queueReorderer         // may be nil; set via WithQueueReorderer
	undoer              queueDequeuer          // may be nil; set via WithQueueDequeuer
	sessionWriter       sessionWriter          // may be nil; set via WithSessionWriter
	playlistReader      playlistTrackReader    // may be nil; set via WithPlaylistTrackReader
	playlistWriter      playlistCreator        // may be nil; set via WithPlaylistWriter
	playlistRenamer     playlistRenamer        // may be nil; set via WithPlaylistRenamer
	playlistDeleter     playlistDeleter        // may be nil; set via WithPlaylistDeleter
	playlistAdder       playlistTrackAdder     // may be nil; set via WithPlaylistAdder
	batchEnqueuer       batchEnqueuer          // may be nil; set via WithBatchEnqueuer
	favoriteReader      favoriteReader         // may be nil; set via WithFavoriteReader
	favoriteWriter      favoriteWriter         // may be nil; set via WithFavoriteWriter
	favoriteHashes      map[string]struct{}    // set of favorited track hashes for highlight
	favoriteArtists     map[string]struct{}    // set of favorited artist names for highlight
	favoriteAlbums      map[string]struct{}    // set of favorited album names for highlight
	smartEval           smartPlaylistEvaluator // may be nil; set via WithSmartPlaylistEvaluator
	smartPlaylistWriter smartPlaylistWriter    // may be nil; set via WithSmartPlaylistWriter
	playCountWriter     playCountIncrementer   // may be nil; set via WithPlayCountWriter
	historyRecorder     playHistoryRecorder    // may be nil; set via WithHistoryRecorder
	historyReader       playHistoryReader      // may be nil; set via WithHistoryReader
	statsReader         listeningStatsReader   // may be nil; set via WithStatsReader
	tagReader           tagReader              // may be nil; set via WithTagReader
	tagWriter           tagWriter              // may be nil; set via WithTagWriter

	// Tag in-memory cache — loaded on startup, refreshed on any mutation.
	tags      []db.TagRow
	trackTags map[string]map[int64]struct{} // hash → set of tag IDs
	tagNames  map[int64]string              // id → name (quick display lookup)
	tagColors map[int64]string              // id → color hex

	// activeTagFilter, when non-nil, restricts track-displaying views to only
	// show tracks that carry the given tag ID. Set via ctrl+t; cleared by
	// pressing ctrl+t again or esc in the tag filter picker.
	activeTagFilter *int64

	statsVisible  bool           // true while the listening stats overlay is shown
	statsViewport viewport.Model // scrollable viewport for the stats overlay
	statsPrevView viewID         // the view that was active before stats was opened
	statsPeriod   int            // index into statsPeriods slice

	overlay         overlayInfo       // all overlay and modal state
	metadataWriter  metadataUpdater   // may be nil; set via WithMetadataWriter
	durationWriter  durationUpdater   // may be nil; set via WithDurationWriter
	scanWriter      scanUpsertBatcher // may be nil; set via WithScanWriter
	upsertBatchSize int               // batch size for scan DB writes; set via WithScanWriter

	undo       undoInfo     // reversible undo slot and pending confirmation flags
	visual     visualState  // Vim-like visual line selection mode state
	yankBuffer []QueueEntry // clipboard for queue yank/paste; persists until next yank

	// missingTrackCount tracks how many queue entries were skipped due to
	// missing files during the current playback session. Reset when playback
	// ends or the queue is cleared.
	missingTrackCount int

	playback playbackInfo // current playback state, repeat, shuffle, volume, position

	// runtimeDir is the XDG_RUNTIME_DIR-based directory for ephemeral sockets
	// (e.g. preview player). Set via WithRuntimeDir before tea.NewProgram.
	runtimeDir string

	// styles holds the current lipgloss style set. Initialized to dark-safe
	// defaults in New(); replaced on tea.BackgroundColorMsg once the terminal
	// reports its actual background color.
	styles *styles.Styles
}

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
		appCtx:        ctx,
		activeView:    viewLibrary,
		cfg:           config.Defaults(),
		library:       LibraryView{fuzzy: library.SahilmFuzzySearcher{}},
		playback:      playbackInfo{currentIndex: -1},
		recentlyAdded: newRecentlyAddedView(),
		history:       newHistoryView(),
		registry:      newRegistry(),
		styles:        &initialStyles,
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Derive scanning state from scanCh after options are applied.
	m.scanning = m.scanCh != nil
	// Pre-load the set of favorited track hashes for row highlighting.
	m.favoriteHashes = loadFavoriteTrackHashes(m.favoriteReader)
	m.favoriteArtists = loadFavoriteArtists(m.favoriteReader)
	m.favoriteAlbums = loadFavoriteAlbums(m.favoriteReader)
	m = m.syncRecentlyAddedFromLibrary()
	// Sync statusBar fields from model state.
	m.statusBar.configPath = m.configPath
	m.statusBar.configCreated = m.configCreated
	if len(m.configWarnings) > 0 {
		m.statusBar.notification = newNotificationFor(
			"Config: "+strings.Join(m.configWarnings, "; "),
			10*time.Second,
		)
	}
	return m
}

// Volume returns the current volume level as an integer percentage.
func (m Model) Volume() int { return m.playback.volume }

// PlaybackCurrentIndex returns the current queue index for the playing track.
func (m Model) PlaybackCurrentIndex() int { return m.playback.currentIndex }

// StatusBarArtist returns the current status bar artist string.
func (m Model) StatusBarArtist() string { return m.statusBar.artist }

// StatusBarTitle returns the current status bar title string.
func (m Model) StatusBarTitle() string { return m.statusBar.title }

// StatusBarPlaylistSource returns the current status bar playlist source string.
func (m Model) StatusBarPlaylistSource() string { return m.statusBar.playlistSource }

// RemoveLibraryTrackByPath removes a track from the in-memory library index
// by its file path. Call this before starting the TUI to evict deleted files
// discovered during the diff scan setup phase.
func (m *Model) RemoveLibraryTrackByPath(path string) {
	m.library.index.RemoveByPath(path)
	updated := m.syncRecentlyAddedFromLibrary()
	m.recentlyAdded = updated.recentlyAdded
}
