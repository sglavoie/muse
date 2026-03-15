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
	config.SetLogLevel(config.ParseLogLevel(cfg.LogLevel))

	if err := config.ValidateMusicDir(cfg.MusicDirectory, configPath); err != nil {
		slog.Error("invalid music directory", "err", err)
		fmt.Fprintf(os.Stderr, "muse: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

	// Bridge the scan pipeline to both the tracks table and the TUI.
	// rawScanCh carries individual Track values from the scanner goroutines.
	// tuiScanCh is a forwarding channel that the TUI reads via waitForNextBatch;
	// it is buffered so the bridge goroutine does not stall when the TUI is busy.
	// Tracks are accumulated into batches of upsertBatchSize before being
	// committed through the single-writer goroutine, ensuring FK constraints on
	// queue and playlist_tracks are satisfied for all subsequent DB operations.
	const upsertBatchSize = 500
	rawScanCh := library.Scan(cfg.MusicDirectory, extractor)
	tuiScanCh := make(chan library.Track, 256)
	go func() {
		defer close(tuiScanCh)
		var batch []library.Track
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := result.Writer.UpsertBatch(batch); err != nil {
				slog.Warn("scan: upsert batch", "err", err, "count", len(batch))
			}
			batch = batch[:0]
		}
		for t := range rawScanCh {
			tuiScanCh <- t
			batch = append(batch, t)
			if len(batch) >= upsertBatchSize {
				flush()
			}
		}
		flush()
	}()

	watchCh, _ := library.Watch(ctx, cfg.MusicDirectory, extractor, library.DefaultDebounce)

	restored := result.Restored

	model := tui.New(config.LogPath(), configPath, configCreated, tuiScanCh, watchCh, cfg.MusicDirectory, extractor, mgr)
	model.SetQueueWriter(result.Writer)
	model.SetQueueDequeuer(result.Writer)
	model.SetSessionWriter(result.Writer)

	adapter := db.NewPlaylistAdapter(result.DB, result.Writer)
	model.SetPlaylistTrackReader(result.DB)
	model.SetPlaylistWriter(adapter)
	model.SetPlaylistRenamer(adapter)
	model.SetPlaylistDeleter(result.Writer)
	model.SetPlaylistAdder(result.Writer)
	model.SetQueueReorderer(result.Writer)

	// setQueue keeps the player Manager and the TUI QueueView in sync.
	// IMPORTANT: this helper MUST be called any time the queue is mutated
	// outside the TUI message loop (e.g. session restore, undo clear). Calling
	// mgr.SetQueue or model.SetQueueEntries independently will cause the
	// player and TUI indices to diverge, breaking PlayQueueIndexMsg addressing.
	setQueue := func(pe []player.QueueTrack, te []tui.QueueEntry) {
		mgr.SetQueue(pe)
		model.SetQueueEntries(te)
	}
	setQueue(restored.PlayerTracks, restored.QueueEntries)

	if playlists, err := result.DB.ReadPlaylists(); err == nil {
		model.SetPlaylists(playlists)
	} else {
		slog.Warn("startup: read playlists", "err", err)
	}

	// Apply restored session state to the model so the first render reflects
	// the correct repeat/shuffle/playback state from the previous session.
	// These Set* calls MUST happen before tea.NewProgram so the program
	// captures the fully-initialised model at construction time.
	model.SetRepeatMode(restored.RepeatMode)
	// Regenerate shuffle_position for all queue rows when shuffle is on.
	// Done here (after app.Run) rather than inside RestoreSession so that
	// RestoreSession remains read-dominated and startup latency is predictable.
	if restored.Shuffle && len(restored.QueueEntries) > 0 {
		if err := result.Writer.WriteShuffle(true); err != nil {
			slog.Warn("startup: regenerate shuffle positions", "err", err)
		}
	}
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
	// NOTE(initial-version): PlaylistSource is always "Queue" here because
	// auto-advance is driven by the manager's in-memory queue, which has no
	// knowledge of the originating playlist. A future improvement could store
	// the playlist source on each QueueTrack so the correct name is forwarded.
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

