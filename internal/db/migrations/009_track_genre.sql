-- +goose Up
ALTER TABLE tracks ADD COLUMN genre TEXT NOT NULL DEFAULT '';

-- +goose Down
SELECT 1; -- SQLite does not support DROP COLUMN for older versions; no-op
