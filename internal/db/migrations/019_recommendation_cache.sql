-- +goose Up
CREATE TABLE recommendation_cache (
    mix_id      INTEGER NOT NULL,
    track_hash  TEXT    NOT NULL REFERENCES tracks(hash),
    score       REAL    NOT NULL DEFAULT 0,
    computed_at INTEGER NOT NULL,
    PRIMARY KEY (mix_id, track_hash)
);
CREATE INDEX idx_recommendation_cache_mix
    ON recommendation_cache(mix_id);

-- +goose Down
DROP TABLE IF EXISTS recommendation_cache;
