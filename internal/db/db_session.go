package db

import (
	"fmt"
	"time"
)

// IncrementStartupCount increments the startup_count in the session row and
// returns the new count. This is used to show first-launch tips in the TUI
// for the first few sessions. The UPDATE and read of the new value happen
// atomically via RETURNING so a concurrent increment (or a write between the
// UPDATE and a separate SELECT) cannot return a stale or doubled count.
func (d *DB) IncrementStartupCount() (int, error) {
	var count int
	err := d.QueryRow(
		`UPDATE session SET startup_count = startup_count + 1, updated_at = ?
		 WHERE id = 1
		 RETURNING startup_count`,
		time.Now().Unix(),
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("db: increment startup count: %w", err)
	}
	return count, nil
}
