package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/notify"
)

// reportProgramPanic leaves a durable record of a Bubble Tea panic. Bubble Tea
// recovers panics raised inside its own loop and in the goroutines it runs for
// tea.Cmd values: it restores the terminal, prints the stack trace to stderr,
// and makes p.Run() return an error wrapping tea.ErrProgramPanic. Nothing,
// however, reaches config.LogPath() — so a user who files a bug after the crash
// has a scrolled-away terminal and an empty log.
//
// This writes the crash to the slog file in the same key/value style as the rest
// of the startup path, and prints one short line to stderr naming that path,
// where it lands directly beneath Bubble Tea's own stack trace. Non-panic errors
// are left alone (runTUI logs them generically) and reported as false.
//
// Background goroutines outside Bubble Tea's protection are covered separately
// by internal/safego, which logs their panics with a stack of its own.
func reportProgramPanic(err error, stderr io.Writer) bool {
	if !errors.Is(err, tea.ErrProgramPanic) {
		return false
	}
	logPath := config.LogPath()
	slog.Error("TUI panicked: Bubble Tea recovered the panic, restored the terminal and printed the stack trace to stderr",
		"err", err, "log", logPath)
	// stderr is an io.Writer for testability, so errcheck no longer waives the
	// write the way it does for a direct os.Stderr call; a failed crash notice
	// is not worth acting on.
	_, _ = fmt.Fprintf(stderr, "\nmuse crashed. The stack trace above came from Bubble Tea; a record was written to %s\n", logPath)
	return true
}

// checkMusicDirs validates the configured music directories without letting a
// failure end the session. A first-run user with no ~/Music would otherwise get
// an error printed to a terminal they have already left; instead the TUI boots
// into an empty but usable library, carrying the reason on the existing
// config-warning channel and the fix in the welcome overlay. `muse check` stays
// strict — it is the explicit pre-flight for exactly this.
//
// It returns the load result with any warning appended, the hint text for the
// welcome overlay (empty when every directory validated), and whether the
// failure is partial — some directories are still usable, so the overlay must
// not claim that none were found.
func checkMusicDirs(cfgResult config.LoadResult, musicDirs []string, configPath string) (config.LoadResult, string, bool) {
	res := config.ValidateMusicDirsDetailed(musicDirs, configPath)
	if res.Err == nil {
		return cfgResult, "", false
	}
	partial := res.Degraded()
	slog.Error("invalid music directory", "err", res.Err, "usable_dirs", len(res.Usable))
	cfgResult.Warnings = append(cfgResult.Warnings, musicDirWarning(res.Err, partial))
	return cfgResult, config.ConfigHint(configPath), partial
}

// mergeStartupWarnings folds the prerequisite warnings returned by app.Run into
// the config-load warnings so both reach the TUI through tui.WithConfigWarnings,
// which is the only warning channel the model renders. Without this the ffprobe
// fallback is visible only in the log file and in `muse check`, so a user whose
// ExifTool is broken sees a slower scan with no explanation on screen.
//
// Warnings already present are skipped, so the status bar never repeats a line
// even if a warning reaches both channels. The result is a fresh slice: appending
// in place could otherwise write through a shared backing array.
func mergeStartupWarnings(cfgResult config.LoadResult, prereqWarnings []string) config.LoadResult {
	merged := slices.Clone(cfgResult.Warnings)
	for _, w := range prereqWarnings {
		if slices.Contains(merged, w) {
			continue
		}
		merged = append(merged, w)
	}
	cfgResult.Warnings = merged
	return cfgResult
}

// musicDirWarning collapses a ValidateMusicDirs error into a single line
// suitable for the TUI's config-warning notification, which joins warnings with
// "; " and renders them in the status bar. The full multi-line error (including
// the config hint) has already gone to the log, and the actionable instructions
// are shown separately in the welcome overlay.
//
// When only some directories failed, the line says the bad one is being skipped
// rather than implying the whole library is unusable.
func musicDirWarning(err error, partial bool) string {
	msg, _, _ := strings.Cut(err.Error(), "\n")
	prefix := "invalid music directory: "
	if partial {
		prefix = "skipping invalid music directory: "
	}
	return prefix + strings.TrimSpace(msg)
}

// suggestMusicDir looks for the platform music folder once at startup, so
// the first-run screen can offer it without probing the disk while drawing.
func suggestMusicDir() config.MusicDirSuggestion {
	s, _ := config.DetectMusicDirectory(config.OSMusicDirEnv())
	return s
}

// desktopNotifier adapts internal/notify to the TUI's trackNotifier port. It
// carries no state: notify.Send spawns the platform backend fire-and-forget.
type desktopNotifier struct{}

func (desktopNotifier) NotifyTrack(title, artist, album string) error {
	return notify.Send(notify.Track{Title: title, Artist: artist, Album: album})
}
