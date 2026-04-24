package db

import (
	"fmt"
	"strings"
)

// EvalSmartPlaylist evaluates a SmartPlaylistQuery against the tracks table and
// returns the matching tracks in the requested order. ID and Position fields in
// the returned rows are always 0 (smart playlists have no playlist_tracks rows).
func (d *DB) EvalSmartPlaylist(q SmartPlaylistQuery) ([]PlaylistTrackRow, error) {
	var sb strings.Builder
	var args []any

	sb.WriteString(`
		SELECT 0, 0, t.title, t.artist, t.album, t.duration_ms, t.hash, t.path
		FROM tracks t
		WHERE t.missing = 0`)

	if q.Root.Field != "" || q.Root.Conjunction != "" || len(q.Root.Rules) > 0 {
		sb.WriteString(" AND (")
		if err := buildWhere(&sb, &args, q.Root); err != nil {
			return nil, fmt.Errorf("db: eval smart playlist: %w", err)
		}
		sb.WriteString(")")
	}

	sb.WriteString(orderByClause(q))

	if q.MaxTracks > 0 {
		sb.WriteString(" LIMIT ?")
		args = append(args, q.MaxTracks)
	}

	rows, err := d.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("db: eval smart playlist: %w", err)
	}
	defer rows.Close()

	var result []PlaylistTrackRow
	for rows.Next() {
		var r PlaylistTrackRow
		if err := rows.Scan(&r.ID, &r.Position, &r.Title, &r.Artist, &r.Album, &r.DurationMs, &r.Hash, &r.Path); err != nil {
			return nil, fmt.Errorf("db: eval smart playlist row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// buildWhere recursively appends SQL fragments and bind parameters for the
// given SmartRule to sb/args.
func buildWhere(sb *strings.Builder, args *[]any, rule SmartRule) error {
	// group node
	if rule.Conjunction != "" || len(rule.Rules) > 0 {
		if len(rule.Rules) == 0 {
			// empty group — always true
			sb.WriteString("1")
			return nil
		}
		conj := " AND "
		if rule.Conjunction == ConjOr {
			conj = " OR "
		}
		sb.WriteString("(")
		for i, child := range rule.Rules {
			if i > 0 {
				sb.WriteString(conj)
			}
			if err := buildWhere(sb, args, child); err != nil {
				return err
			}
		}
		sb.WriteString(")")
		return nil
	}

	// leaf node
	return buildLeaf(sb, args, rule)
}

// buildLeaf writes the SQL fragment for a single leaf condition.
func buildLeaf(sb *strings.Builder, args *[]any, rule SmartRule) error {
	switch rule.Field {
	case FieldArtist, FieldTitle, FieldAlbum, FieldGenre, FieldPath:
		col := stringFieldCol(rule.Field)
		return buildStringOp(sb, args, col, rule.Operator, rule.Value)

	case FieldDuration:
		// duration_ms / 1000.0 gives seconds as a real; value is expected in seconds.
		// Using 1000.0 (not 1000) forces SQLite to perform real division so that
		// sub-second precision is preserved in comparisons.
		return buildNumericOp(sb, args, "t.duration_ms / 1000.0", rule.Operator, rule.Value)

	case FieldSize:
		return buildNumericOp(sb, args, "t.size_bytes", rule.Operator, rule.Value)

	case FieldPlayCount:
		return buildNumericOp(sb, args, "t.play_count", rule.Operator, rule.Value)

	case FieldDateAdded:
		// in_last is the only operator that makes sense for date_added.
		// Value is a number of days.
		if rule.Operator != OpInLast {
			return buildNumericOp(sb, args, "t.created_at", rule.Operator, rule.Value)
		}
		sb.WriteString("t.created_at >= unixepoch('now') - (? * 86400)")
		*args = append(*args, rule.Value)
		return nil

	case FieldLastPlayed:
		if rule.Operator == OpInLast {
			sb.WriteString("t.last_played_at >= unixepoch('now') - (? * 86400)")
			*args = append(*args, rule.Value)
			return nil
		}
		return buildNumericOp(sb, args, "t.last_played_at", rule.Operator, rule.Value)

	case FieldFavorite:
		// Treat "equals true" / "equals 1" as EXISTS, anything else as NOT EXISTS.
		exists := rule.Value == "true" || rule.Value == "1"
		if rule.Operator == OpNotEquals {
			exists = !exists
		}
		if exists {
			sb.WriteString("EXISTS (SELECT 1 FROM favorites f WHERE f.type='track' AND f.value=t.hash)")
		} else {
			sb.WriteString("NOT EXISTS (SELECT 1 FROM favorites f WHERE f.type='track' AND f.value=t.hash)")
		}
		return nil

	case FieldTag:
		// OpEquals: track has the tag with the given name.
		// OpNotEquals: track does NOT have the tag with the given name.
		const subq = "EXISTS (SELECT 1 FROM track_tags tt JOIN tags tg ON tt.tag_id = tg.id WHERE tt.track_hash = t.hash AND tg.name = ? AND tg.deleted_at IS NULL)"
		if rule.Operator == OpNotEquals {
			sb.WriteString("NOT " + subq)
		} else {
			sb.WriteString(subq)
		}
		*args = append(*args, rule.Value)
		return nil

	default:
		return fmt.Errorf("unknown field %q", rule.Field)
	}
}

func stringFieldCol(f RuleField) string {
	switch f {
	case FieldArtist:
		return "t.artist"
	case FieldTitle:
		return "t.title"
	case FieldAlbum:
		return "t.album"
	case FieldGenre:
		return "t.genre"
	case FieldPath:
		return "t.path"
	default:
		return "t." + string(f)
	}
}

func buildStringOp(sb *strings.Builder, args *[]any, col string, op RuleOperator, val string) error {
	switch op {
	case OpEquals:
		sb.WriteString(col + " = ?")
		*args = append(*args, val)
	case OpNotEquals:
		sb.WriteString(col + " != ?")
		*args = append(*args, val)
	case OpContains:
		sb.WriteString(col + " LIKE ? ESCAPE '\\'")
		*args = append(*args, "%"+escapeLike(val)+"%")
	case OpNotContains:
		sb.WriteString(col + " NOT LIKE ? ESCAPE '\\'")
		*args = append(*args, "%"+escapeLike(val)+"%")
	case OpStartsWith:
		sb.WriteString(col + " LIKE ? ESCAPE '\\'")
		*args = append(*args, escapeLike(val)+"%")
	case OpEndsWith:
		sb.WriteString(col + " LIKE ? ESCAPE '\\'")
		*args = append(*args, "%"+escapeLike(val))
	default:
		return fmt.Errorf("operator %q not valid for string field", op)
	}
	return nil
}

func buildNumericOp(sb *strings.Builder, args *[]any, col string, op RuleOperator, val string) error {
	// Use CAST(? AS NUMERIC) so that a Go string binding is treated as a
	// number, not TEXT. SQLite's type comparison rules place INTEGER below
	// TEXT, causing wrong results without the cast.
	const p = "CAST(? AS NUMERIC)"
	switch op {
	case OpEquals:
		sb.WriteString(col + " = " + p)
		*args = append(*args, val)
	case OpNotEquals:
		sb.WriteString(col + " != " + p)
		*args = append(*args, val)
	case OpGreaterThan:
		sb.WriteString(col + " > " + p)
		*args = append(*args, val)
	case OpLessThan:
		sb.WriteString(col + " < " + p)
		*args = append(*args, val)
	default:
		return fmt.Errorf("operator %q not valid for numeric field", op)
	}
	return nil
}

// escapeLike escapes LIKE special characters % and _.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// orderByClause returns the ORDER BY fragment for the query (empty if no sort
// fields are set).
func orderByClause(q SmartPlaylistQuery) string {
	var col string
	switch q.SortBy {
	case "artist":
		col = "t.artist COLLATE NOCASE"
	case "title":
		col = "t.title COLLATE NOCASE"
	case "album":
		col = "t.album COLLATE NOCASE"
	case "date_added":
		col = "t.created_at"
	case "duration":
		col = "t.duration_ms"
	default:
		return ""
	}
	dir := "ASC"
	if strings.EqualFold(q.SortDir, "desc") {
		dir = "DESC"
	}
	return " ORDER BY " + col + " " + dir
}
