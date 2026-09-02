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
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/filter"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/keys"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/mixes"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/settings"
	"github.com/sglavoie/muse/internal/tui/smartrules"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/styles"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// overlayInfo groups all overlay and modal state.
type overlayInfo struct {
	preview               *overlay.PreviewOverlay
	previewPlayer         player.Previewer
	previewWasPlaying     bool
	previewDurationFilled bool
	nowPlaying            *overlay.NowPlayingOverlay
	queueSummary          *overlay.QueueSummaryOverlay
	modesLegend           *overlay.ModesLegendOverlay
	trackInfo             *overlay.TrackInfoOverlay
	metadataEditor        *metadata.Editor
	metadataImport        *metadata.ImportOverlay
	destPicker            *pltui.DestinationPickerModal
	destPickerTracks      []library.Track
	goTo                  *overlay.GoToModal
	settings              *settings.SettingsOverlay
	smartRuleEditor       *smartrules.SmartRuleEditor
	commandPalette        *palette.CommandPalette
	tagPicker             *tagtui.TagPicker
	tagMergePicker        *tagtui.TagMergePicker
	tagFilterPicker       *tagtui.TagFilterPicker
	favoriteTargetPicker  *favtui.TargetPicker
}

// undoInfo holds the reversible undo slot.
type undoInfo struct {
	slot undoSlot
}

// playbackInfo groups the fields that describe the current playback state.
type playbackInfo struct {
	state             playback.State
	repeatMode        playback.RepeatMode
	shuffle           bool
	volume            int
	currentHash       string
	currentIndex      int // index into queue entries; -1 = nothing playing
	elapsedSecs       float64
	totalSecs         float64
	peakElapsedSecs   float64 // high-water mark: max elapsed seen for current track
	peakTotalSecs     float64 // total duration corresponding to peakElapsedSecs
	listenedSecs      float64 // audio actually played for the current track
	lastSampleSecs    float64 // previous elapsed reading, for delta accumulation
	stopAfterCurrent  bool    // true when "stop after current track" is armed
	durationCorrected bool    // true once mpv duration has been compared/corrected for the current track
	queueExhausted    bool    // true when playback stopped because the queue ran out of tracks

	pending pendingListen   // in-flight listen capture; see root_listen_capture.go
	flushes []pendingListen // listens awaiting persistence, drained once per Update
}

// Model is the root Bubble Tea v2 model. It manages view switching between
// Library, Playlists, Queue, and Artists panels, and renders a persistent status bar.
type Model struct {
	appCtx           context.Context // application-level context for subprocess lifecycle
	ActiveView       viewID
	cfg              config.Config
	configPath       string
	configCreated    bool
	welcomeDismissed bool
	dataDir          string // XDG data directory (contains muse.db); set via WithDataDir
	configWarnings   []string
	// musicDirHint carries config.ConfigHint's instruction block when at least
	// one configured music directory failed validation at startup. Non-empty
	// also makes the welcome overlay fire for users whose config file already
	// existed.
	musicDirHint string
	// musicDirsPartial is true when some music directories failed validation
	// but at least one still validated, so the library is being scanned. It
	// selects the overlay copy: claiming no directories were found would be
	// false while valid roots are feeding tracks in.
	musicDirsPartial bool

	// fixtureSafeCommands is true only for fixture-backed models built by the
	// hidden `muse tui-test` harness. When set, key-driven command-palette
	// selections are gated through the same fixture-safe allowlist as direct
	// `command` actions (isSafeDirectCommand), so a scenario cannot reach a
	// side-effectful command such as Reveal in Finder by opening the palette and
	// pressing enter. It is never set for production models.
	fixtureSafeCommands bool

	library    libtui.View
	playlists  pltui.PlaylistsView
	queue      queue.View
	queueDirty bool // true when queue has unsaved mutations since last save-as-playlist or clear
	browse     browse.BrowseView
	favorites  favtui.FavoritesView
	tagsView   tagtui.TagsView
	mixes      mixes.MixesView
	insights   instui.View

	// scanCh is the channel returned by library.Scan. It is nil when no scan
	// is in progress or after the scan has completed.
	scanCh <-chan library.Track
	// scanOutcome is the hash-stage outcome that accompanies scanCh. A rescan
	// closes its channel identically whether every file was hashed or a worker
	// panicked partway, and the deletion reconciliation below infers deletions
	// from "in the index before, absent after" — so without this handle a
	// panicked hash worker would mark still-present files missing. nil when the
	// scan came from an option rather than from a rescan.
	scanOutcome *library.StageOutcome
	// scanErrCh carries non-fatal walk errors from the initial diff scan.
	// It is already closed when Init() runs; drained once on startup.
	scanErrCh <-chan library.ScanErrorMsg
	scanning  bool
	scanned   int
	scanTotal int // estimated total from the parallel counting goroutine
	// scanCancel cancels the per-scan context derived from m.appCtx. Set when
	// a scan starts; cleared on completion or user-initiated cancel. nil when
	// no scan is in progress.
	scanCancel context.CancelFunc
	// longOp tracks progress, ETA, and the rate-window samples for the
	// currently active long-running operation (e.g. a library scan). The
	// zero value (Active==false) means no long op is running.
	longOp statusbar.LongOpState

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
	// rescanPriorPaths maps every path present in the library index just before
	// a full rescan clears it to that track's hash. Consulted at rescan
	// completion to compute files that vanished from disk (present before,
	// absent after and not merely moved) so the manual-rescan fallback
	// reconciles deletions the same way the live watcher does. Cleared when the
	// rescan finishes or is canceled.
	rescanPriorPaths map[string]string

	// watchCh is the channel returned by library.Watch. It emits WatchEvents
	// (track additions/updates and deletion notifications) and is nil when
	// watching is disabled.
	watchCh <-chan library.WatchEvent

	// auditCh receives DurationFix values from the background duration audit
	// goroutine. It is nil when ffprobe is unavailable or the audit is done.
	auditCh        <-chan library.DurationFix
	auditCorrected int // number of tracks corrected across all audit batches

	// watchStatusCh carries the single asynchronous watcher-availability report
	// emitted once the background library pipeline finishes registering the
	// filesystem watcher. It is drained once by waitForWatchStatus and then
	// closed. Nil when no pipeline was started.
	watchStatusCh <-chan library.WatchStatusMsg

	// watchDegraded is true when the filesystem watcher could not be started
	// (e.g. fd exhaustion). It is set asynchronously from the watcher-status
	// report, not at construction time. The status bar shows a notice so the
	// user knows to use manual rescan.
	watchDegraded bool

	// libraryRoots and extractor are required for manual rescan. When
	// libraryRoots is empty the rescan hotkey is a no-op.
	libraryRoots []string
	extractor    library.MetadataExtractor

	statusBar        statusbar.StatusBar
	termWidth        int
	termHeight       int
	registry         keys.Registry
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
	queueWriter         libtui.QueueAppender         // may be nil; set via WithQueueWriter
	reorderer           queueReorderer               // may be nil; set via WithQueueReorderer
	undoer              queueDequeuer                // may be nil; set via WithQueueDequeuer
	sessionWriter       sessionWriter                // may be nil; set via WithSessionWriter
	playlistReader      playlistTrackReader          // may be nil; set via WithPlaylistTrackReader
	playlistWriter      playlistCreator              // may be nil; set via WithPlaylistWriter
	playlistRenamer     playlistRenamer              // may be nil; set via WithPlaylistRenamer
	playlistDeleter     playlistDeleter              // may be nil; set via WithPlaylistDeleter
	playlistAdder       playlistTrackAdder           // may be nil; set via WithPlaylistAdder
	batchEnqueuer       batchEnqueuer                // may be nil; set via WithBatchEnqueuer
	favoriteReader      favoriteReader               // may be nil; set via WithFavoriteReader
	favoriteWriter      favoriteWriter               // may be nil; set via WithFavoriteWriter
	favoriteHashes      map[string]struct{}          // set of favorited track hashes for highlight
	favoriteArtists     map[string]struct{}          // set of favorited artist names for highlight
	favoriteAlbums      map[string]struct{}          // set of favorited album names for highlight
	smartEval           smartPlaylistEvaluator       // may be nil; set via WithSmartPlaylistEvaluator
	smartPlaylistWriter smartPlaylistWriter          // may be nil; set via WithSmartPlaylistWriter
	playCountWriter     playCountIncrementer         // may be nil; set via WithPlayCountWriter
	historyRecorder     playHistoryRecorder          // may be nil; set via WithHistoryRecorder
	historyReader       playHistoryReader            // may be nil; set via WithHistoryReader
	insightsReader      insightsReader               // may be nil; set via WithInsightsReader
	insightsRand        func(n int) int              // nil uses the global source; seeded in tests
	insightsPending     *insightsPendingWrites       // database writes an Insights reload must wait for
	recReader           recommendationReader         // may be nil; set via WithRecommendationReader
	dismissalReader     dismissalReader              // may be nil; set via WithDismissalReader
	dismissalWriter     dismissalWriter              // may be nil; set via WithDismissalWriter
	dismissedHashes     map[string]struct{}          // set of dismissed track hashes
	cacheReader         cachedMixReader              // may be nil; set via WithCacheReader
	cacheRebuilder      recommendationCacheRebuilder // may be nil; set via WithCacheRebuilder
	tagReader           tagReader                    // may be nil; set via WithTagReader
	notifier            trackNotifier                // may be nil; set via WithTrackNotifier
	tagWriter           tagWriter                    // may be nil; set via WithTagWriter

	// Tag in-memory cache — loaded on startup, refreshed on any mutation.
	tags      []db.TagRow
	trackTags map[string]map[int64]struct{} // hash → set of tag IDs
	tagNames  map[int64]string              // id → name (quick display lookup)
	tagColors map[int64]string              // id → color hex

	// activeTagFilter, when non-nil, restricts track-displaying views to only
	// show tracks that carry the given tag ID. Set via ctrl+t; cleared by
	// pressing ctrl+t again or esc in the tag filter picker.
	activeTagFilter *int64

	overlay             overlayInfo          // all overlay and modal state
	metadataWriter      metadataUpdater      // may be nil; set via WithMetadataWriter
	durationWriter      durationUpdater      // may be nil; set via WithDurationWriter
	trackPositionWriter trackPositionUpdater // may be nil; set via WithTrackPositionWriter
	metadataWrites      *metadata.WriteQueue // orders the async metadata row writes
	scanWriter          scanUpsertBatcher    // may be nil; set via WithScanWriter
	upsertBatchSize     int                  // batch size for scan DB writes; set via WithScanWriter

	undo            undoInfo       // reversible undo slot and pending confirmation flags
	confirmationBar string         // when non-empty, rendered as a full-width warning between body and status bar
	exportCopyHint  exportPathCopy // arms y to copy the last export path while its notification is on screen
	visual          visualState    // Vim-like visual line selection mode state
	yankBuffer      []queue.Entry  // clipboard for queue yank/paste; persists until next yank
	yankIsCut       bool           // true when the buffer was populated via cut (ctrl+x)

	// missingTrackCount tracks how many queue entries were skipped due to
	// missing files during the current playback session. Reset when playback
	// ends or the queue is cleared.
	missingTrackCount int

	// sessionTracksPlayed and sessionListenSecs accumulate across the lifetime
	// of the program. Printed to stderr as a session summary on quit.
	sessionTracksPlayed int
	sessionListenSecs   float64
	sessionArtistCounts map[string]int // counts of plays per artist this session; used to derive the top artist on quit

	// pulseUntil drives a brief inversion of the now-playing row on track
	// changes. Set in handleTrackChangedMsg; checked at render time.
	pulseUntil time.Time

	// sessionCount is the number of times the app has been launched (including
	// the current session). Set via WithSessionCount. Used to show one-time tips.
	sessionCount int

	// freshLibrary is true when this is the first-ever library scan. Set via
	// WithFreshLibrary to switch to Recently Added mode on first launch.
	freshLibrary bool

	playback playbackInfo // current playback state, repeat, shuffle, volume, position
	radio    radioState   // auto-DJ / radio mode state

	// runtimeDir is the XDG_RUNTIME_DIR-based directory for ephemeral sockets
	// (e.g. preview player). Set via WithRuntimeDir before tea.NewProgram.
	runtimeDir string

	// styles holds the current lipgloss style set. Initialized to dark-safe
	// defaults in New(); replaced on tea.BackgroundColorMsg once the terminal
	// reports its actual background color.
	styles *styles.Styles

	// paletteUsage tracks command palette usage counts for MRU ranking.
	// May be nil when persistence is disabled (e.g. headless tests).
	paletteUsage *palette.UsageStore

	// showFooterHints toggles the context-aware footer hint strip rendered
	// above the status bar. Initialized from cfg.ShowFooterHints after
	// options apply; flipped at runtime via the "Toggle footer hints"
	// palette command.
	showFooterHints bool

	// followNowPlaying gates auto-scrolling the Queue view so the
	// now-playing row stays ~⅓ from the top on track change and view
	// enter. Initialized from cfg.QueueFollowNowPlaying; flipped via the
	// "Toggle queue follow" palette command.
	followNowPlaying bool

	// notifyOnTrackChange gates the macOS banner posted through notifier
	// whenever playback advances to a genuinely new track. Initialized from
	// cfg.NotifyOnTrackChange; flipped via the "Toggle track notifications"
	// palette command or the Display settings tab.
	notifyOnTrackChange bool
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
		appCtx:          ctx,
		ActiveView:      viewLibrary,
		cfg:             config.Defaults(),
		library:         libtui.View{Fuzzy: library.SahilmFuzzySearcher{}},
		playback:        playbackInfo{currentIndex: -1},
		registry:        keys.NewRegistry(),
		styles:          &initialStyles,
		insightsPending: &insightsPendingWrites{},
		metadataWrites:  metadata.NewWriteQueue(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	// Initialize the palette usage store from the resolved data directory.
	// Load errors are swallowed (best-effort persistence).
	m.paletteUsage = palette.NewUsageStore(m.dataDir)
	_ = m.paletteUsage.Load()
	// Derive scanning state from scanCh after options are applied.
	m.scanning = m.scanCh != nil
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
