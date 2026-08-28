package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/safego"
)

func runStartupTasks(ctx context.Context, database *db.DB, socketPath string, deps Deps) error {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make(chan error, 2)
	done := make(chan struct{})
	var wg sync.WaitGroup

	start := func(name string, run func(context.Context) error) {
		report := func(err error) {
			select {
			case errs <- err:
				cancel()
			default:
				if !errors.Is(err, context.Canceled) {
					slog.Warn("startup: additional parallel-task error", "err", err)
				}
			}
		}
		wg.Go(func() {
			// A panic in a startup task must not be swallowed into an apparent
			// success: RecoverTo reports it on errs exactly as a returned error
			// would, so runStartupTasks still fails startup and the caller
			// still reaches Shutdown.
			defer safego.RecoverTo("app.startupTask."+name, func(err error) {
				report(fmt.Errorf("startup: %s: %w", name, err))
			})
			if err := run(taskCtx); err != nil {
				report(fmt.Errorf("startup: %s: %w", name, err))
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
		defer safego.Recover("app.startupTasksCloser")
		defer close(errs)
		defer close(done)
		wg.Wait()
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

// Bounds for reapOrphanedMPV. The dial is a liveness probe against a local Unix
// socket, so it either connects immediately or not at all; the exit wait is
// generous enough for mpv to tear down an audio device.
const (
	orphanDialTimeout  = 250 * time.Millisecond
	orphanExitTimeout  = 2 * time.Second
	orphanPollInterval = 25 * time.Millisecond
)

// reapOrphanedMPV terminates an mpv process left listening on path by a previous
// muse that died without shutting down.
//
// Unlinking the socket file (which removeStaleSocket does next) is not enough on
// its own. mpv is spawned with --idle=yes and is not killed when its parent
// dies, so it survives a crash; removing the socket only detaches its name from
// the still-bound inode. The orphan then keeps running forever — invisible,
// since it also runs with --no-terminal --really-quiet — holding an audio device
// and, if the crash happened mid-playback, still playing.
//
// This is safe to do unconditionally at startup because it runs after the
// exclusive PID lockfile has been acquired (app.Run step 1): no other muse can
// be running, so anything still answering on muse's own socket path in muse's
// own runtime directory is definitionally an orphan of a crashed session. It can
// never reach an mpv the user started themselves, which would have neither this
// socket path nor this IPC server.
//
// Failures are logged and ignored: a surviving orphan is a leak, not a reason to
// refuse to start, and the new mpv gets a fresh socket at the same path either
// way.
func reapOrphanedMPV(path string) {
	conn, err := net.DialTimeout("unix", path, orphanDialTimeout)
	if err != nil {
		// Nothing is listening: an ordinary stale socket file (or no file).
		return
	}
	slog.Warn("startup: mpv from a previous session is still running on the IPC socket; asking it to quit", "socket", path)

	_ = conn.SetWriteDeadline(time.Now().Add(orphanDialTimeout))
	_, writeErr := conn.Write([]byte("{\"command\":[\"quit\"]}\n"))
	_ = conn.Close()
	if writeErr != nil {
		slog.Warn("startup: failed to send quit to the orphaned mpv", "socket", path, "err", writeErr)
		return
	}

	deadline := time.Now().Add(orphanExitTimeout)
	for time.Now().Before(deadline) {
		probe, err := net.DialTimeout("unix", path, orphanDialTimeout)
		if err != nil {
			slog.Info("startup: orphaned mpv exited", "socket", path)
			return
		}
		_ = probe.Close()
		time.Sleep(orphanPollInterval)
	}
	slog.Warn("startup: orphaned mpv did not exit within the timeout; it is still holding an audio device",
		"socket", path, "timeout", orphanExitTimeout)
}

// removeStaleSocket reaps any mpv still listening on path, then removes the
// socket file if it exists. A missing file is not an error, and non-socket files
// are left untouched.
func removeStaleSocket(path string) error {
	reapOrphanedMPV(path)
	if err := removeSocketFile(path); err != nil {
		return fmt.Errorf("remove stale mpv socket %q: %w", path, err)
	}
	return nil
}
