-- +goose Up

-- Named DJ recipes. A recipe stores settings only. The captured seed lives in
-- dj_session, so editing or deleting a recipe can never change what is playing:
-- applying a recipe copies its settings into the session snapshot.
CREATE TABLE dj_recipes (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT    NOT NULL,
    version           INTEGER NOT NULL DEFAULT 1,
    discovery_percent INTEGER NOT NULL,
    anchor_mode       TEXT    NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    deleted_at        INTEGER
);

-- Names are unique case-insensitively among active recipes only, so a
-- soft-deleted recipe never holds its old name hostage.
CREATE UNIQUE INDEX dj_recipes_active_name_idx
    ON dj_recipes (name COLLATE NOCASE) WHERE deleted_at IS NULL;

-- Singleton row holding the active DJ session: the applied settings snapshot,
-- the seed captured when the session started, and the discovery rounding carry.
-- The snapshot is deliberately independent of dj_recipes.
CREATE TABLE dj_session (
    id                INTEGER PRIMARY KEY CHECK (id = 1),
    active            INTEGER NOT NULL DEFAULT 0,
    version           INTEGER NOT NULL DEFAULT 1,
    discovery_percent INTEGER NOT NULL DEFAULT 40,
    anchor_mode       TEXT    NOT NULL DEFAULT 'anchor',
    carry_units       INTEGER NOT NULL DEFAULT 0,
    seed_hash         TEXT    NOT NULL DEFAULT '',
    seed_title        TEXT    NOT NULL DEFAULT '',
    seed_artist       TEXT    NOT NULL DEFAULT '',
    seed_album        TEXT    NOT NULL DEFAULT '',
    seed_genres       TEXT    NOT NULL DEFAULT '[]',
    seed_duration     REAL    NOT NULL DEFAULT 0,
    seed_captured_at  INTEGER NOT NULL DEFAULT 0,
    updated_at        INTEGER NOT NULL DEFAULT 0
);
INSERT INTO dj_session (id) VALUES (1);

-- Tag references held by DJ settings (include/exclude) and by the captured
-- seed. Stored as plain ids with no foreign key on purpose: soft-deleting or
-- purging a tag must leave the reference in place so the DJ can report it as
-- unresolved, rather than silently dropping a restrictive filter or widening an
-- exclusion. Validity is decided against the active tag catalog, never by the
-- presence of a raw tags row.
CREATE TABLE dj_tag_refs (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    scope    TEXT    NOT NULL CHECK (scope IN ('session', 'recipe')),
    owner_id INTEGER NOT NULL,
    role     TEXT    NOT NULL CHECK (role IN ('include', 'exclude', 'seed')),
    tag_id   INTEGER NOT NULL
);
CREATE UNIQUE INDEX dj_tag_refs_unique_idx
    ON dj_tag_refs (scope, owner_id, role, tag_id);
CREATE INDEX dj_tag_refs_tag_idx ON dj_tag_refs (tag_id);

-- Genre and artist filter values for DJ settings. Kept relational alongside
-- dj_tag_refs so one owner's filters are rewritten as a unit.
CREATE TABLE dj_filter_values (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    scope    TEXT    NOT NULL CHECK (scope IN ('session', 'recipe')),
    owner_id INTEGER NOT NULL,
    role     TEXT    NOT NULL CHECK (role IN ('include', 'exclude')),
    kind     TEXT    NOT NULL CHECK (kind IN ('genre', 'artist')),
    value    TEXT    NOT NULL
);
CREATE UNIQUE INDEX dj_filter_values_unique_idx
    ON dj_filter_values (scope, owner_id, role, kind, value COLLATE NOCASE);
CREATE INDEX dj_filter_values_owner_idx ON dj_filter_values (scope, owner_id);

-- +goose Down
DROP INDEX IF EXISTS dj_filter_values_owner_idx;
DROP INDEX IF EXISTS dj_filter_values_unique_idx;
DROP TABLE IF EXISTS dj_filter_values;
DROP INDEX IF EXISTS dj_tag_refs_tag_idx;
DROP INDEX IF EXISTS dj_tag_refs_unique_idx;
DROP TABLE IF EXISTS dj_tag_refs;
DROP TABLE IF EXISTS dj_session;
DROP INDEX IF EXISTS dj_recipes_active_name_idx;
DROP TABLE IF EXISTS dj_recipes;
