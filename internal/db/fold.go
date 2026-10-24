package db

import (
	"database/sql/driver"
	"strings"
	"unicode"

	"modernc.org/sqlite"
)

// foldFunc is the SQL name of foldText. SQLite's NOCASE collation and LIKE only
// fold ASCII, so "björk" would miss "BJÖRK"; comparing folded values instead
// ignores case for every script, matching the in-memory library ordering.
const foldFunc = "muse_fold"

func init() {
	// Registration applies to every connection opened afterwards, and init runs
	// before any caller can reach Open.
	sqlite.MustRegisterDeterministicScalarFunction(foldFunc, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		switch v := args[0].(type) {
		case string:
			return foldText(v), nil
		case []byte:
			return foldText(string(v)), nil
		default:
			// NULL stays NULL and numbers have no case, so comparisons keep
			// SQLite's own semantics for them.
			return v, nil
		}
	})
}

// foldText lowercases s rune by rune with unicode.ToLower, the same folding the
// library index uses for case-insensitive ordering.
func foldText(s string) string {
	return strings.Map(unicode.ToLower, s)
}
