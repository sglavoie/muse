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
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Go(func() {
		if err := deps.LaunchMPV(ctx, socketPath); err != nil {
			errs <- fmt.Errorf("startup: mpv: %w", err)
		}
	})

	if deps.RunScan != nil {
		wg.Go(func() {
			if err := deps.RunScan(ctx, database); err != nil {
				errs <- fmt.Errorf("startup: scan: %w", err)
			}
		})
	}

	wg.Wait()
	close(errs)

	var firstErr error
	for err := range errs {
		if err == nil {
			continue
		}
		if firstErr == nil {
			firstErr = err
			continue
		}
		slog.Warn("startup: additional parallel-task error", "err", err)
	}

	return firstErr
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

// removeStaleSocket removes the mpv socket file if it exists. A missing file
// is not an error.
func removeStaleSocket(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
