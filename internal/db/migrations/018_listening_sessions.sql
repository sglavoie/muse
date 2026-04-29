-- +goose Up
ALTER TABLE play_history ADD COLUMN session_id TEXT;
CREATE INDEX idx_play_history_session ON play_history(session_id);

CREATE TABLE listening_sessions (
    id         TEXT    PRIMARY KEY,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER NOT NULL
);

-- Backfill using window functions to detect 30-min gaps.
WITH ordered AS (
    SELECT id, played_at,
           LAG(played_at) OVER (ORDER BY played_at) AS prev
    FROM play_history
),
gaps AS (
    SELECT id, played_at,
           CASE WHEN prev IS NULL OR played_at - prev > 1800
                THEN 1 ELSE 0 END AS new_session
    FROM ordered
),
sessions AS (
    SELECT id, played_at,
           SUM(new_session) OVER (ORDER BY played_at) AS session_num
    FROM gaps
)
UPDATE play_history SET session_id = (
    SELECT 'backfill-' || s.session_num
    FROM sessions s WHERE s.id = play_history.id
);

INSERT INTO listening_sessions (id, started_at, ended_at)
SELECT session_id, MIN(played_at), MAX(played_at)
FROM play_history WHERE session_id IS NOT NULL
GROUP BY session_id;

-- +goose Down
DROP TABLE IF EXISTS listening_sessions;
DROP INDEX IF EXISTS idx_play_history_session;
