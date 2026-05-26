package palette

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsageStoreBumpIncrementsAndStampsTime(t *testing.T) {
	store := NewUsageStore("")
	before := time.Now()
	store.Bump("foo")
	store.Bump("foo")
	rec := store.Get("foo")
	if rec.Count != 2 {
		t.Errorf("count after two bumps: want 2, got %d", rec.Count)
	}
	if rec.LastUsed.Before(before) {
		t.Errorf("LastUsed should be at or after the test start; got %v", rec.LastUsed)
	}
}

func TestUsageStoreBumpSaturatesAtMaxCount(t *testing.T) {
	store := NewUsageStore("")
	store.records["foo"] = UsageRecord{Count: maxUsageCount}
	store.Bump("foo")
	rec := store.Get("foo")
	if rec.Count != maxUsageCount {
		t.Errorf("count should saturate at max int; got %d", rec.Count)
	}
	if rec.LastUsed.IsZero() {
		t.Errorf("LastUsed should still be updated when saturated")
	}
}

func TestUsageStoreGetMissingReturnsZero(t *testing.T) {
	store := NewUsageStore("")
	rec := store.Get("never")
	if rec.Count != 0 {
		t.Errorf("missing key count: want 0, got %d", rec.Count)
	}
	if !rec.LastUsed.IsZero() {
		t.Errorf("missing key LastUsed: want zero, got %v", rec.LastUsed)
	}
}

func TestUsageStoreNilSafe(t *testing.T) {
	var store *UsageStore
	store.Bump("foo")
	rec := store.Get("foo")
	if rec.Count != 0 {
		t.Errorf("nil store should return zero record; got %+v", rec)
	}
	if err := store.Save(); err != nil {
		t.Errorf("nil Save should be no-op; got %v", err)
	}
	if err := store.Load(); err != nil {
		t.Errorf("nil Load should be no-op; got %v", err)
	}
}

func TestUsageStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := NewUsageStore(dir)
	a.Bump("alpha")
	a.Bump("alpha")
	a.Bump("beta")
	if err := a.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	b := NewUsageStore(dir)
	if err := b.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := b.Get("alpha").Count; got != 2 {
		t.Errorf("alpha count after round-trip: want 2, got %d", got)
	}
	if got := b.Get("beta").Count; got != 1 {
		t.Errorf("beta count after round-trip: want 1, got %d", got)
	}
}

func TestUsageStoreLoadMissingFileIsNotError(t *testing.T) {
	dir := t.TempDir()
	store := NewUsageStore(dir)
	store.Bump("stale")
	if err := store.Load(); err != nil {
		t.Fatalf("Load on missing file should not error; got %v", err)
	}
	if got := store.Get("stale").Count; got != 0 {
		t.Fatalf("missing-file load should clear stale records, got %d", got)
	}
}

func TestUsageStoreLoadCorruptIsRecoverable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	store := NewUsageStore(dir)
	store.Bump("stale")
	// Load returns the parse error, but Bump should still work afterwards.
	_ = store.Load()
	if got := store.Get("stale").Count; got != 0 {
		t.Fatalf("corrupt load should clear stale records, got %d", got)
	}
	store.Bump("alpha")
	if got := store.Get("alpha").Count; got != 1 {
		t.Errorf("alpha count after recovery: want 1, got %d", got)
	}
}

func TestUsageStoreLoadEmptyFileClearsStaleRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	store := NewUsageStore(dir)
	store.Bump("stale")
	if err := store.Load(); err != nil {
		t.Fatalf("Load on empty file: %v", err)
	}
	if got := store.Get("stale").Count; got != 0 {
		t.Fatalf("empty-file load should clear stale records, got %d", got)
	}
}

func TestUsageStoreLoadNullFileNormalizesToEmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)
	if err := os.WriteFile(path, []byte("null"), 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	store := NewUsageStore(dir)
	store.Bump("stale")
	if err := store.Load(); err != nil {
		t.Fatalf("Load on null file: %v", err)
	}
	if got := store.Get("stale").Count; got != 0 {
		t.Fatalf("null-file load should clear stale records, got %d", got)
	}
	if err := store.Save(); err != nil {
		t.Fatalf("Save after null-file load: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved usage file: %v", err)
	}
	if string(data) != "{}" {
		t.Fatalf("Save after null-file load should write empty object, got %q", string(data))
	}
}

func TestUsageStoreLoadNegativeCountSanitized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)
	data := []byte(`{"alpha":{"count":-5,"last_used":"2024-01-01T00:00:00Z"}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	store := NewUsageStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := store.Get("alpha").Count; got != 0 {
		t.Fatalf("negative persisted count should be sanitized to 0, got %d", got)
	}
	store.Bump("alpha")
	if got := store.Get("alpha").Count; got != 1 {
		t.Fatalf("count after bump should start from 0, got %d", got)
	}
}

func TestUsageStoreLoadNonPositiveCountClearsLastUsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)
	data := []byte(`{"alpha":{"count":0,"last_used":"2024-01-01T00:00:00Z"}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("setup write: %v", err)
	}
	store := NewUsageStore(dir)
	if err := store.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := store.Get("alpha").Count; got != 0 {
		t.Fatalf("zero persisted count should remain 0, got %d", got)
	}
	if got := store.Get("alpha").LastUsed; !got.IsZero() {
		t.Fatalf("non-positive persisted count should clear LastUsed, got %v", got)
	}
}

func TestUsageStoreEmptyDataDirNoPersistence(t *testing.T) {
	store := NewUsageStore("")
	store.Bump("alpha")
	// Save is a no-op with empty path; should not error.
	if err := store.Save(); err != nil {
		t.Errorf("Save on empty data dir should be no-op; got %v", err)
	}
}

func TestUsageStoreSaveCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	// Use a nested dir that does not yet exist.
	dir := filepath.Join(base, "nested", "subdir")
	store := NewUsageStore(dir)
	store.Bump("alpha")
	if err := store.Save(); err != nil {
		t.Fatalf("Save should create parent dirs; got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, usageFileName)); err != nil {
		t.Errorf("expected file at %s; stat err: %v", dir, err)
	}
}

func TestUsageStoreSaveBestEffortNeverPanics(t *testing.T) {
	// A nil store should be no-op.
	var store *UsageStore
	store.SaveBestEffort()
}

func TestUsageStoreSave_PreservesPreviousFileOnReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, usageFileName)

	store := NewUsageStore(dir)
	store.Bump("alpha")
	if err := store.Save(); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	pre, err := os.Open(path)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer pre.Close()

	preInfo, err := pre.Stat()
	if err != nil {
		t.Fatalf("stat original: %v", err)
	}
	originalSize := preInfo.Size()

	store.Bump("beta-with-longer-key")
	if err := store.Save(); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	post, err := pre.Stat()
	if err != nil {
		t.Fatalf("stat via open fd after second save: %v", err)
	}
	if post.Size() != originalSize {
		t.Errorf("open fd size = %d, want unchanged %d (Save should replace, not truncate, the original inode)",
			post.Size(), originalSize)
	}

	loaded := NewUsageStore(dir)
	if err := loaded.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := loaded.Get("alpha").Count; got != 1 {
		t.Fatalf("alpha count after atomic save: want 1, got %d", got)
	}
	if got := loaded.Get("beta-with-longer-key").Count; got != 1 {
		t.Fatalf("beta count after atomic save: want 1, got %d", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Errorf("leftover temp file after Save: %q", entry.Name())
		}
	}
}
