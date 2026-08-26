package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/player"
)

func TestBuildNowPlaying(t *testing.T) {
	inLibrary := db.TrackRow{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}

	tests := []struct {
		name         string
		trackPath    string
		paused       bool
		row          db.TrackRow
		found        bool
		wantTitle    string
		wantArtist   string
		wantMessage  string
		wantDegraded string
	}{
		{
			name:        "playing in library",
			trackPath:   "/music/Aurora/sunrise.flac",
			row:         inLibrary,
			found:       true,
			wantTitle:   "Sunrise",
			wantArtist:  "Aurora",
			wantMessage: "Dawn",
		},
		{
			name:        "paused in library",
			trackPath:   "/music/Aurora/sunrise.flac",
			paused:      true,
			row:         inLibrary,
			found:       true,
			wantTitle:   "Sunrise",
			wantArtist:  "Aurora",
			wantMessage: "Paused — Dawn",
		},
		{
			name:        "in library without album",
			trackPath:   "/music/Aurora/sunrise.flac",
			row:         db.TrackRow{Title: "Sunrise", Artist: "Aurora"},
			found:       true,
			wantTitle:   "Sunrise",
			wantArtist:  "Aurora",
			wantMessage: "Now playing",
		},
		{
			name:        "in library without artist",
			trackPath:   "/music/Aurora/sunrise.flac",
			row:         db.TrackRow{Title: "Sunrise", Album: "Dawn"},
			found:       true,
			wantTitle:   "Sunrise",
			wantArtist:  unknownArtist,
			wantMessage: "Dawn",
		},
		{
			name:         "not in library",
			trackPath:    "/music/Aurora/sunrise.flac",
			wantTitle:    "sunrise",
			wantArtist:   "Aurora",
			wantMessage:  "Not in library",
			wantDegraded: "Not in library",
		},
		{
			name:         "not in library paused",
			trackPath:    "/music/Aurora/sunrise.flac",
			paused:       true,
			wantTitle:    "sunrise",
			wantArtist:   "Aurora",
			wantMessage:  "Paused — Not in library",
			wantDegraded: "Not in library",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildNowPlaying(tt.trackPath, tt.paused, tt.row, tt.found)
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Artist != tt.wantArtist {
				t.Errorf("Artist = %q, want %q", got.Artist, tt.wantArtist)
			}
			if got.message() != tt.wantMessage {
				t.Errorf("message() = %q, want %q", got.message(), tt.wantMessage)
			}
			if got.Degraded != tt.wantDegraded {
				t.Errorf("Degraded = %q, want %q", got.Degraded, tt.wantDegraded)
			}
			if got.Paused != tt.paused {
				t.Errorf("Paused = %v, want %v", got.Paused, tt.paused)
			}
		})
	}
}

func TestNowPlayingIdleAndNotRunning(t *testing.T) {
	idle := nowPlayingIdle()
	if idle.Title != "muse" || idle.Artist != "" || idle.message() != "Nothing playing" {
		t.Fatalf("idle = %+v, want title muse / no artist / Nothing playing", idle)
	}

	down := nowPlayingNotRunning()
	if down.Title != "muse" || down.Artist != "" || down.message() != "muse is not running" {
		t.Fatalf("notRunning = %+v, want title muse / no artist / muse is not running", down)
	}
}

func TestAlerterArgs(t *testing.T) {
	args := buildNowPlaying("/music/Aurora/sunrise.flac", false,
		db.TrackRow{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}, true).alerterArgs()

	wantPairs := map[string]string{
		"--title":    "Sunrise",
		"--subtitle": "Aurora",
		"--message":  "Dawn",
		"--group":    alerterGroup,
		"--sender":   alerterSender,
		"--timeout":  alerterTimeout,
	}
	for flag, want := range wantPairs {
		got, ok := argValue(args, flag)
		if !ok {
			t.Errorf("%s missing from %v", flag, args)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
}

func TestAlerterArgsOmitsBlankSubtitle(t *testing.T) {
	// Only the mpv-idle / not-running states carry a blank artist.
	args := nowPlayingIdle().alerterArgs()

	if slices.Contains(args, "--subtitle") {
		t.Errorf("--subtitle present for a blank artist: %v", args)
	}
	if got, _ := argValue(args, "--message"); got != "Nothing playing" {
		t.Errorf("--message = %q, want %q", got, "Nothing playing")
	}
}

func TestAlerterArgsMessageNeverEmpty(t *testing.T) {
	states := []nowPlaying{
		nowPlayingIdle(),
		nowPlayingNotRunning(),
		buildNowPlaying("/music/a.flac", false, db.TrackRow{}, false),
		buildNowPlaying("/music/a.flac", true, db.TrackRow{Title: "A"}, true),
		buildNowPlaying("", false, db.TrackRow{}, false),
	}

	for _, n := range states {
		args := n.alerterArgs()
		got, ok := argValue(args, "--message")
		if !ok || got == "" {
			t.Errorf("empty --message for %+v (args %v)", n, args)
		}
		if title, ok := argValue(args, "--title"); !ok || title == "" {
			t.Errorf("empty --title for %+v (args %v)", n, args)
		}
	}
}

// argValue returns the value following flag in an argv slice.
func argValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func TestFallbackMetaFromPath(t *testing.T) {
	tests := []struct {
		path       string
		wantTitle  string
		wantArtist string
	}{
		{"/music/Aurora/sunrise.flac", "sunrise", "Aurora"},
		{"/music/Aurora/Dawn/02 - Sunrise.mp3", "02 - Sunrise", "Dawn"},
		{"sunrise.flac", "sunrise", ""},
		{"/sunrise.flac", "sunrise", ""},
		{"/music/Aurora/noext", "noext", "Aurora"},
		{"", "Unknown track", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			title, artist := fallbackMetaFromPath(tt.path)
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if artist != tt.wantArtist {
				t.Errorf("artist = %q, want %q", artist, tt.wantArtist)
			}
		})
	}
}

func TestSummaryLine(t *testing.T) {
	n := buildNowPlaying("/music/Aurora/sunrise.flac", true,
		db.TrackRow{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}, true)

	want := "Sunrise — Aurora (Paused — Dawn)"
	if got := n.summaryLine(); got != want {
		t.Fatalf("summaryLine() = %q, want %q", got, want)
	}

	if got := nowPlayingIdle().summaryLine(); got != "muse (Nothing playing)" {
		t.Fatalf("idle summaryLine() = %q, want %q", got, "muse (Nothing playing)")
	}
}

// fakeMPV starts a Unix socket that answers get_property requests from props.
// A property mapped to nil is reported as unavailable.
func fakeMPV(t *testing.T, props map[string]any) string {
	t.Helper()

	socketPath := filepath.Join(shortTempDir(t), "mpv.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeMPV(conn, props)
		}
	}()
	return socketPath
}

func serveFakeMPV(conn net.Conn, props map[string]any) {
	defer conn.Close()
	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var req struct {
			Command   []any  `json:"command"`
			RequestID uint64 `json:"request_id"`
		}
		if err := dec.Decode(&req); err != nil {
			return
		}

		resp := map[string]any{"error": "success", "request_id": req.RequestID}
		if len(req.Command) == 2 {
			name, _ := req.Command[1].(string)
			value, known := props[name]
			switch {
			case !known:
				resp["error"] = "property unavailable"
			default:
				resp["data"] = value
			}
		}
		if err := enc.Encode(resp); err != nil {
			return
		}
	}
}

func TestQueryMPVStateNoSocketFile(t *testing.T) {
	socketPath := filepath.Join(shortTempDir(t), "absent.sock")

	_, _, ok, err := queryMPVState(socketPath)
	if err == nil {
		t.Fatal("queryMPVState on a missing socket succeeded, want error")
	}
	if errors.Is(err, player.ErrPlaybackFailed) {
		t.Fatalf("err = %v, want a dial error not ErrPlaybackFailed", err)
	}
	if ok {
		t.Error("ok = true, want false")
	}
}

func TestQueryMPVStateSocketWithoutListener(t *testing.T) {
	// A stale socket file left behind by a crashed instance: it exists on
	// disk but nothing is accepting on it.
	socketPath := filepath.Join(shortTempDir(t), "stale.sock")
	if err := os.WriteFile(socketPath, nil, 0o600); err != nil {
		t.Fatalf("write stale socket file: %v", err)
	}

	if _, _, _, err := queryMPVState(socketPath); err == nil {
		t.Fatal("queryMPVState on a stale socket succeeded, want error")
	}
}

func TestQueryMPVStatePlaying(t *testing.T) {
	socketPath := fakeMPV(t, map[string]any{
		"path":  "/music/Aurora/sunrise.flac",
		"pause": false,
	})

	path, paused, ok, err := queryMPVState(socketPath)
	if err != nil {
		t.Fatalf("queryMPVState: %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if path != "/music/Aurora/sunrise.flac" {
		t.Errorf("path = %q, want %q", path, "/music/Aurora/sunrise.flac")
	}
	if paused {
		t.Error("paused = true, want false")
	}
}

func TestQueryMPVStatePaused(t *testing.T) {
	socketPath := fakeMPV(t, map[string]any{
		"path":  "/music/Aurora/sunrise.flac",
		"pause": true,
	})

	_, paused, ok, err := queryMPVState(socketPath)
	if err != nil || !ok {
		t.Fatalf("queryMPVState: ok=%v err=%v", ok, err)
	}
	if !paused {
		t.Error("paused = false, want true")
	}
}

func TestQueryMPVStatePropertyUnavailable(t *testing.T) {
	// mpv is up but idle: it reports "property unavailable" for path.
	socketPath := fakeMPV(t, map[string]any{})

	_, _, ok, err := queryMPVState(socketPath)
	if !errors.Is(err, player.ErrPlaybackFailed) {
		t.Fatalf("err = %v, want ErrPlaybackFailed", err)
	}
	if ok {
		t.Error("ok = true, want false")
	}
}

func TestQueryMPVStateNullPath(t *testing.T) {
	socketPath := fakeMPV(t, map[string]any{"path": nil, "pause": false})

	path, _, ok, err := queryMPVState(socketPath)
	if err != nil {
		t.Fatalf("queryMPVState: %v", err)
	}
	if ok {
		t.Error("ok = true, want false for a null path")
	}
	if path != "" {
		t.Errorf("path = %q, want empty", path)
	}
}

func TestLookupTrack(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, dbFileName)

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Migrate(database.DB); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := database.Exec(
		`INSERT INTO tracks (hash, path, title, artist, album, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"h1", "/music/a.flac", "A", "Artist", "Album", 1, 1,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	row, found := lookupTrack(dbPath, "/music/a.flac")
	if !found {
		t.Fatal("found = false, want true")
	}
	if row.Title != "A" {
		t.Errorf("title = %q, want %q", row.Title, "A")
	}

	if _, found := lookupTrack(dbPath, "/music/nope.flac"); found {
		t.Error("found = true for an unknown path, want false")
	}

	// A missing database degrades instead of failing.
	if _, found := lookupTrack(filepath.Join(dir, "absent.db"), "/music/a.flac"); found {
		t.Error("found = true for a missing database, want false")
	}
}

// TestNotifyIsFireAndForget points alerterPath at a stub that records its argv
// and then sleeps. notify must return before the sleep finishes, proving it
// never calls Wait.
func TestNotifyIsFireAndForget(t *testing.T) {
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	stub := filepath.Join(dir, "alerter-stub")

	// The stub records its argv immediately, then sleeps far longer than the
	// deadline below, so a notify that waited on it could not return in time.
	// The record is written to a scratch file and renamed so the poll below
	// can never observe a partially flushed argv list.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvFile + ".tmp\nmv " + argvFile + ".tmp " + argvFile + "\nsleep 30\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	original := alerterPath
	alerterPath = stub
	t.Cleanup(func() { alerterPath = original })

	want := buildNowPlaying("/music/Aurora/sunrise.flac", false,
		db.TrackRow{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}, true).alerterArgs()

	start := time.Now()
	if err := notify(want); err != nil {
		t.Fatalf("notify: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("notify took %v, want it to return without waiting on the stub", elapsed)
	}

	// The stub writes its argv before sleeping. Poll generously: process spawn
	// is slow under -race with the whole suite running in parallel.
	var recorded string
	for range 500 {
		data, err := os.ReadFile(argvFile)
		if err == nil && len(data) > 0 {
			recorded = string(data)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recorded == "" {
		t.Fatalf("stub %s never recorded its argv to %s", stub, argvFile)
	}

	got := strings.Split(strings.TrimSuffix(recorded, "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Fatalf("stub argv = %q, want %q", got, want)
	}
}

func TestNotifyMissingBinary(t *testing.T) {
	original := alerterPath
	alerterPath = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { alerterPath = original })

	// Empty PATH so the LookPath fallback cannot resolve a real alerter.
	t.Setenv("PATH", "")

	if err := notify([]string{"--title", "muse"}); err == nil {
		t.Fatal("notify with no alerter available succeeded, want error")
	}
}

func TestResolveMPVSocketPath(t *testing.T) {
	dirs := config.Dirs{Runtime: "/run/muse"}

	if got := resolveMPVSocketPath(dirs, config.Config{}); got != filepath.Join("/run/muse", mpvSocketName) {
		t.Errorf("default socket path = %q, want %q", got, filepath.Join("/run/muse", mpvSocketName))
	}
	if got := resolveMPVSocketPath(dirs, config.Config{MPVSocketPath: "/tmp/custom.sock"}); got != "/tmp/custom.sock" {
		t.Errorf("configured socket path = %q, want %q", got, "/tmp/custom.sock")
	}
}
