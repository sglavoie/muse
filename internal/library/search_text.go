package library

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// FoldSearchText folds case and Latin accents for matching only. Stored metadata
// and paths remain untouched. Non-Latin vowel marks and emoji selectors survive.
func FoldSearchText(s string) string {
	latin := false
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			if latin {
				return -1
			}
		} else {
			latin = unicode.Is(unicode.Latin, r)
		}
		return unicode.ToLower(r)
	}, norm.NFD.String(s))
}

func SearchContains(value, query string) bool {
	return query == "" || strings.Contains(FoldSearchText(value), FoldSearchText(query))
}

// Each folded rune retains its source rune index for accurate highlighting,
// including composed accents and decomposed sequences from macOS metadata.
func foldSearch(s string) (string, []int) {
	var folded strings.Builder
	positions := make([]int, 0, len(s))
	latin := false
	for index, original := range []rune(s) {
		if original < 128 {
			folded.WriteRune(unicode.ToLower(original))
			positions = append(positions, index)
			latin = unicode.Is(unicode.Latin, original)
			continue
		}
		for _, r := range norm.NFD.String(string(original)) {
			if unicode.Is(unicode.Mn, r) {
				if latin {
					continue
				}
			} else {
				latin = unicode.Is(unicode.Latin, r)
			}
			folded.WriteRune(unicode.ToLower(r))
			positions = append(positions, index)
		}
	}
	return folded.String(), positions
}
