package library

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/sglavoie/muse/internal/safego"
	"github.com/zeebo/blake3"
)

// HashedFile pairs a file path with its BLAKE3 fingerprint and file stat info.
type HashedFile struct {
	Path      string
	Hash      string
	ModTime   int64 // Unix timestamp of last modification
	SizeBytes int64 // file size in bytes
}

// HashWorkers spawns runtime.NumCPU() goroutines that consume file paths from
// in and emit HashedFile values to the returned channel. The output channel is
// closed once all workers have finished draining the input.
//
// The returned StageOutcome distinguishes a complete drain from one cut short by
// a recovered worker panic. Both close the output channel, so a caller that
// treats the close as scan-complete would otherwise act on a silently short
// batch — most damagingly by concluding that the files that were never hashed
// have been deleted. Check Err after the channel has been drained.
func HashWorkers(in <-chan string) (<-chan HashedFile, *StageOutcome) {
	return hashWorkersN(runtime.NumCPU(), in, hashFileFn)
}

// hashFileFn is HashFile, indirected so tests can drive the exported HashWorkers
// (and therefore Scan) into the one failure a real filesystem cannot be made to
// produce on demand: a panicking hash worker.
var hashFileFn = HashFile

// hashWorkersN spawns n goroutines that consume file paths from in, calling
// hashFn on each path, and emits HashedFile values to the returned channel.
// The output channel is closed once all workers have finished draining the
// input. This unexported function exists to allow tests to inject a controlled
// hashFn for measuring concurrency behaviour.
func hashWorkersN(n int, in <-chan string, hashFn func(string) (string, error)) (<-chan HashedFile, *StageOutcome) {
	out := make(chan HashedFile, n)
	outcome := &StageOutcome{}
	var wg sync.WaitGroup
	// live counts the workers that have not yet died to a panic. The last one to
	// go has to drain in itself: nothing else is reading it any more, and the
	// walker goroutine upstream would otherwise block forever on `ch <- path`.
	var live atomic.Int64
	live.Store(int64(n))
	for range n {
		wg.Go(func() {
			// hashFn is injected and, in production, reads arbitrary files off
			// disk; a panic here must not take the process down. Recovering
			// stops this worker early; the remaining workers keep draining in,
			// and when none are left the handler drains it so the producer can
			// finish. wg.Wait below still closes out, so no consumer hangs.
			defer safego.RecoverTo("library.hashWorker", func(err error) {
				outcome.Fail(errors.Join(ErrHashWorkerPanicked, err))
				if live.Add(-1) > 0 {
					return
				}
				for range in { //nolint:revive // draining to unblock the producer
				}
			})
			for path := range in {
				hash, err := hashFn(path)
				if err != nil {
					slog.Warn("hash failed", "path", path, "err", err)
					continue
				}
				// If Stat fails (file removed between hashing and statting),
				// skip emitting rather than passing zero ModTime/SizeBytes
				// downstream. Zero values would cause DiffScan to flag the
				// path as changed on every subsequent rescan.
				info, statErr := os.Stat(path)
				if statErr != nil {
					slog.Warn("hash: stat after hash failed, skipping", "path", path, "err", statErr)
					continue
				}
				out <- HashedFile{
					Path:      path,
					Hash:      hash,
					ModTime:   info.ModTime().Unix(),
					SizeBytes: info.Size(),
				}
				slog.Debug("track hashed", "path", path, "hash", hash)
			}
		})
	}
	go func() {
		defer safego.Recover("library.hashWorkersCloser")
		// close(out) is deferred so it still runs on the recovery path, and it
		// is registered after wg.Wait so the outcome is populated before any
		// consumer can observe the close and read it.
		defer close(out)
		wg.Wait()
		if err := outcome.Err(); err != nil {
			// The closed output channel otherwise reads as "every path was
			// hashed"; say plainly that the batch is short. Callers learn the
			// same thing from outcome.Err.
			slog.Error("hash stage incomplete", "err", err)
		}
	}()
	return out, outcome
}

const (
	// partialThreshold is the file size above which partial hashing is used.
	partialThreshold = 2 * 1024 * 1024 // 2 MB
	// chunkSize is the number of bytes read from each end for partial hashing.
	chunkSize = 1 * 1024 * 1024 // 1 MB
)

// HashFile returns a collision-resistant hex fingerprint for the audio file at
// path. For files smaller than 2 MB the full content is hashed. For larger
// files the first 1 MB and last 1 MB are concatenated with the 8-byte
// little-endian file size before hashing.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := info.Size()

	h := blake3.New()

	if size < partialThreshold {
		// Full-file hash.
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
	} else {
		// Partial hash: first 1 MB.
		if _, err := io.CopyN(h, f, chunkSize); err != nil {
			return "", err
		}
		// Last 1 MB.
		if _, err := f.Seek(-chunkSize, io.SeekEnd); err != nil {
			return "", err
		}
		if _, err := io.CopyN(h, f, chunkSize); err != nil {
			return "", err
		}
		// File size as 8-byte little-endian suffix.
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(size))
		h.Write(buf[:])
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
