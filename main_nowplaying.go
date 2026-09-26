package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/notify"
	"github.com/sglavoie/muse/internal/player"
)

const (
	// mpvSocketName and dbFileName mirror the unexported constants used by
	// internal/app during startup.
	mpvSocketName = "mpv.sock"
	dbFileName    = "muse.db"

	unknownArtist = "Unknown artist"
)

// sendNotification delivers the banner; tests replace it so they never post a
// real desktop notification.
var sendNotification = notify.Send

// runNowPlaying reads the currently playing track straight from mpv's IPC
// socket, enriches it from the library database, and posts a desktop
// notification. It deliberately bypasses the muse.ctrl control socket, whose
// protocol only acknowledges a command with "ok" and cannot return data, and
// which the TUI must be free to answer; reading mpv directly works even when
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
		// The banner stays generic, but the cause (a stale socket, a
		// permission problem, a wrong mpv_socket_path) goes to stderr.
		fmt.Fprintf(os.Stderr, "error: query mpv: %v\n", err)
		return emit(nowPlayingNotRunning(), 1)
	case !ok:
		return emit(nowPlayingIdle(), 0)
	}

	row, found := lookupTrack(filepath.Join(dirs.Data, dbFileName), trackPath)
	return emit(buildNowPlaying(trackPath, paused, row, found), 0)
}

// emit prints the summary line and fires the notification, returning exitCode
// unless the notification itself could not be delivered.
func emit(n notify.Track, exitCode int) int {
	fmt.Println(n.SummaryLine())
	if err := sendNotification(n); err != nil {
		fmt.Fprintf(os.Stderr, "error: notify: %v\n", err)
		return 1
	}
	return exitCode
}

func nowPlayingIdle() notify.Track {
	return notify.Track{Title: "muse", Degraded: "Nothing playing"}
}

func nowPlayingNotRunning() notify.Track {
	return notify.Track{Title: "muse", Degraded: "muse is not running"}
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
func buildNowPlaying(trackPath string, paused bool, row db.TrackRow, found bool) notify.Track {
	n := notify.Track{Paused: paused}
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
