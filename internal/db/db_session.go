package db

import (
	"fmt"
	"time"
)

// IncrementStartupCount increments the startup_count in the session row and
// returns the new count. This is used to show first-launch tips in the TUI
// for the first few sessions.
func (d *DB) IncrementStartupCount() (int, error) {
	_, err := d.Exec(
		`UPDATE session SET startup_count = startup_count + 1, updated_at = ? WHERE id = 1`,
		time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("db: increment startup count: %w", err)
	}
	var count int
	if err := d.QueryRow(`SELECT startup_count FROM session WHERE id = 1`).Scan(&count); err != nil {
		return 0, fmt.Errorf("db: read startup count: %w", err)
	}
	return count, nil
}
