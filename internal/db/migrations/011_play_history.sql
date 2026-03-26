-- +goose Up
CREATE TABLE play_history (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    track_hash    TEXT    NOT NULL REFERENCES tracks(hash),
    played_at     INTEGER NOT NULL,
    listened_secs INTEGER NOT NULL
);
CREATE INDEX idx_play_history_played_at ON play_history(played_at);
CREATE INDEX idx_play_history_track_hash ON play_history(track_hash);
ALTER TABLE tracks ADD COLUMN last_played_at INTEGER NOT NULL DEFAULT 0;

-- +goose Down
DROP TABLE IF EXISTS play_history;
