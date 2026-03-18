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

	// Load the cached library from the database for instant startup display.
	dbTracks, err := result.DB.ReadTracks()
	if err != nil {
		slog.Warn("startup: read tracks from DB", "err", err)
	}

	// Build the library track slice and known-file map for the diff scan.
	libraryTracks := make([]library.Track, len(dbTracks))
	known := make(map[string]library.KnownFile, len(dbTracks))
	for i, row := range dbTracks {
		libraryTracks[i] = library.Track{
			Hash:      row.Hash,
			Path:      row.Path,
			Title:     row.Title,
			Artist:    row.Artist,
			Album:     row.Album,
			Duration:  float64(row.DurationMs) / 1000.0,
			ModTime:   row.ModTime,
			SizeBytes: row.SizeBytes,
		}
		known[row.Path] = library.KnownFile{
			ModTime:   row.ModTime,
			SizeBytes: row.SizeBytes,
		}
	}

	// Diff scan: walk the filesystem and only process files that are new or
	// have changed since the last scan. Deletions are returned synchronously.
	const upsertBatchSize = 500
	rawScanCh, deletedPaths := library.DiffScan(cfg.MusicDirectory, extractor, known)

	// batchFlusher accumulates library.Track values and flushes them in
	// batches to a caller-provided function (typically DB upsert).
	type batchFlusher struct {
		batch []library.Track
		size  int
		flush func([]library.Track) error
	}
	flushBatch := func(f *batchFlusher) {
		if len(f.batch) == 0 {
			return
		}
		if err := f.flush(f.batch); err != nil {
			slog.Warn("batch flush", "err", err, "count", len(f.batch))
		}
		f.batch = f.batch[:0]
	}
	addToBatch := func(f *batchFlusher, t library.Track) {
		f.batch = append(f.batch, t)
		if len(f.batch) >= f.size {
			flushBatch(f)
		}
	}

	// Bridge new/changed tracks to both the DB and the TUI scan channel.
	// tuiScanCh is buffered so the bridge goroutine does not stall when the
	// TUI is busy processing messages.
	tuiScanCh := make(chan library.Track, 256)
	go func() {
		defer close(tuiScanCh)
		f := &batchFlusher{size: upsertBatchSize, flush: result.Writer.UpsertBatch}
		for t := range rawScanCh {
			tuiScanCh <- t
			addToBatch(f, t)
		}
		flushBatch(f)
	}()

	// Build path→hash lookup from cached tracks. Used by the deletion-path
	// handler in both the pre-TUI eviction loop and the watcher bridge.
	hashByPath := make(map[string]string, len(libraryTracks))
	for _, t := range libraryTracks {
		hashByPath[t.Path] = t.Hash
	}

	rawWatchCh, watchErr := library.Watch(ctx, cfg.MusicDirectory, extractor, library.DefaultDebounce)
	if watchErr != nil {
		slog.Error("failed to start file watcher", "err", watchErr)
	}

	// Bridge watcher events to both the DB (for persistence) and the TUI
	// (for in-memory index updates). Without this bridge, watcher-discovered
	// tracks would appear in the TUI but never reach the tracks table,
	// causing FK violations when the user tries to enqueue them and data
	// loss on restart.
	tuiWatchCh := make(chan library.WatchEvent, 64)
	go func() {
		defer close(tuiWatchCh)
		f := &batchFlusher{size: upsertBatchSize, flush: result.Writer.UpsertBatch}
		for ev := range rawWatchCh {
			tuiWatchCh <- ev
			if ev.Deleted {
				flushBatch(f)
				if hash, ok := hashByPath[ev.Path]; ok {
					if err := result.Writer.MarkTrackMissing(hash); err != nil {
						slog.Warn("watcher: mark deleted track missing", "path", ev.Path, "err", err)
					}
				}
			} else {
				addToBatch(f, ev.Track)
			}
		}
		flushBatch(f)
	}()

	restored := result.Restored

	model := tui.New(ctx, configPath, configCreated, tuiScanCh, tuiWatchCh, cfg.MusicDirectory, extractor, mgr)
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
	model.SetBatchEnqueuer(result.Writer)
	model.SetMetadataWriter(result.Writer)

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

	// Pre-populate the library with cached tracks for instant display.
	model.SetLibraryTracks(libraryTracks)

	// Evict deleted files from the library index before the TUI starts.
	for _, path := range deletedPaths {
		model.RemoveLibraryTrackByPath(path)
		if hash, ok := hashByPath[path]; ok {
			if err := result.Writer.MarkTrackMissing(hash); err != nil {
				slog.Warn("startup: mark deleted track missing", "path", path, "err", err)
			}
		}
	}

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
	model.SetVolume(cfg.DefaultVolume)
	// Propagate restored repeat/shuffle modes into mpv so its behaviour
	// matches the status bar indicators from the first user interaction.
	_ = mgr.SetRepeat(restored.RepeatMode.String())
	_ = mgr.SetShuffle(restored.Shuffle)
	if len(restored.QueueEntries) > 0 {
		model.SetPlaybackState(tui.PlaybackPaused)
	}
	if restored.CurrentHash != "" {
		model.SetCurrentTrackHash(restored.CurrentHash)
		// Set the player's currentIndex so that Next/Previous navigate
		// relative to the restored track rather than defaulting to index 0.
		for i, pt := range restored.PlayerTracks {
			if pt.Hash == restored.CurrentHash {
				mgr.SetCurrentIndex(i)
				break
			}
		}
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
