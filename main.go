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

// runCheck validates the muse configuration and prerequisites without
// launching the full TUI. It prints warnings and errors to stderr and returns
// 0 on success or 1 if any check fails.
func runCheck() int {
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

	for _, w := range loadResult.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}

	exitCode := 0

	configPath := config.ConfigPath(dirs.Config)
	if err := config.ValidateMusicDir(loadResult.Config.MusicDirectory, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		exitCode = 1
	}

	if err := config.CheckPrereqs(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		exitCode = 1
	}

	if exitCode == 0 {
		fmt.Println("ok")
	}
	return exitCode
}

// runTUI resolves dirs, initialises logging, loads config, and runs the full
// TUI music player until it exits or ctx is cancelled.
func runTUI(ctx context.Context) error {
	dirs, err := config.ResolveDirs()
	if err != nil {
		return fmt.Errorf("resolve dirs: %w", err)
	}

	// Init logging before loading config so any config warnings are captured.
	if err := config.InitLogging(dirs.State, slog.LevelInfo); err != nil {
		return fmt.Errorf("init logging: %w", err)
	}
	fmt.Println("log:", config.LogPath())
	slog.Info("log file", "path", config.LogPath())

	configPath := config.ConfigPath(dirs.Config)
	cfgResult, err := config.Load(dirs.Config)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		return fmt.Errorf("load config: %w", err)
	}
	cfg := cfgResult.Config

	// Apply the log level from config immediately so all subsequent log calls
	// respect the file-configured verbosity without re-truncating the log.
	logLevel, ok := config.ParseLogLevel(cfg.LogLevel)
	if !ok {
		slog.Warn("unrecognized log level, defaulting to info", "value", cfg.LogLevel)
	}
	config.SetLogLevel(logLevel)

	if err := config.ValidateMusicDir(cfg.MusicDirectory, configPath); err != nil {
		slog.Error("invalid music directory", "err", err)
		return fmt.Errorf("%w", err)
	}

	player.SetSoundContext(ctx)

	// mgr is set inside LaunchMPV (app.Run step 6), which runs after the PID
	// lockfile is acquired (step 1) and the stale socket is removed (step 2).
	// This ordering ensures that "another instance is running" is surfaced
	// before any socket liveness check, giving users the correct error message.
	var mgr *player.Manager

	deps := app.Deps{
		Config: cfg,
		LaunchMPV: func(ctx context.Context, socketPath string) error {
			mgr = player.NewManager(socketPath)
			mgr.SetExtraFlags(cfg.MPVExtraFlags)
			if err := mgr.Start(); err != nil {
				return err
			}
			// Apply the configured default volume. SetVolumeAbsolute enqueues
			// the command for replay once the IPC socket connects, so calling
			// it here (before the socket is ready) is safe.
			if err := mgr.SetVolumeAbsolute(float64(cfg.DefaultVolume)); err != nil {
				slog.Warn("startup: set default volume", "err", err)
			}
			if cfg.NormalizeVolume {
				if err := mgr.SetNormalize(true); err != nil {
					slog.Warn("startup: set normalize", "err", err)
				}
			}
			if err := mgr.SetGapless(cfg.GaplessAudio); err != nil {
				slog.Warn("startup: set gapless", "err", err)
			}
			return nil
		},
		RestorePlayer: func(path string, positionMs int64) error {
			return mgr.LoadFilePaused(path, positionMs)
		},
	}

	result, err := app.Run(ctx, dirs, deps)
	if err != nil {
		slog.Error("startup failed", "err", err)
		return err
	}

	shutdownDeps := app.ShutdownDeps{
		ClosePlayer: func() error {
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
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}

	adapter := db.NewPlaylistAdapter(result.DB, result.Writer)
	model := buildModel(ctx, cfg, configPath, cfgResult, result, libResult, dirs, extractor, adapter, mgr)

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
	mgr.SetEventCallbacks(player.EventCallbacks{
		OnStatus:        func(s string) { p.Send(tui.PlayerStatusMsg{Text: s}) },
		OnMissingFile:   func(path string) { p.Send(tui.MissingTrackMsg{Path: path}) },
		OnPlaybackEnded: func() { p.Send(tui.PlaybackEndedMsg{}) },
		OnTrackChanged: func(hash, artist, title, source string) {
			p.Send(tui.TrackChangedMsg{Hash: hash, Artist: artist, Title: title, PlaylistSource: source})
		},
		OnPauseChanged: func(paused bool) { p.Send(tui.PauseChangedMsg{Paused: paused}) },
		OnDisconnected: func() { p.Send(tui.PlayerDisconnectedMsg{}) },
	})
	mgr.SetMissingMarker(result.Writer)

	// Listen for remote commands on a Unix domain socket.
	ctrlListener, err := startCommandListener(p, dirs.Runtime)
	if err != nil {
		slog.Warn("remote control unavailable", "err", err)
	}
	shutdownDeps.CtrlListener = ctrlListener

	// When the signal context is cancelled (SIGTERM/SIGINT from outside),
	// tell the TUI to quit so p.Run() returns and shutdown proceeds.
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		slog.Error("TUI error", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}

	app.Shutdown(shutdownDeps, app.ShutdownTimeout)
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: muse [command] [flags]\n\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  %-14s %s\n", "check", "Validate config and prerequisites, then exit")
		for _, a := range remoteActions {
			fmt.Fprintf(os.Stderr, "  %-14s %s\n", a.Name, a.Desc)
		}
		fmt.Fprintf(os.Stderr, "\nFlags:\n")
		flag.PrintDefaults()
	}

	// Handle flags that should work regardless of position.
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--version", "-version":
			fmt.Println(Version)
			return
		case "--help", "-h":
			flag.Usage()
			return
		}
	}

	// Handle subcommands before flag parsing.
	if len(os.Args) > 1 && os.Args[1] == "check" {
		os.Exit(runCheck())
	}

	if len(os.Args) > 1 {
		if isKnownAction(os.Args[1]) {
			if err := runRemoteAction(os.Args[1]); err != nil {
				fmt.Fprintf(os.Stderr, "muse %s: %v\n", os.Args[1], err)
				os.Exit(1)
			}
			return
		}
	}

	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "muse: unknown command %q\n", flag.Arg(0))
		flag.Usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	if err := runTUI(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}
	cancel()
}

// buildModel wires all dependencies into a tui.Model. It is extracted from
// runTUI so that the surrounding setup and teardown code remains scannable.
func buildModel(
	ctx context.Context,
	cfg config.Config,
	configPath string,
	cfgResult config.LoadResult,
	result *app.RunResult,
	libResult app.LibraryResult,
	dirs config.Dirs,
	extractor library.MetadataExtractor,
	adapter *db.PlaylistAdapter,
	mgr *player.Manager,
) tui.Model {
	return tui.New(ctx,
		tui.WithConfig(cfg),
		tui.WithConfigPath(configPath, cfgResult.Created),
		tui.WithConfigWarnings(cfgResult.Warnings),
		tui.WithScanChannel(libResult.TuiScanCh),
		tui.WithScanErrChannel(libResult.TuiScanErrCh),
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
}
