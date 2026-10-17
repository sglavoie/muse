package db

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
)

type smartTagReader interface {
	QueryRow(string, ...any) *sql.Row
}

// Resolve on a copy: callers may still own the query in an editor or cached
// playlist list. Missing dependencies fail the whole query, including NOT/OR,
// so deletion cannot silently broaden a playlist.
func resolveSmartTags(reader smartTagReader, rule SmartRule) (SmartRule, error) {
	rule.Rules = slices.Clone(rule.Rules)
	for i, child := range rule.Rules {
		resolved, err := resolveSmartTags(reader, child)
		if err != nil {
			return rule, err
		}
		rule.Rules[i] = resolved
	}
	if rule.Field != FieldTag {
		return rule, nil
	}
	var row *sql.Row
	if rule.TagID == 0 {
		row = reader.QueryRow(`SELECT id, name FROM tags WHERE name = ? AND deleted_at IS NULL`, rule.Value)
	} else {
		row = reader.QueryRow(`SELECT id, name FROM tags WHERE id = COALESCE(
			(SELECT target_id FROM tag_redirects WHERE source_id = ?), ?) AND deleted_at IS NULL`, rule.TagID, rule.TagID)
	}
	if err := row.Scan(&rule.TagID, &rule.Value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rule, fmt.Errorf("tag %q is missing; edit the playlist's Tag rule to choose a replacement, or undo the tag deletion", rule.Value)
		}
		return rule, fmt.Errorf("resolve smart playlist tag: %w", err)
	}
	return rule, nil
}

func bindSmartQuery(reader smartTagReader, raw string) (string, error) {
	query, err := ParseSmartPlaylistQuery(raw)
	if err != nil {
		return "", err
	}
	query.Root, err = resolveSmartTags(reader, query.Root)
	if err != nil {
		return "", err
	}
	return MarshalSmartPlaylistQuery(query)
}

func redirectSmartTag(tx *sql.Tx, sourceID, targetID int64) error {
	if _, err := tx.Exec(`UPDATE tag_redirects SET target_id = ? WHERE target_id = ?`, targetID, sourceID); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO tag_redirects(source_id, target_id) VALUES (?, ?)
		ON CONFLICT(source_id) DO UPDATE SET target_id = excluded.target_id`, sourceID, targetID)
	return err
}
