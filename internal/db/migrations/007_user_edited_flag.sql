-- +goose Up
ALTER TABLE tracks ADD COLUMN user_edited INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tracks DROP COLUMN user_edited;
