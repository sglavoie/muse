package tui

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// DebugPanel holds a snapshot of runtime state used to render the debug panel.
type DebugPanel struct {
	// Player section
	socketPath    string
	connected     bool
	playerInfo    map[string]string // keyed by "codec", "sample_rate", etc.
	playerInfoErr string

	// Playback state section
	playbackState string
	repeatMode    string
	shuffle       bool
	volume        int
	currentHash   string
	queueLen      int
	missingCount  int
	currentIndex  int

	// Library & data section
	trackTotal    int
	scanning      bool
	scanned       int
	scanTotal     int
	playlistCount int
	favoriteCount int
	watchDegraded bool

	// Go runtime section
	goroutines int
	memAlloc   uint64
	memSys     uint64
	numGC      uint32
	goVersion  string

	// TUI state section
	activeView          string
	termWidth           int
	termHeight          int
	configPath          string
	configWarningsCount int
	undoAction          string
	undoExpiresAt       time.Time
}

// collectDebugInfo snapshots all diagnostic state from m into a DebugPanel.
func collectDebugInfo(m Model) DebugPanel {
	var p DebugPanel

	// Player info via type assertion.
	if dbg, ok := m.player.(debugInfoProvider); ok {
		info, err := dbg.DebugInfo()
		p.playerInfo = info
		p.connected = err == nil
		if err != nil {
			p.playerInfoErr = err.Error()
		}
	} else {
		p.playerInfo = map[string]string{
			"codec":         "N/A",
			"sample_rate":   "N/A",
			"bitrate":       "N/A",
			"audio_device":  "N/A",
			"path":          "N/A",
			"audio_filters": "",
		}
	}

	// Socket path from Manager (if available).
	if sp, ok := m.player.(interface{ SocketPath() string }); ok {
		p.socketPath = sp.SocketPath()
	}

	// Playback state.
	switch m.playback.state {
	case PlaybackPlaying:
		p.playbackState = "playing"
	case PlaybackPaused:
		p.playbackState = "paused"
	case PlaybackStopped:
		p.playbackState = "stopped"
	default:
		p.playbackState = "idle"
	}
	p.repeatMode = m.playback.repeatMode.DisplayString()
	p.shuffle = m.playback.shuffle
	p.volume = m.playback.volume
	p.currentHash = m.playback.currentHash
	p.queueLen = len(m.queue.entries)
	p.missingCount = m.missingTrackCount

	p.currentIndex = m.playback.currentIndex

	// Library & data.
	p.trackTotal = m.library.index.Len()
	p.scanning = m.scanning
	p.scanned = m.scanned
	p.scanTotal = m.scanTotal
	p.playlistCount = len(m.playlists.playlists)
	p.favoriteCount = len(m.favoriteHashes)
	p.watchDegraded = m.watchDegraded

	// Go runtime.
	p.goroutines = runtime.NumGoroutine()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p.memAlloc = ms.Alloc
	p.memSys = ms.Sys
	p.numGC = ms.NumGC
	p.goVersion = runtime.Version()

	// TUI state.
	p.activeView = viewName(m.activeView)
	p.termWidth = m.termWidth
	p.termHeight = m.termHeight
	p.configPath = m.configPath
	p.configWarningsCount = len(m.configWarnings)
	if m.undo.slot.action != nil {
		p.undoAction = m.undo.slot.action.description()
		p.undoExpiresAt = m.undo.slot.expiresAt
	} else {
		p.undoAction = "none"
	}

	return p
}

// render builds the debug panel string with section headers and key-value rows.
func (p DebugPanel) render(s styles.Styles) string {
	type row struct{ key, val string }
	type section struct {
		title string
		rows  []row
	}

	connStatus := "disconnected"
	if p.connected {
		connStatus = "connected"
	}
	audioFilters := p.playerInfo["audio_filters"]
	if audioFilters == "" {
		audioFilters = "(none)"
	}

	scanStatus := "idle"
	if p.scanning {
		scanStatus = fmt.Sprintf("scanning (%d / %d)", p.scanned, p.scanTotal)
	}

	watchStatus := "active"
	if p.watchDegraded {
		watchStatus = "degraded"
	}

	shuffleStr := "off"
	if p.shuffle {
		shuffleStr = "on"
	}

	currentIdxStr := "N/A"
	if p.currentIndex >= 0 {
		currentIdxStr = fmt.Sprintf("%d", p.currentIndex)
	}

	undoStr := p.undoAction
	if p.undoAction != "none" && !p.undoExpiresAt.IsZero() {
		undoStr = fmt.Sprintf("%s (expires %s)", p.undoAction, p.undoExpiresAt.Format("15:04:05"))
	}

	sections := []section{
		{
			title: "Player",
			rows: []row{
				{"socket", p.socketPath},
				{"status", connStatus},
				{"codec", p.playerInfo["codec"]},
				{"sample_rate", p.playerInfo["sample_rate"]},
				{"bitrate", p.playerInfo["bitrate"]},
				{"audio_device", p.playerInfo["audio_device"]},
				{"path", p.playerInfo["path"]},
				{"audio_filters", audioFilters},
			},
		},
		{
			title: "Playback",
			rows: []row{
				{"state", p.playbackState},
				{"repeat", p.repeatMode},
				{"shuffle", shuffleStr},
				{"volume", fmt.Sprintf("%d", p.volume)},
				{"current_hash", p.currentHash},
				{"queue_len", fmt.Sprintf("%d", p.queueLen)},
				{"missing", fmt.Sprintf("%d", p.missingCount)},
				{"current_index", currentIdxStr},
			},
		},
		{
			title: "Library",
			rows: []row{
				{"tracks", fmt.Sprintf("%d", p.trackTotal)},
				{"scan", scanStatus},
				{"playlists", fmt.Sprintf("%d", p.playlistCount)},
				{"favorites", fmt.Sprintf("%d", p.favoriteCount)},
				{"watch", watchStatus},
			},
		},
		{
			title: "Runtime",
			rows: []row{
				{"goroutines", fmt.Sprintf("%d", p.goroutines)},
				{"mem_alloc", fmt.Sprintf("%d KiB", p.memAlloc/1024)},
				{"mem_sys", fmt.Sprintf("%d KiB", p.memSys/1024)},
				{"gc_cycles", fmt.Sprintf("%d", p.numGC)},
				{"go_version", p.goVersion},
			},
		},
		{
			title: "TUI",
			rows: []row{
				{"view", p.activeView},
				{"terminal", fmt.Sprintf("%dx%d", p.termWidth, p.termHeight)},
				{"config_path", p.configPath},
				{"config_warnings", fmt.Sprintf("%d", p.configWarningsCount)},
				{"undo", undoStr},
			},
		},
	}

	// Find max key length for alignment.
	maxKeyLen := 0
	for _, sec := range sections {
		for _, r := range sec.rows {
			if len(r.key) > maxKeyLen {
				maxKeyLen = len(r.key)
			}
		}
	}

	var b strings.Builder
	b.WriteString(s.Heading.Render("Debug Panel") + "\n\n")
	for _, sec := range sections {
		b.WriteString(s.Bold.Render(sec.title) + "\n")
		for _, r := range sec.rows {
			fmt.Fprintf(&b, "  %-*s  %s\n", maxKeyLen, r.key, r.val)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
