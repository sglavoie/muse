-- +goose Up
-- Old metadata edits did not include Album Artist; keep refreshing that file tag
-- until a metadata operation saves a value with the new writer.
ALTER TABLE tracks ADD COLUMN album_artist_edited INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tracks DROP COLUMN album_artist_edited;
