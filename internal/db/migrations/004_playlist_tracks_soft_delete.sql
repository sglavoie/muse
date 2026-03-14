-- +goose Up
ALTER TABLE playlist_tracks ADD COLUMN deleted_at INTEGER;  -- soft-delete; NULL means active

-- +goose Down
ALTER TABLE playlist_tracks DROP COLUMN deleted_at;
