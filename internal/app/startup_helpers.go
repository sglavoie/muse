package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/sglavoie/muse/internal/db"
)

func runStartupTasks(ctx context.Context, database *db.DB, socketPath string, deps Deps) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make(chan error, 2)
	done := make(chan struct{})
	var wg sync.WaitGroup

	start := func(name string, run func(context.Context) error) {
		wg.Go(func() {
			if err := run(taskCtx); err != nil {
				wrapped := fmt.Errorf("startup: %s: %w", name, err)
				select {
				case errs <- wrapped:
					cancel()
				default:
					if !errors.Is(err, context.Canceled) {
						slog.Warn("startup: additional parallel-task error", "err", wrapped)
					}
				}
			}
		})
	}

	start("mpv", func(ctx context.Context) error {
		return deps.LaunchMPV(ctx, socketPath)
	})

	if deps.RunScan != nil {
		start("scan", func(ctx context.Context) error {
			return deps.RunScan(ctx, database)
		})
	}

	go func() {
		wg.Wait()
		close(done)
		close(errs)
	}()

	select {
	case err, ok := <-errs:
		if !ok {
			return nil
		}
		<-done
		for extra := range errs {
			if extra == nil || errors.Is(extra, context.Canceled) {
				continue
			}
			slog.Warn("startup: additional parallel-task error", "err", extra)
		}
		return err
	case <-done:
		return nil
	}
}

func restoreStartupPlayer(restored *RestoredSession, restorePlayer func(path string, positionMs int64) error) {
	if restored.CurrentHash == "" || restorePlayer == nil {
		return
	}

	for i, pt := range restored.PlayerTracks {
		if pt.Hash != restored.CurrentHash {
			continue
		}
		if i < len(restored.QueueEntries) && restored.QueueEntries[i].Missing {
			slog.Info("startup: skip restore for missing current track", "path", pt.Path)
			return
		}
		if err := restorePlayer(pt.Path, restored.PositionMs); err != nil {
			slog.Warn("startup: restore player position", "path", pt.Path, "err", err)
		}
		return
	}

	slog.Warn("startup: current_hash from session does not match any player track; skipping player restore",
		"hash", restored.CurrentHash, "queue_size", len(restored.PlayerTracks))
}

func removeSocketFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeStaleSocket removes the mpv socket file if it exists. A missing file
// is not an error, and non-socket files are left untouched.
func removeStaleSocket(path string) error {
	if err := removeSocketFile(path); err != nil {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
