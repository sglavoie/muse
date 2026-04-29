-- +goose Up
CREATE TABLE recommendation_dismissals (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    type         TEXT    NOT NULL CHECK (type IN ('track')),
    value        TEXT    NOT NULL,
    dismissed_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX dismissals_type_value_idx
    ON recommendation_dismissals (type, value);

-- +goose Down
DROP TABLE IF EXISTS recommendation_dismissals;
