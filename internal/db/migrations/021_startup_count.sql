-- +goose Up
ALTER TABLE session ADD COLUMN startup_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE session DROP COLUMN startup_count;
