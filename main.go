package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	version := flag.Bool("version", false, "print version and exit")
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
	if lvl, err := parseLogLevel(cfg.LogLevel); err == nil {
		config.SetLogLevel(lvl)
	}

	if err := config.ValidateMusicDir(cfg.MusicDirectory, configPath); err != nil {
		slog.Error("invalid music directory", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mgr, err := player.NewManager()
	if err != nil {
		slog.Error("player: create manager", "err", err)
		fmt.Fprintf(os.Stderr, "muse: player: %v\n", err)
		os.Exit(1)
	}

	deps := app.Deps{
		Config: cfg,
		LaunchMPV: func(ctx context.Context, socketPath string) error {
			mgr.SetSocketPath(socketPath)
			return mgr.Start()
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
	scanCh := library.Scan(cfg.MusicDirectory, extractor)
	watchCh, _ := library.Watch(ctx, cfg.MusicDirectory, extractor, library.DefaultDebounce)

	restored := result.Restored

	model := tui.New(config.LogPath(), configPath, configCreated, scanCh, watchCh, cfg.MusicDirectory, extractor, mgr)
	model.SetQueueWriter(result.Writer)
	model.SetQueueDequeuer(result.Writer)
	model.SetSessionWriter(result.Writer)

	// setQueue keeps the player Manager and the TUI QueueView in sync.
	// Callers MUST use this helper whenever the queue changes so that
	// PlayQueueIndexMsg.Index addresses the same track in both sides.
	setQueue := func(pe []player.QueueTrack, te []tui.QueueEntry) {
		mgr.SetQueue(pe)
		model.SetQueueEntries(te)
	}
	setQueue(restored.PlayerTracks, restored.QueueEntries)

	// Apply restored session state to the model so the first render reflects
	// the correct repeat/shuffle/playback state from the previous session.
	// These Set* calls MUST happen before tea.NewProgram so the program
	// captures the fully-initialised model at construction time.
	model.SetRepeatMode(restored.RepeatMode)
	model.SetShuffle(restored.Shuffle)
	// Propagate restored repeat/shuffle modes into mpv so its behaviour
	// matches the status bar indicators from the first user interaction.
	_ = mgr.SetRepeat(restored.RepeatMode.String())
	_ = mgr.SetShuffle(restored.Shuffle)
	if len(restored.QueueEntries) > 0 {
		model.SetPlaybackState(tui.PlaybackPaused)
	}
	if restored.CurrentHash != "" {
		model.SetCurrentTrackHash(restored.CurrentHash)
	}

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
	mgr.SetTrackChangedFunc(func(hash, artist, title string) {
		p.Send(tui.TrackChangedMsg{Hash: hash, Artist: artist, Title: title, PlaylistSource: "Queue"})
	})
	mgr.SetMissingMarker(result.Writer)

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

// parseLogLevel maps a log_level string from config to a slog.Level.
func parseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}
