package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
)

// alerterPath is the preferred absolute location of the alerter binary.
// Declared as a var so tests can point it at a stub.
var alerterPath = "/opt/homebrew/bin/alerter"

const (
	alerterGroup   = "com.sglavoie.muse.now-playing"
	alerterSender  = "com.apple.Terminal"
	alerterTimeout = "5"

	// mpvSocketName and dbFileName mirror the unexported constants used by
	// internal/app during startup.
	mpvSocketName = "mpv.sock"
	dbFileName    = "muse.db"

	unknownArtist = "Unknown artist"
)

// nowPlaying is the resolved notification content for a single fn+F6 press.
type nowPlaying struct {
	Title    string
	Artist   string
	Album    string
	Paused   bool
	Degraded string // non-empty when metadata could not be read from the library
}

// runNowPlaying reads the currently playing track straight from mpv's IPC
// socket, enriches it from the library database, and posts a macOS
// notification. It deliberately bypasses the muse.ctrl control socket, whose
// protocol is fire-and-forget and has no response path, so it works even when
// the TUI is busy.
func runNowPlaying() int {
	dirs, err := config.ResolveDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve dirs: %v\n", err)
		return 1
	}

	loadResult, err := config.Load(dirs.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		return 1
	}

	socketPath := resolveMPVSocketPath(dirs, loadResult.Config)
	trackPath, paused, ok, err := queryMPVState(socketPath)
	switch {
	case errors.Is(err, player.ErrPlaybackFailed):
		// mpv is running but has nothing loaded.
		return emit(nowPlayingIdle(), 0)
	case err != nil:
		return emit(nowPlayingNotRunning(), 1)
	case !ok:
		return emit(nowPlayingIdle(), 0)
	}

	row, found := lookupTrack(filepath.Join(dirs.Data, dbFileName), trackPath)
	return emit(buildNowPlaying(trackPath, paused, row, found), 0)
}

// emit prints the summary line and fires the notification, returning exitCode
// unless the notification itself could not be delivered.
func emit(n nowPlaying, exitCode int) int {
	fmt.Println(n.summaryLine())
	if err := notify(n.alerterArgs()); err != nil {
		fmt.Fprintf(os.Stderr, "error: notify: %v\n", err)
		return 1
	}
	return exitCode
}

func nowPlayingIdle() nowPlaying {
	return nowPlaying{Title: "muse", Degraded: "Nothing playing"}
}

func nowPlayingNotRunning() nowPlaying {
	return nowPlaying{Title: "muse", Degraded: "muse is not running"}
}

// resolveMPVSocketPath mirrors the resolution performed by internal/app at
// startup: an explicit config value wins (config.Load already expanded the
// tilde), otherwise the socket lives under the runtime directory.
func resolveMPVSocketPath(dirs config.Dirs, cfg config.Config) string {
	if cfg.MPVSocketPath != "" {
		return cfg.MPVSocketPath
	}
	return filepath.Join(dirs.Runtime, mpvSocketName)
}

// queryMPVState reads the loaded file path and pause state from mpv. ok is
// false when mpv answered with a null path (idle). A dial failure means muse
// is not running; player.ErrPlaybackFailed means mpv is up but has no file.
func queryMPVState(socketPath string) (path string, paused, ok bool, err error) {
	client, err := player.NewIPCClient(socketPath)
	if err != nil {
		return "", false, false, err
	}
	defer client.Close()

	path, ok, err = client.GetPropertyString("path")
	if err != nil || !ok {
		return "", false, false, err
	}

	// A missing pause property is not worth failing over: default to playing.
	paused, _, err = client.GetPropertyBool("pause")
	if err != nil {
		return path, false, true, nil
	}
	return path, paused, true, nil
}

// lookupTrack reads library metadata for trackPath. Every failure degrades to
// "not found" rather than aborting: a notification with filename-derived
// metadata beats no notification at all.
func lookupTrack(dbPath, trackPath string) (db.TrackRow, bool) {
	database, err := db.OpenReadOnly(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: open database: %v\n", err)
		return db.TrackRow{}, false
	}
	defer database.Close()

	row, found, err := database.ReadTrackByPath(trackPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: read track: %v\n", err)
		return db.TrackRow{}, false
	}
	return row, found
}

// buildNowPlaying assembles the notification content, falling back to
// path-derived metadata when the track is absent from the library.
func buildNowPlaying(trackPath string, paused bool, row db.TrackRow, found bool) nowPlaying {
	n := nowPlaying{Paused: paused}
	if found {
		n.Title = row.Title
		n.Artist = row.Artist
		n.Album = row.Album
	}
	if n.Title == "" {
		title, artist := fallbackMetaFromPath(trackPath)
		n.Title = title
		if n.Artist == "" {
			n.Artist = artist
		}
		if !found {
			n.Degraded = "Not in library"
		}
	}
	if n.Artist == "" {
		n.Artist = unknownArtist
	}
	return n
}

// fallbackMetaFromPath derives a title and artist from a file path when the
// library has nothing: the filename without its extension, and the name of the
// containing directory.
func fallbackMetaFromPath(p string) (title, artist string) {
	base := filepath.Base(p)
	if base == "." || base == string(filepath.Separator) {
		base = ""
	}
	title = strings.TrimSuffix(base, filepath.Ext(base))
	if title == "" {
		title = "Unknown track"
	}
	parent := filepath.Base(filepath.Dir(p))
	if parent == "." || parent == string(filepath.Separator) {
		parent = ""
	}
	return title, parent
}

// message returns the notification body, which doubles as the state line.
func (n nowPlaying) message() string {
	body := n.Degraded
	if body == "" {
		body = n.Album
	}
	if body == "" {
		body = "Now playing"
	}
	if n.Paused {
		return "Paused — " + body
	}
	return body
}

// alerterArgs builds the argv passed to alerter. --group makes repeated
// presses replace the existing banner instead of stacking, and --timeout
// auto-dismisses it (alerter otherwise blocks waiting for interaction).
func (n nowPlaying) alerterArgs() []string {
	args := []string{"--title", n.Title}
	// An empty --subtitle renders as a blank line, so omit the flag entirely.
	if n.Artist != "" {
		args = append(args, "--subtitle", n.Artist)
	}
	return append(args,
		"--message", n.message(),
		"--group", alerterGroup,
		"--sender", alerterSender,
		"--timeout", alerterTimeout,
	)
}

// summaryLine is the single stdout line, so the command is also useful when
// run from a terminal.
func (n nowPlaying) summaryLine() string {
	parts := []string{n.Title}
	if n.Artist != "" {
		parts = append(parts, n.Artist)
	}
	return strings.Join(parts, " — ") + " (" + n.message() + ")"
}

// notify launches alerter fire-and-forget: Start then Release, never Wait, so
// Karabiner's shell_command returns immediately. Stdin/Stdout/Stderr are left
// nil, which wires them to /dev/null.
func notify(args []string) error {
	bin, err := resolveAlerter()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release %s: %w", bin, err)
	}
	return nil
}

// resolveAlerter prefers the hardcoded absolute path because Karabiner's
// shell_command runs with a minimal PATH, falling back to a PATH lookup.
func resolveAlerter() (string, error) {
	if _, err := os.Stat(alerterPath); err == nil {
		return alerterPath, nil
	}
	bin, err := exec.LookPath("alerter")
	if err != nil {
		return "", fmt.Errorf("alerter not found at %s or on PATH", alerterPath)
	}
	return bin, nil
}
