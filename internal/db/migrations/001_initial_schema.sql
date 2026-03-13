-- +goose Up
CREATE TABLE tracks (
    hash        TEXT    NOT NULL PRIMARY KEY,  -- BLAKE3 content hash (composite: first+last 1 MB + size)
    path        TEXT    NOT NULL,              -- last known absolute file path
    title       TEXT    NOT NULL DEFAULT '',
    artist      TEXT    NOT NULL DEFAULT '',
    album       TEXT    NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    size_bytes  INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,              -- Unix timestamp
    updated_at  INTEGER NOT NULL               -- Unix timestamp
);

CREATE INDEX tracks_path_idx ON tracks (path);

CREATE TABLE playlists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER                         -- soft-delete; NULL means active
);

CREATE TABLE playlist_tracks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    playlist_id INTEGER NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    track_hash  TEXT    NOT NULL REFERENCES tracks (hash) ON DELETE CASCADE,
    position    INTEGER NOT NULL,              -- gapped integer (e.g. 1000, 2000) for O(1) reorder
    added_at    INTEGER NOT NULL               -- Unix timestamp
);

CREATE UNIQUE INDEX playlist_tracks_pos_idx ON playlist_tracks (playlist_id, position);
CREATE INDEX playlist_tracks_playlist_idx   ON playlist_tracks (playlist_id);

CREATE TABLE queue (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    track_hash TEXT    NOT NULL REFERENCES tracks (hash) ON DELETE CASCADE,
    position   INTEGER NOT NULL,              -- gapped integer ordering
    added_at   INTEGER NOT NULL               -- Unix timestamp
);

CREATE UNIQUE INDEX queue_pos_idx ON queue (position);

CREATE TABLE session (
    id              INTEGER PRIMARY KEY CHECK (id = 1),  -- enforce single row
    current_hash    TEXT    REFERENCES tracks (hash) ON DELETE SET NULL,
    position_ms     INTEGER NOT NULL DEFAULT 0,
    volume          REAL    NOT NULL DEFAULT 1.0,
    shuffle         INTEGER NOT NULL DEFAULT 0 CHECK (shuffle IN (0, 1)),
    repeat_mode     TEXT    NOT NULL DEFAULT 'none' CHECK (repeat_mode IN ('none', 'one', 'all')),
    updated_at      INTEGER NOT NULL
);

-- Seed the single session row so callers can always UPDATE without INSERT logic.
INSERT INTO session (id, position_ms, volume, shuffle, repeat_mode, updated_at)
VALUES (1, 0, 1.0, 0, 'none', 0);

-- +goose Down
DROP TABLE IF EXISTS session;
DROP TABLE IF EXISTS queue;
DROP TABLE IF EXISTS playlist_tracks;
DROP TABLE IF EXISTS playlists;
DROP TABLE IF EXISTS tracks;
