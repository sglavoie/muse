-- +goose Up
ALTER TABLE play_history RENAME TO play_events;
ALTER TABLE play_events ADD COLUMN counted INTEGER NOT NULL DEFAULT 1;
CREATE INDEX idx_play_events_counted_played_at ON play_events(counted, played_at);

-- Columns are enumerated rather than SELECT * so a later ADD COLUMN on
-- play_events cannot silently leak into every existing play_history consumer.
CREATE VIEW play_history AS
SELECT id, track_hash, played_at, listened_secs, completion_rate, session_id
FROM play_events WHERE counted = 1;

-- +goose Down
DROP VIEW IF EXISTS play_history;
DROP INDEX IF EXISTS idx_play_events_counted_played_at;
ALTER TABLE play_events DROP COLUMN counted;
ALTER TABLE play_events RENAME TO play_history;
