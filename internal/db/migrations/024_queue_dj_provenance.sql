-- +goose Up

-- Explicit DJ ownership for a queue row. Provenance is recorded when the row is
-- inserted and is never inferred from source_playlist: that column holds a
-- user-visible display label, and a user is free to name a playlist "Radio".
-- Deciding ownership from the label would let a refresh silently discard tracks
-- the user queued by hand.
--
-- Every row that already exists when this migration runs keeps the default 0
-- and is therefore protected as a manual entry.
ALTER TABLE queue ADD COLUMN dj_owned INTEGER NOT NULL DEFAULT 0;

-- Structured selection reasons for a DJ-selected row, stored as a JSON array of
-- {code, value, detail} objects. Empty string means "no reasons recorded",
-- which is what every manual entry carries.
ALTER TABLE queue ADD COLUMN dj_reasons TEXT NOT NULL DEFAULT '';

-- Preparation state of a batch that has been persisted but not yet accepted:
-- '' for a settled row, 'append' for an ordinary batch, 'replace' for a
-- prepared refresh. Rows still carrying a non-empty value at startup belong to
-- a preparation a previous process abandoned, and recovery turns them into
-- ordinary tombstones.
ALTER TABLE queue ADD COLUMN pending_kind TEXT NOT NULL DEFAULT '';

-- The three columns deliberately carry no CHECK constraints: SQLite cannot drop
-- a column that appears in one, and the Down section below has to leave the
-- schema in a state a second Up can rebuild. Their domains are enforced in Go,
-- which is the only writer of this table.

-- +goose Down
ALTER TABLE queue DROP COLUMN pending_kind;
ALTER TABLE queue DROP COLUMN dj_reasons;
ALTER TABLE queue DROP COLUMN dj_owned;
