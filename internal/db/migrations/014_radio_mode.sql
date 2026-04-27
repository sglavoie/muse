-- +goose Up
ALTER TABLE session ADD COLUMN radio_mode INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite requires full table recreation to drop columns; column left in place.
