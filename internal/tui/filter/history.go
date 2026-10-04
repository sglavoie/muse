package filter

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sglavoie/muse/internal/fsutil"
)

const historyFile = "filter_history.json"

// LoadHistory restores recall history without applying a filter. An empty
// directory disables persistence, as in fixture-backed and unit-test models.
func LoadHistory(dataDir string) ([]string, error) {
	if dataDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dataDir, historyFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var history []string
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	// Apply the same blank-query, deduplication and size rules as live input.
	s := State{}
	for _, query := range history {
		s.Text = query
		s = s.Commit()
	}
	return s.History(), nil
}

// SaveHistory atomically replaces the small recall file. Callers serialize
// saves on the update loop, so an older snapshot cannot overwrite a newer one.
func SaveHistory(dataDir string, history []string) error {
	if dataDir == "" {
		return nil
	}
	return fsutil.WriteAtomically(filepath.Join(dataDir, historyFile), 0o600, func(f *os.File) error {
		return json.NewEncoder(f).Encode(history)
	})
}
