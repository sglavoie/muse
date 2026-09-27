-- +goose Up

-- Both indexes are left prefixes of an index SQLite already keeps, so every
-- lookup they could serve is served by the wider one, and each only cost an
-- extra b-tree write per insert/delete:
--   idx_recommendation_cache_mix (mix_id) is covered by the table's
--   PRIMARY KEY (mix_id, track_hash);
--   playlist_tracks_playlist_idx (playlist_id) is covered by the unique
--   playlist_tracks_pos_idx (playlist_id, position), which also serves the
--   ON DELETE CASCADE lookup from playlists.
DROP INDEX IF EXISTS idx_recommendation_cache_mix;
DROP INDEX IF EXISTS playlist_tracks_playlist_idx;

-- +goose Down
CREATE INDEX IF NOT EXISTS idx_recommendation_cache_mix ON recommendation_cache (mix_id);
CREATE INDEX IF NOT EXISTS playlist_tracks_playlist_idx ON playlist_tracks (playlist_id);
