-- +goose Up
CREATE TABLE track_genres (
    track_hash TEXT NOT NULL REFERENCES tracks(hash) ON DELETE CASCADE,
    genre      TEXT NOT NULL,
    PRIMARY KEY (track_hash, genre)
);
CREATE INDEX idx_track_genres_genre ON track_genres(genre);

-- Backfill from existing tracks.genre column.
-- This uses a recursive CTE to split on ";" (after replacing / and ,).
-- For tracks with empty genre, no rows are inserted.
INSERT INTO track_genres (track_hash, genre)
WITH RECURSIVE split(hash, rest, genre) AS (
    SELECT hash,
           REPLACE(REPLACE(genre, '/', ';'), ',', ';') || ';',
           ''
    FROM tracks WHERE genre != ''
    UNION ALL
    SELECT hash,
           SUBSTR(rest, INSTR(rest, ';') + 1),
           TRIM(SUBSTR(rest, 1, INSTR(rest, ';') - 1))
    FROM split WHERE rest != ''
)
SELECT DISTINCT hash, genre FROM split
WHERE genre != '';

-- +goose Down
DROP TABLE IF EXISTS track_genres;
