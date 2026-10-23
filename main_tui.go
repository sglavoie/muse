package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/control"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/notify"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// runTUI resolves dirs, initialises logging, loads config, and runs the full
// TUI music player until it exits or ctx is cancelled.
func runTUI(ctx context.Context, opts tuiOptions) error {
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

	cfgResult, musicDirHint, musicDirsPartial := checkMusicDirs(cfgResult, cfg.MusicDirectories, configPath)

	player.SetSoundContext(ctx)

	// mgr is set inside LaunchMPV (app.Run step 6), which runs after the PID
	// lockfile is acquired (step 1) and the stale socket is removed (step 2).
	// This ordering ensures that "another instance is running" is surfaced
	// before any socket liveness check, giving users the correct error message.
	var mgr *player.Manager

	deps := app.Deps{
		Config:       cfg,
		CheckPrereqs: checkPrereqsFor(opts),
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

	// Prerequisite warnings (a degraded metadata backend, an unavailable
	// notification backend) describe behaviour the user will notice, so they
	// join the config-load warnings on the one channel the TUI already renders.
	// This is the only merge point: buildModel below receives the combined
	// slice, so nothing downstream appends them a second time.
	cfgResult = mergeStartupWarnings(cfgResult, result.PrereqWarnings)

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

	ctrlServer, err := listenControl(opts, &shutdownDeps)
	if err != nil {
		slog.Error("startup: control socket", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}

	startupCount, err := result.DB.IncrementStartupCount()
	if err != nil {
		slog.Warn("startup: increment startup count", "err", err)
	}

	// Prerequisites have already been checked by app.Run, so a usable backend
	// exists here; SelectExtractor repeats exactly the same two bounded probes
	// only to construct the matching extractor, so this cannot introduce a
	// verdict the prereq check did not already reach, nor hang on a wedged
	// binary even though ctx has no deadline. This one construction site feeds
	// both the initial scan below and the TUI's manual-rescan path via
	// buildModel → tui.WithExtractor.
	extractor, backend, err := library.SelectExtractor(ctx)
	if err != nil {
		slog.Error("no metadata backend available", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}
	slog.Info("metadata backend selected", "backend", backend)

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
		// The audit outcome is deliberately not consumed here: the TUI drains
		// auditCh itself and muse has no mid-session surface for a degraded
		// background stage, so a truncated audit is reported through the log by
		// RunDurationAudit. The handle exists so the stage can no longer pass a
		// panic off as completion to a caller that does need to tell them apart.
		auditCh, _ = app.RunDurationAudit(ctx, result.DB, result.Writer)
	}

	model := buildModel(ctx, cfg, configPath, cfgResult, musicDirHint, musicDirsPartial, result, libResult, dirs, extractor, adapter, mgr, auditCh, startupCount, ctrlServer)

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
		OnStatus:        func(s string) { p.Send(playback.PlayerStatusMsg{Text: s}) },
		OnMissingFile:   func(path string) { p.Send(playback.MissingTrackMsg{Path: path}) },
		OnPlaybackEnded: func() { p.Send(playback.PlaybackEndedMsg{}) },
		OnTrackChanged: func(hash, artist, title, source string, index int, queueID int64) {
			p.Send(playback.TrackChangedMsg{
				Hash: hash, Artist: artist, Title: title,
				PlaylistSource: source, Index: index, QueueID: queueID,
			})
		},
		OnPauseChanged: func(paused bool) { p.Send(playback.PauseChangedMsg{Paused: paused}) },
		OnDisconnected: func() { p.Send(playback.PlayerDisconnectedMsg{}) },
	})
	mgr.SetMissingMarker(result.Writer)

	// Answer the host only now that p exists to carry its requests. muse.ctrl
	// below keeps running alongside, so `muse next` still reaches this muse.
	if err := serveControl(ctrlServer, p.Send); err != nil {
		slog.Error("startup: serve control socket", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return err
	}

	// Listen for remote commands on a Unix domain socket.
	ctrlListener, err := startCommandListener(p, dirs.Runtime)
	if err != nil {
		slog.Warn("remote control unavailable", "err", err)
	}
	shutdownDeps.CtrlListener = ctrlListener

	// When the signal context is cancelled (SIGINT/SIGTERM/SIGHUP from
	// outside), tell the TUI to quit so p.Run() returns and shutdown proceeds.
	// BeginShutdown comes first: a SIGHUP reaches the whole process group, so
	// mpv may already be dying, and the restart supervisor must not race the
	// coming Close to relaunch it.
	go func() {
		<-ctx.Done()
		mgr.BeginShutdown()
		p.Quit()
	}()

	finalModel, err := p.Run()
	mgr.BeginShutdown()

	// Bubble Tea has exited, so a tea.Cmd would never run: persist the
	// in-flight listen here, ahead of the app.Shutdown calls that cancel the DB
	// writer. Writer.Exec is synchronous and there is no concurrent taker.
	if m, ok := finalModel.(tui.Model); ok {
		if err := m.SaveBrowsingSession(); err != nil {
			slog.Warn("quit: save browsing session failed", "err", err)
		}
		if rec, ok := m.TakePendingListen(); ok {
			if err := result.Writer.RecordListen(rec); err != nil {
				slog.Warn("quit: flush pending listen failed", "err", err)
			}
		}
	}

	// A signal can end p.Run with an error rather than through Quit: a SIGHUP
	// from a closed terminal also takes away the tty Bubble Tea reads. That is
	// still the clean exit the signal asked for, not a TUI failure, and the
	// terminal the session summary would go to may be gone.
	if endedBySignal(ctx, err) {
		slog.Info("TUI ended by shutdown signal", "err", err)
		app.Shutdown(shutdownDeps, app.ShutdownTimeout)
		return nil
	}

	if err != nil {
		reportProgramPanic(err, os.Stderr)
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

// buildModel wires all dependencies into a tui.Model. It is extracted from
// runTUI so that the surrounding setup and teardown code remains scannable.
func buildModel(
	ctx context.Context,
	cfg config.Config,
	configPath string,
	cfgResult config.LoadResult,
	musicDirHint string,
	musicDirsPartial bool,
	result *app.RunResult,
	libResult app.LibraryResult,
	dirs config.Dirs,
	extractor library.MetadataExtractor,
	adapter *db.PlaylistAdapter,
	mgr *player.Manager,
	auditCh <-chan library.DurationFix,
	startupCount int,
	ctrlServer *control.Server,
) tui.Model {
	opts := []tui.Option{
		tui.WithConfig(cfg),
		tui.WithConfigPath(configPath, cfgResult.Created),
		tui.WithConfigWarnings(cfgResult.Warnings),
		tui.WithMusicDirHint(musicDirHint, musicDirsPartial),
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
		tui.WithDJWriter(result.Writer),
		tui.WithDJQueueWriter(result.Writer),
		tui.WithDJRecipeReader(result.DB),
		tui.WithDJRecipeWriter(result.Writer),
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
		tui.WithTrackNotifier(notifierFor(ctrlServer)),
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
	}
	return tui.New(ctx, append(opts, controlModelOptions(ctrlServer)...)...)
}

// desktopNotifier adapts internal/notify to the TUI's trackNotifier port. It
// carries no state: notify.Send spawns the platform backend fire-and-forget.
type desktopNotifier struct{}

func (desktopNotifier) NotifyTrack(title, artist, album string) error {
	return notify.Send(notify.Track{Title: title, Artist: artist, Album: album})
}
