package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// StatusBar manages the persistent status bar at the bottom of the screen.
// transientRight holds a temporary message (e.g. scan progress) that is
// cleared when the scan completes.
//
// The persistent left zone shows playback state, track artist/title, active
// playlist source, and repeat/shuffle mode indicators. The right zone shows
// auto-dismissing transient notifications.
type StatusBar struct {
	logPath        string
	configCreated  bool
	configPath     string
	transientRight string

	// Persistent left zone fields — set during Update when track/state changes.
	artist         string
	title          string
	playlistSource string

	// Playback mode fields — injected by renderStatusBar from the root Model.
	playbackState PlaybackState
	repeat        bool
	shuffle       bool
}

// handleScanProgress sets the transient right zone to a formatted progress string.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) handleScanProgress(msg library.ScanProgressMsg) {
	if msg.Total > 0 {
		s.transientRight = fmt.Sprintf("Scanning… %s / %s tracks", formatCount(msg.Scanned), formatCount(msg.Total))
	} else {
		s.transientRight = fmt.Sprintf("Scanning… %s tracks", formatCount(msg.Scanned))
	}
}

// handleScanDone clears the transient right zone.
func (s *StatusBar) handleScanDone() {
	s.transientRight = ""
}

// handleMissingTrack sets the transient right zone to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.transientRight = fmt.Sprintf("Error: track not found, skipped: %s", path)
}

// render builds the full status bar string. The persistent left zone shows
// playback state indicator, track info, playlist source, and mode indicators.
// The right zone shows transient notifications.
func (s StatusBar) render() string {
	var segments []string

	// Persistent left zone: shown whenever a track is loaded or state is active.
	if s.playbackState != PlaybackIdle || s.artist != "" || s.title != "" {
		stateIcon := ""
		switch s.playbackState {
		case PlaybackPlaying:
			stateIcon = "▶"
		case PlaybackPaused:
			stateIcon = "⏸"
		case PlaybackStopped:
			stateIcon = "■"
		}

		trackInfo := ""
		switch {
		case s.artist != "" && s.title != "":
			trackInfo = s.artist + " – " + s.title
		case s.title != "":
			trackInfo = s.title
		case s.artist != "":
			trackInfo = s.artist
		}

		left := ""
		switch {
		case stateIcon != "" && trackInfo != "":
			left = stateIcon + " " + trackInfo
		case stateIcon != "":
			left = stateIcon
		case trackInfo != "":
			left = trackInfo
		}

		if s.playlistSource != "" {
			left += "  " + s.playlistSource
		}
		if s.repeat {
			left += "  R"
		}
		if s.shuffle {
			left += "  S"
		}
		if left != "" {
			segments = append(segments, left)
		}
	}

	// Log path and optional first-run config notice (always shown).
	logPart := fmt.Sprintf("log: %s", s.logPath)
	if s.configCreated {
		logPart += fmt.Sprintf("  |  config created: %s", s.configPath)
	}
	segments = append(segments, logPart)

	// Transient right zone.
	if s.transientRight != "" {
		segments = append(segments, s.transientRight)
	}

	return strings.Join(segments, "  |  ")
}
