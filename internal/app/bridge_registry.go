package app

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// pathHashRegistry is the shared path↔hash state that watcher deletion
// handling consults to resolve a deleted file back to its content hash. Both
// the scan bridge (startup/rescan discoveries and moves) and the watch bridge
// (live changes and deletions) mutate it, so every access is guarded by mu.
//
// Seeding it from the cached DB rows and updating it from scan discoveries is
// what lets a track first seen by the startup scan be marked missing when a
// later live deletion arrives: the deletion lookup and the discovery now share
// one map instead of the watch bridge owning a private copy the scan never
// touched.
type pathHashRegistry struct {
	mu         sync.Mutex
	hashByPath map[string]string
	pathByHash map[string]string
}

// newPathHashRegistry returns a registry pre-seeded from seed (typically the
// cached DB rows' path→hash map). The seed map is copied, never retained or
// mutated, so callers keep sole ownership of it.
func newPathHashRegistry(seed map[string]string) *pathHashRegistry {
	r := &pathHashRegistry{
		hashByPath: make(map[string]string, len(seed)),
		pathByHash: make(map[string]string, len(seed)),
	}
	for path, hash := range seed {
		r.hashByPath[path] = hash
		r.pathByHash[hash] = path
	}
	return r
}

// register records that path currently holds the file identified by hash and
// keeps both directions of the mapping consistent. It returns replaced (with
// ok=true) when a *different* file previously occupied path, so the caller can
// mark the displaced hash missing. It also folds in the move case: if the same
// hash previously lived at a different path, that stale path mapping is dropped
// so a later deletion of the old path resolves to nothing (a move, not a
// removal). Re-registering the same (path, hash) is idempotent.
func (r *pathHashRegistry) register(path, hash string) (replaced string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old := r.hashByPath[path]; old != "" && old != hash {
		replaced, ok = old, true
		delete(r.pathByHash, old)
	}
	if oldPath := r.pathByHash[hash]; oldPath != "" && oldPath != path {
		delete(r.hashByPath, oldPath)
	}
	r.hashByPath[path] = hash
	r.pathByHash[hash] = path
	return replaced, ok
}

// remove resolves path to its content hash and deletes the mapping in both
// directions, returning the hash (ok=true) so the caller can mark it missing.
// It returns ok=false when the path is unknown, which is a no-op deletion.
func (r *pathHashRegistry) remove(path string) (hash string, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	hash, ok = r.hashByPath[path]
	if !ok {
		return "", false
	}
	delete(r.hashByPath, path)
	if r.pathByHash[hash] == path {
		delete(r.pathByHash, hash)
	}
	return hash, true
}

// deletionPaths expands a watcher deletion into the known track paths it
// removes. A known path resolves to itself. Any other path may be a directory
// that was moved or trashed, which fsnotify reports as a single event for the
// directory alone, so every known path below it is returned in sorted order.
// A path with nothing known below it is returned unchanged, which keeps the
// deletion a no-op for both the database and the TUI index.
func (r *pathHashRegistry) deletionPaths(path string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.hashByPath[path]; ok {
		return []string{path}
	}
	prefix := strings.TrimRight(path, string(filepath.Separator)) + string(filepath.Separator)
	var below []string
	for known := range r.hashByPath {
		if strings.HasPrefix(known, prefix) {
			below = append(below, known)
		}
	}
	if len(below) == 0 {
		return []string{path}
	}
	slices.Sort(below)
	return below
}
