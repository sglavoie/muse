-- +goose Up
CREATE TABLE favorites (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    type       TEXT    NOT NULL CHECK (type IN ('artist', 'album', 'track')),
    value      TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX favorites_type_value_idx ON favorites (type, value);

-- +goose Down
DROP TABLE IF EXISTS favorites;
