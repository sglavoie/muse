package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = "dev"

func main() {
	// Handle subcommands before flag parsing.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "toggle-play", "volume-up", "volume-down", "next", "previous":
			if err := runRemoteAction(os.Args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "muse %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			return
		}
	}

	version := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: muse [command] [flags]\n\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  toggle-play   Toggle play/pause on the running instance\n")
		fmt.Fprintf(os.Stderr, "  next          Skip to the next track\n")
		fmt.Fprintf(os.Stderr, "  previous      Skip to the previous track\n")
		fmt.Fprintf(os.Stderr, "  volume-up     Increase volume\n")
		fmt.Fprintf(os.Stderr, "  volume-down   Decrease volume\n")
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *version {
		fmt.Println(Version)
		os.Exit(0)
	}

	dirs, err := config.ResolveDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "muse: resolve dirs: %v\n", err)
		os.Exit(1)
	}

	// Init logging before loading config so any config warnings are captured.
	if err := config.InitLogging(dirs.State, slog.LevelInfo); err != nil {
		fmt.Fprintf(os.Stderr, "muse: init logging: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("log:", config.LogPath())
	slog.Info("log file", "path", config.LogPath())

	configPath := config.ConfigPath(dirs.Config)
	cfg, configCreated, err := config.Load(dirs.Config)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		fmt.Fprintf(os.Stderr, "muse: load config: %v\n", err)
		os.Exit(1)
	}

	// Apply the log level from config immediately so all subsequent log calls
	// respect the file-configured verbosity without re-truncating the log.
	logLevel, ok := config.ParseLogLevel(cfg.LogLevel)
	if !ok {
		slog.Warn("unrecognized log level, defaulting to info", "value", cfg.LogLevel)
	}
	config.SetLogLevel(logLevel)

	if err := config.ValidateMusicDir(cfg.MusicDirectory, configPath); err != nil {
		slog.Error("invalid music directory", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	player.SetSoundContext(ctx)

	// mgr is set inside LaunchMPV (app.Run step 6), which runs after the PID
	// lockfile is acquired (step 1) and the stale socket is removed (step 2).
	// This ordering ensures that "another instance is running" is surfaced
	// before any socket liveness check, giving users the correct error message.
	var mgr *player.Manager

	deps := app.Deps{
		Config: cfg,
		LaunchMPV: func(ctx context.Context, socketPath string) error {
			var err error
			mgr, err = player.NewManager()
			if err != nil {
				return fmt.Errorf("player: %w", err)
			}
			mgr.SetSocketPath(socketPath)
			mgr.SetExtraFlags(cfg.MPVExtraFlags)
			if err := mgr.Start(); err != nil {
				return err
			}
			// Apply the configured default volume. SetVolumeAbsolute enqueues
			// the command for replay once the IPC socket connects, so calling
			// it here (before the socket is ready) is safe.
			_ = mgr.SetVolumeAbsolute(float64(cfg.DefaultVolume))
			return nil
		},
		RunScan: func(ctx context.Context, database *db.DB) error {
			return nil
		},
		RestorePlayer: func(path string, positionMs int64) error {
			return mgr.LoadFilePaused(path, positionMs)
		},
	}

	result, err := app.Run(ctx, dirs, deps)
	if err != nil {
		slog.Error("startup failed", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}

	shutdownDeps := app.ShutdownDeps{
		StopPlayback: func() error {
			if mgr != nil {
				return mgr.Close()
			}
			return nil
		},
		CancelDBWriter: result.CancelWriter,
		WaitDBWriter:   result.Writer.Wait,
		CloseDB: func() {
			if err := result.DB.Close(); err != nil {
				slog.Warn("shutdown: close database", "err", err)
			}
		},
		Lockfile:   result.Lockfile,
		SocketPath: result.SocketPath,
		CloseLog:   config.CloseLog,
	}

	extractor := library.ExifToolExtractor{}

	const upsertBatchSize = 500
	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, cfg.MusicDirectory, extractor, upsertBatchSize)
	if err != nil {
		slog.Error("load library failed", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		app.Shutdown(shutdownDeps)
		os.Exit(1)
	}

	adapter := db.NewPlaylistAdapter(result.DB, result.Writer)
	model := tui.New(ctx,
		tui.WithConfigPath(configPath, configCreated),
		tui.WithScanChannel(libResult.TuiScanCh),
		tui.WithWatchChannel(libResult.TuiWatchCh),
		tui.WithLibraryRoot(cfg.MusicDirectory),
		tui.WithExtractor(extractor),
		tui.WithPlayer(mgr),
		tui.WithWatchDegraded(libResult.WatchDegraded),
		tui.WithRuntimeDir(dirs.Runtime),
		tui.WithQueueWriter(result.Writer),
		tui.WithQueueDequeuer(result.Writer),
		tui.WithSessionWriter(result.Writer),
		tui.WithPlaylistTrackReader(result.DB),
		tui.WithPlaylistWriter(adapter),
		tui.WithPlaylistRenamer(adapter),
		tui.WithPlaylistDeleter(result.Writer),
		tui.WithPlaylistAdder(result.Writer),
		tui.WithQueueReorderer(result.Writer),
		tui.WithBatchEnqueuer(result.Writer),
		tui.WithMetadataWriter(result.Writer),
		tui.WithFavoriteReader(result.DB),
		tui.WithFavoriteWriter(result.Writer),
	)

	// Apply all restored session state (queue, library, playlists,
	// repeat/shuffle/volume, current track) before tea.NewProgram so the
	// program captures the fully-initialised model at construction time.
	app.ApplyRestoredSession(
		&model, mgr, result.Restored, result.Writer, result.DB,
		libResult.Tracks, libResult.DeletedPaths, libResult.HashByPath,
		cfg.DefaultVolume,
	)

	p := tea.NewProgram(model)

	// Wire player callbacks after p is created so events reach the TUI via Send.
	mgr.SetMissingFileFunc(func(path string) {
		p.Send(tui.MissingTrackMsg{Path: path})
	})
	mgr.SetStatusFunc(func(s string) {
		p.Send(tui.PlayerStatusMsg{Text: s})
	})
	mgr.SetPlaybackEndedFunc(func() {
		p.Send(tui.PlaybackEndedMsg{})
	})
	mgr.SetTrackChangedFunc(func(hash, artist, title, source string) {
		p.Send(tui.TrackChangedMsg{Hash: hash, Artist: artist, Title: title, PlaylistSource: source})
	})
	mgr.SetPauseChangedFunc(func(paused bool) {
		p.Send(tui.PauseChangedMsg{Paused: paused})
	})
	mgr.SetMissingMarker(result.Writer)

	// Listen for SIGUSR1 to dispatch remote playback actions. The action name
	// is read from $RUNTIME/muse.cmd (written by runRemoteAction); a missing
	// file (bare kill -USR1) defaults to play/pause for backward compat.
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGUSR1)
	go func() {
		cmdPath := filepath.Join(dirs.Runtime, "muse.cmd")
		for range sigusr1 {
			actionName := ""
			if data, err := os.ReadFile(cmdPath); err == nil {
				actionName = strings.TrimSpace(string(data))
				os.Remove(cmdPath)
			}
			p.Send(tui.RemoteActionMsg{ActionName: actionName})
		}
	}()

	// When the signal context is cancelled (SIGTERM/SIGINT from outside),
	// tell the TUI to quit so p.Run() returns and shutdown proceeds.
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		slog.Error("TUI error", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		// Still run shutdown to clean up resources.
		app.Shutdown(shutdownDeps)
		os.Exit(1)
	}

	app.Shutdown(shutdownDeps)
}

// runRemoteAction writes the action name to $RUNTIME/muse.cmd and sends
// SIGUSR1 to the running muse process. The TUI's signal handler reads the
// file and dispatches the action through handlePlaybackKeyAction, keeping
// all playback logic (including Go-side queue indexing and TUI state sync)
// in one place.
func runRemoteAction(action string) error {
	runtimeDir := config.ResolveRuntimeDir()
	cmdPath := filepath.Join(runtimeDir, "muse.cmd")
	pidPath := filepath.Join(runtimeDir, "muse.pid")

	// Atomic write: write to a temp file then rename.
	tmpPath := cmdPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(action+"\n"), 0o600); err != nil {
		return fmt.Errorf("write command file: %w", err)
	}
	if err := os.Rename(tmpPath, cmdPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename command file: %w", err)
	}

	// Signal muse TUI to read the command file and dispatch the action.
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("parse PID: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if err := proc.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("send SIGUSR1: %w", err)
	}
	return nil
}
