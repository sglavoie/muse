package db

import (
	"fmt"
	"time"
)

// MixProgress describes how close a mix is to having enough data to show
// recommendations, for display in the empty-state of each mix.
type MixProgress struct {
	Current     int
	Needed      int
	Description string
}

// ReadAllMixProgress queries scalar progress indicators for each mix and
// returns them keyed by MixID. These values are cheap (indexed scalar queries)
// and are shown when a mix has zero tracks to explain what data is needed.
func (d *DB) ReadAllMixProgress() (map[MixID]MixProgress, error) {
	result := make(map[MixID]MixProgress, 5)

	// Rediscover: tracks with play_count >= 10 (need ≥1 unheard in 90+ days).
	var rediscoverCount int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM tracks WHERE missing = 0 AND play_count >= 10`,
	).Scan(&rediscoverCount); err != nil {
		return nil, fmt.Errorf("db: mix progress rediscover: %w", err)
	}
	result[MixRediscover] = MixProgress{
		Current:     rediscoverCount,
		Description: fmt.Sprintf("%d tracks with 10+ plays, none older than 90 days", rediscoverCount),
	}

	// HiddenGems and NeverPlayed both require 20+ total plays.
	var totalPlays int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM play_history`,
	).Scan(&totalPlays); err != nil {
		return nil, fmt.Errorf("db: mix progress total plays: %w", err)
	}
	result[MixHiddenGems] = MixProgress{
		Current:     totalPlays,
		Needed:      20,
		Description: fmt.Sprintf("%d total plays (need 20+ to identify taste)", totalPlays),
	}
	result[MixNeverPlayed] = MixProgress{
		Current:     totalPlays,
		Needed:      20,
		Description: fmt.Sprintf("%d total plays (need 20+ to surface unplayed)", totalPlays),
	}

	// OnRepeat: plays in the last 7 days (need 3+ on a single track).
	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()
	var weekPlays int
	if err := d.QueryRow(
		`SELECT COUNT(*) FROM play_history WHERE played_at >= ?`, sevenDaysAgo,
	).Scan(&weekPlays); err != nil {
		return nil, fmt.Errorf("db: mix progress on repeat: %w", err)
	}
	result[MixOnRepeat] = MixProgress{
		Current:     weekPlays,
		Description: fmt.Sprintf("%d plays this week (need 3+ on a single track)", weekPlays),
	}

	// DeepCuts: skipped listens on record. Unlike the other mixes, more history
	// alone never makes this one ready — its completion filter stays inert
	// until the log holds listens that failed to complete.
	skips, err := d.countSkippedListens()
	if err != nil {
		return nil, fmt.Errorf("db: mix progress deep cuts: %w", err)
	}
	result[MixDeepCuts] = MixProgress{
		Current: skips,
		Needed:  deepCutsMinSkips,
		Description: fmt.Sprintf("%d skipped tracks recorded (need %d+ before completion can discriminate)",
			skips, deepCutsMinSkips),
	}

	return result, nil
}
