-- +goose Up
ALTER TABLE tracks ADD COLUMN album_artist TEXT NOT NULL DEFAULT '';
-- Refresh unchanged files once so existing libraries receive the new tag.
-- User-edited title/artist/album/genre fields remain protected by the upsert.
UPDATE tracks SET mod_time = 0;

-- +goose Down
ALTER TABLE tracks DROP COLUMN album_artist;
