-- +goose Up
CREATE TABLE tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    color      TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    deleted_at INTEGER
);

CREATE TABLE track_tags (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    track_hash TEXT    NOT NULL REFERENCES tracks (hash) ON DELETE CASCADE,
    tag_id     INTEGER NOT NULL REFERENCES tags (id)     ON DELETE CASCADE,
    created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX track_tags_unique_idx ON track_tags (track_hash, tag_id);
CREATE INDEX track_tags_tag_idx  ON track_tags (tag_id);
CREATE INDEX track_tags_hash_idx ON track_tags (track_hash);

-- +goose Down
DROP INDEX IF EXISTS track_tags_hash_idx;
DROP INDEX IF EXISTS track_tags_tag_idx;
DROP INDEX IF EXISTS track_tags_unique_idx;
DROP TABLE IF EXISTS track_tags;
DROP TABLE IF EXISTS tags;
