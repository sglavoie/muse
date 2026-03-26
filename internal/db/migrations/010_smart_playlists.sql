-- +goose Up
ALTER TABLE playlists ADD COLUMN query TEXT;
ALTER TABLE tracks ADD COLUMN play_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
SELECT 1; -- SQLite does not support DROP COLUMN for older versions; no-op
