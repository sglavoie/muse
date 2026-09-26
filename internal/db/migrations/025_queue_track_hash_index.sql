-- +goose Up

-- UpsertBatch clears queue.missing with `WHERE track_hash = ?` once per scanned
-- track, and ClearQueueEntriesMissing filters the queue the same way. The only
-- queue index was on position, so each of those statements scanned the whole
-- queue table.
CREATE INDEX queue_track_hash_idx ON queue (track_hash);

-- +goose Down
DROP INDEX IF EXISTS queue_track_hash_idx;
