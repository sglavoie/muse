package palette

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// usageFileName is the JSON file written to the data directory.
const usageFileName = "palette_usage.json"

var maxUsageCount = int(^uint(0) >> 1)

// UsageRecord captures how often and how recently a command was confirmed.
type UsageRecord struct {
	Count    int       `json:"count"`
	LastUsed time.Time `json:"last_used"`
}

// UsageStore tracks per-command usage and persists it to disk best-effort.
type UsageStore struct {
	path    string
	records map[string]UsageRecord
}

// NewUsageStore returns a store backed by <dataDir>/palette_usage.json. When
// dataDir is empty, persistence is disabled and the store only lives in
// memory (useful for tests).
func NewUsageStore(dataDir string) *UsageStore {
	path := ""
	if dataDir != "" {
		path = filepath.Join(dataDir, usageFileName)
	}
	return &UsageStore{
		path:    path,
		records: make(map[string]UsageRecord),
	}
}

// Load reads usage records from disk. A missing file is not an error.
// Corrupt files are reported via slog and the in-memory map is left empty.
func (s *UsageStore) Load() error {
	if s == nil || s.path == "" {
		return nil
	}
	s.records = make(map[string]UsageRecord)
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	records := make(map[string]UsageRecord)
	if err := json.Unmarshal(data, &records); err != nil {
		slog.Warn("palette usage: parse failed, starting fresh", "path", s.path, "err", err)
		// Usage history is best-effort only; treat parse errors as recoverable.
		return nil
	}
	s.records = sanitizeUsageRecords(records)
	return nil
}

// sanitizeUsageRecords clears non-positive counts and future LastUsed stamps so
// a hand-edited or clock-skewed file cannot distort ranking.
func sanitizeUsageRecords(records map[string]UsageRecord) map[string]UsageRecord {
	if records == nil {
		return make(map[string]UsageRecord)
	}
	now := time.Now()
	for key, rec := range records {
		if rec.Count <= 0 {
			records[key] = UsageRecord{}
			continue
		}
		if rec.LastUsed.After(now) {
			rec.LastUsed = time.Time{}
			records[key] = rec
		}
	}
	return records
}

// Save writes the current records to disk. Errors are logged but not
// returned to callers via the UI path (use SaveBestEffort for that).
func (s *UsageStore) Save() error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.records, "", "  ")
	if err != nil {
		return err
	}
	return saveUsageAtomically(s.path, 0o644, data)
}

// SaveBestEffort writes the store to disk and swallows errors with a warning.
// Intended for UI hot paths where save failure should not interrupt the user.
func (s *UsageStore) SaveBestEffort() {
	if err := s.Save(); err != nil {
		slog.Warn("palette usage: save failed", "path", s.path, "err", err)
	}
}

// Bump increments the count for key and updates LastUsed to now. A nil store
// is a no-op so callers don't need to check.
func (s *UsageStore) Bump(key string) {
	if s == nil || key == "" {
		return
	}
	if s.records == nil {
		s.records = make(map[string]UsageRecord)
	}
	rec := s.records[key]
	if rec.Count < maxUsageCount {
		rec.Count++
	}
	rec.LastUsed = time.Now()
	s.records[key] = rec
}

// Get returns the record for key (zero value if absent).
func (s *UsageStore) Get(key string) UsageRecord {
	if s == nil || key == "" {
		return UsageRecord{}
	}
	return s.records[key]
}

func saveUsageAtomically(path string, perm os.FileMode, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = os.Remove(tmpPath)
	}

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
