-- +goose Up
ALTER TABLE tracks ADD COLUMN duration_verified INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot drop columns with constraints; intentionally left.
