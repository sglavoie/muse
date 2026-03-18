package library

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"

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
func HashWorkers(in <-chan string) <-chan HashedFile {
	return hashWorkersN(runtime.NumCPU(), in, HashFile)
}

// hashWorkersN spawns n goroutines that consume file paths from in, calling
// hashFn on each path, and emits HashedFile values to the returned channel.
// The output channel is closed once all workers have finished draining the
// input. This unexported function exists to allow tests to inject a controlled
// hashFn for measuring concurrency behaviour.
func hashWorkersN(n int, in <-chan string, hashFn func(string) (string, error)) <-chan HashedFile {
	out := make(chan HashedFile, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			for path := range in {
				hash, err := hashFn(path)
				if err != nil {
					slog.Warn("hash failed", "path", path, "err", err)
					continue
				}
				hf := HashedFile{Path: path, Hash: hash}
				if info, statErr := os.Stat(path); statErr == nil {
					hf.ModTime = info.ModTime().Unix()
					hf.SizeBytes = info.Size()
				}
				out <- hf
				slog.Debug("track hashed", "path", path, "hash", hash)
			}
		})
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
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
