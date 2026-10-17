-- +goose Up
-- Redirects outlive purged source tags, including drafts saved after a merge.
CREATE TABLE tag_redirects (
    source_id INTEGER PRIMARY KEY,
    target_id INTEGER NOT NULL
);

-- Walk every nested tag rule, including soft-deleted playlists. Unresolved
-- names get an explicit missing identity rather than binding a future namesake.
WITH RECURSIVE
refs AS (
    SELECT p.id, j.path || '.tag_id' AS path,
           COALESCE((SELECT id FROM tags WHERE name = json_extract(p.query, j.path || '.value')), -1) AS tag_id,
           ROW_NUMBER() OVER (PARTITION BY p.id ORDER BY j.id) AS n
    FROM playlists p, json_tree(CASE WHEN json_valid(p.query) THEN p.query ELSE '{}' END) j
    WHERE j.key = 'field' AND j.value = 'has_tag'
      AND json_extract(p.query, j.path || '.tag_id') IS NULL
),
edits(id, n, doc) AS (
    SELECT id, 0, query FROM playlists WHERE id IN (SELECT id FROM refs)
    UNION ALL
    SELECT e.id, r.n, json_set(e.doc, r.path, r.tag_id)
    FROM edits e JOIN refs r ON r.id = e.id AND r.n = e.n + 1
)
UPDATE playlists SET query = (SELECT doc FROM edits WHERE edits.id = playlists.id ORDER BY n DESC LIMIT 1)
WHERE id IN (SELECT id FROM refs);

-- +goose Down
-- Restore current names before removing identities for older application versions.
WITH RECURSIVE
refs AS (
    SELECT p.id, j.path AS path,
           COALESCE((SELECT name FROM tags WHERE id = COALESCE(
               (SELECT target_id FROM tag_redirects WHERE source_id = j.value), j.value)),
               json_extract(p.query, j.path || '.value')) AS name,
           ROW_NUMBER() OVER (PARTITION BY p.id ORDER BY j.id) AS n
    FROM playlists p, json_tree(CASE WHEN json_valid(p.query) THEN p.query ELSE '{}' END) j
    WHERE j.key = 'tag_id'
),
edits(id, n, doc) AS (
    SELECT id, 0, query FROM playlists WHERE id IN (SELECT id FROM refs)
    UNION ALL
    SELECT e.id, r.n, json_remove(json_set(e.doc, r.path || '.value', r.name), r.path || '.tag_id')
    FROM edits e JOIN refs r ON r.id = e.id AND r.n = e.n + 1
)
UPDATE playlists SET query = (SELECT doc FROM edits WHERE edits.id = playlists.id ORDER BY n DESC LIMIT 1)
WHERE id IN (SELECT id FROM refs);
DROP TABLE tag_redirects;
