-- +goose Up
ALTER TABLE tracks ADD COLUMN missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1));
ALTER TABLE queue  ADD COLUMN missing INTEGER NOT NULL DEFAULT 0 CHECK (missing IN (0, 1));

-- +goose Down
-- SQLite requires full table recreation to drop columns with constraints.
-- These columns are intentionally left in place during a rollback.
