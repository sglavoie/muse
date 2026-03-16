-- +goose Up
ALTER TABLE tracks ADD COLUMN mod_time INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN reliably; leave the column in place.
