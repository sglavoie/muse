-- +goose Up
ALTER TABLE play_history ADD COLUMN completion_rate REAL NOT NULL DEFAULT 0;

-- Backfill from existing rows where track duration is known.
UPDATE play_history SET completion_rate = MIN(1.0,
    CAST(listened_secs AS REAL) / (
        SELECT MAX(t.duration_ms / 1000.0, 1)
        FROM tracks t WHERE t.hash = play_history.track_hash
    )
) WHERE EXISTS (
    SELECT 1 FROM tracks t
    WHERE t.hash = play_history.track_hash AND t.duration_ms > 0
);

-- +goose Down
-- SQLite cannot drop columns; this is a one-way migration.
