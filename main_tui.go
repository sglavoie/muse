package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

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

	if err := config.ValidateMusicDirs(cfg.MusicDirectories, configPath); err != nil {
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
			if cfg.EQEnabled {
				if err := mgr.SetEqualizer(true, cfg.EQBands); err != nil {
					slog.Warn("startup: set equalizer", "err", err)
				}
			}
			if cfg.StereoWideEnabled {
				if err := mgr.SetStereoWide(true, cfg.StereoWideAmount); err != nil {
					slog.Warn("startup: set stereo wide", "err", err)
				}
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
		if mgr != nil {
			if closeErr := mgr.Close(); closeErr != nil {
				slog.Warn("startup: close player after failure", "err", closeErr)
			}
		}
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

	startupCount, err := result.DB.IncrementStartupCount()
	if err != nil {
		slog.Warn("startup: increment startup count", "err", err)
	}

	extractor := library.ExifToolExtractor{}

	libResult, err := app.LoadLibrary(ctx, result.DB, result.Writer, cfg.MusicDirectories, cfg.ExcludedPaths, extractor, upsertBatchSize)
	if err != nil {
		slog.Error("load library failed", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}
	// Stop and flush the asynchronous library pipeline before the DB writer is
	// drained so its final watcher/scan upserts land durably. StopAndWait is
	// idempotent, so wiring it here is safe for both the normal-quit and
	// signal-driven shutdown paths.
	shutdownDeps.StopLibraryPipeline = libResult.StopAndWait

	adapter := db.NewPlaylistAdapter(result.DB, result.Writer)

	var auditCh <-chan library.DurationFix
	if library.FFProbeAvailable() {
		auditCh = app.RunDurationAudit(ctx, result.DB, result.Writer)
	}

	model := buildModel(ctx, cfg, configPath, cfgResult, result, libResult, dirs, extractor, adapter, mgr, auditCh, startupCount)

	// Apply all restored session state (queue, library, playlists,
	// repeat/shuffle/volume, current track) before tea.NewProgram so the
	// program captures the fully-initialised model at construction time.
	app.ApplyRestoredSession(
		&model, mgr, result.Restored, result.DB,
		libResult.Tracks, cfg.DefaultVolume,
	)

	p := tea.NewProgram(model)

	// Wire player callbacks after p is created so events reach the TUI via Send.
	mgr.SetEventCallbacks(player.EventCallbacks{
		OnStatus:        func(s string) { p.Send(tui.PlayerStatusMsg{Text: s}) },
		OnMissingFile:   func(path string) { p.Send(tui.MissingTrackMsg{Path: path}) },
		OnPlaybackEnded: func() { p.Send(tui.PlaybackEndedMsg{}) },
		OnTrackChanged: func(hash, artist, title, source string, index int) {
			p.Send(tui.TrackChangedMsg{Hash: hash, Artist: artist, Title: title, PlaylistSource: source, Index: index})
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

	finalModel, err := p.Run()

	// Bubble Tea has exited, so a tea.Cmd would never run: persist the
	// in-flight listen here, ahead of the app.Shutdown calls that cancel the DB
	// writer. Writer.Exec is synchronous and there is no concurrent taker.
	if m, ok := finalModel.(tui.Model); ok {
		if rec, ok := m.TakePendingListen(); ok {
			if err := result.Writer.RecordListen(rec); err != nil {
				slog.Warn("quit: flush pending listen failed", "err", err)
			}
		}
	}

	if err != nil {
		slog.Error("TUI error", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}

	if m, ok := finalModel.(tui.Model); ok && m.SessionTracksPlayed() > 0 {
		count := m.SessionTracksPlayed()
		secs := m.SessionListenSecs()
		word := "tracks"
		if count == 1 {
			word = "track"
		}
		total := int(secs)
		h := total / 3600
		mn := (total % 3600) / 60
		var formatted string
		if h > 0 {
			formatted = fmt.Sprintf("%dh %02dm", h, mn)
		} else {
			formatted = fmt.Sprintf("%dm", mn)
		}
		topArtist := m.SessionTopArtist()
		if topArtist != "" {
			fmt.Fprintf(os.Stderr, "\nSession: %d %s · %s · top artist: %s\n", count, word, formatted, topArtist)
		} else {
			fmt.Fprintf(os.Stderr, "\nSession: %d %s · %s\n", count, word, formatted)
		}
	}

	app.Shutdown(shutdownDeps, app.ShutdownTimeout)
	return nil
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
	auditCh <-chan library.DurationFix,
	startupCount int,
) tui.Model {
	return tui.New(ctx,
		tui.WithConfig(cfg),
		tui.WithConfigPath(configPath, cfgResult.Created),
		tui.WithConfigWarnings(cfgResult.Warnings),
		tui.WithScanChannel(libResult.TuiScanCh),
		tui.WithScanErrChannel(libResult.TuiScanErrCh),
		tui.WithWatchChannel(libResult.TuiWatchCh),
		tui.WithWatchStatusChannel(libResult.TuiWatchStatusCh),
		tui.WithLibraryRoots(cfg.MusicDirectories),
		tui.WithExtractor(extractor),
		tui.WithPlayer(mgr),
		tui.WithRuntimeDir(dirs.Runtime),
		tui.WithDataDir(dirs.Data),
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
		tui.WithDurationWriter(result.Writer),
		tui.WithTrackPositionWriter(result.Writer),
		tui.WithFavoriteReader(result.DB),
		tui.WithFavoriteWriter(result.Writer),
		tui.WithSmartPlaylistEvaluator(adapter),
		tui.WithSmartPlaylistWriter(adapter),
		tui.WithPlayCountWriter(result.Writer),
		tui.WithHistoryRecorder(result.Writer),
		tui.WithHistoryReader(result.DB),
		tui.WithInsightsReader(result.DB),
		tui.WithRecommendationReader(result.DB),
		tui.WithDismissalReader(result.DB),
		tui.WithDismissalWriter(result.Writer),
		tui.WithCacheReader(result.DB),
		tui.WithCacheRebuilder(result.Writer),
		tui.WithScanWriter(result.Writer, upsertBatchSize),
		tui.WithUserEditedReader(result.DB),
		tui.WithDurationAuditChannel(auditCh),
		tui.WithTagReader(result.DB),
		tui.WithTagWriter(result.Writer),
		tui.WithSessionCount(startupCount),
		tui.WithFreshLibrary(libResult.FreshLibrary),
	)
}
