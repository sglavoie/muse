package tui

import "github.com/sglavoie/muse/internal/db"

// stubFavoriteWriter records calls to AddFavorite, RemoveFavorite, and
// RemoveFavoritesByTypeAndValues for assertion in undo-favorite tests.
type stubFavoriteWriter struct {
	added   []struct{ typ, value string }
	removed []struct{ typ, value string }
}

func (s *stubFavoriteWriter) AddFavorite(typ, value string) error {
	s.added = append(s.added, struct{ typ, value string }{typ, value})
	return nil
}

func (s *stubFavoriteWriter) RemoveFavorite(typ, value string) error {
	s.removed = append(s.removed, struct{ typ, value string }{typ, value})
	return nil
}

func (s *stubFavoriteWriter) RemoveFavoritesByTypeAndValues(typ string, values []string) error {
	for _, v := range values {
		s.removed = append(s.removed, struct{ typ, value string }{typ, v})
	}
	return nil
}

// stubFavoriteReader returns canned favorites data for undo-favorite tests.
type stubFavoriteReader struct {
	favorites map[string][]string // typ -> values
}

func (s *stubFavoriteReader) ReadFavoritesByType(typ string) ([]db.FavoriteRow, error) {
	var result []db.FavoriteRow
	for i, v := range s.favorites[typ] {
		result = append(result, db.FavoriteRow{ID: int64(i + 1), Type: typ, Value: v})
	}
	return result, nil
}

func (s *stubFavoriteReader) ReadFavoriteCounts() (int, int, int, error) {
	return len(s.favorites["artist"]), len(s.favorites["album"]), len(s.favorites["track"]), nil
}

func (s *stubFavoriteReader) IsFavorite(typ, value string) (bool, error) {
	for _, v := range s.favorites[typ] {
		if v == value {
			return true, nil
		}
	}
	return false, nil
}
