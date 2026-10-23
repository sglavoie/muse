package tui

import (
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui/autodj"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/settings"
	"github.com/sglavoie/muse/internal/tui/smartrules"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// overlayInfo groups all overlay and modal state.
type overlayInfo struct {
	messages              *overlay.MessagesOverlay
	preview               *overlay.PreviewOverlay
	previewPlayer         player.Previewer
	previewStarting       bool
	previewWasPlaying     bool
	previewDurationFilled bool
	nowPlaying            *overlay.NowPlayingOverlay
	queueSummary          *overlay.QueueSummaryOverlay
	modesLegend           *overlay.ModesLegendOverlay
	trackInfo             *overlay.TrackInfoOverlay
	trackInfoRequest      uint64
	metadataEditor        *metadata.Editor
	metadataImport        *metadata.ImportOverlay
	destPicker            *pltui.DestinationPickerModal
	destPickerTracks      []library.Track
	goTo                  *overlay.GoToModal
	settings              *settings.SettingsOverlay
	smartRuleEditor       *smartrules.SmartRuleEditor
	seekTime              *seekTimeState
	commandPalette        *palette.CommandPalette
	tagPicker             *tagtui.TagPicker
	tagMergePicker        *tagtui.TagMergePicker
	tagFilterPicker       *tagtui.TagFilterPicker
	favoriteTargetPicker  *favtui.TargetPicker
	autoDJ                *autodj.TunePanel
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

	// requestSeq is the identity of the newest ordered player request of any
	// kind. playRequestSeq separately remembers the newest selection whose
	// PlayFromIndexResultMsg is still authoritative: a later transport control
	// must order after that selection without hiding a selection failure.
	requestSeq uint64
	// Every selection dispatch stamps its result message with playRequestSeq, so
	// an outcome that a newer selection or teardown superseded is recognisable
	// even when both requests named the same queue occurrence.
	playRequestSeq uint64
	// requests carries the same identity into the goroutines those requests run
	// on, which is what makes them reach the player in dispatch order instead of
	// in scheduler order. It is a pointer so every copy of the Model shares one
	// gate; see root_play_request.go.
	requests *playRequestGate
	// pendingSelection is true while a dispatched PlayFromIndex has neither been
	// confirmed by the player nor resolved by its own result. Until then
	// currentIndex/currentHash above are the optimistic row the user asked for
	// rather than the row the player is holding, and must not be read as the
	// current track. A replay does not set it: ReplayCurrent restarts the
	// current entry without moving it, so what is current stays confirmed.
	pendingSelection bool
	// awaitingSelectionStart is true from the moment a selection is dispatched
	// until the player's own start-file names the selected row. It is the barrier
	// that lets handleTrackChangedMsg refuse a report about any other row, and it
	// deliberately outlives pendingSelection: a successful PlayFromIndex only
	// proves mpv accepted the load, not that every start-file mpv emitted before
	// it has been delivered. The event reader blocks in SyncPosition between
	// reading a start-file and dispatching it, so the row an abandoned selection
	// briefly started can still be reported after the newer load has returned.
	// Retiring the barrier on that return would let the late report move the
	// current row onto a track the player has already left.
	awaitingSelectionStart bool
	// awaitingSelectionID is the queue row id the barrier above is waiting for.
	// The selected row's content hash cannot stand in for it: two occurrences of
	// one track share that value, so the abandoned duplicate's delayed report
	// would be read as the selected one's arrival and retire the barrier the
	// newer selection is relying on. Zero when the selected row is not persisted,
	// which is the only case that falls back to the hash.
	awaitingSelectionID int64

	selectionListen selectionListen // outgoing measurement awaiting a selection outcome
	pending         pendingListen   // in-flight listen capture; see root_listen_capture.go
	flushes         []pendingListen // listens awaiting persistence, drained once per Update
}
