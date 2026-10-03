package library

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestEmptyLibraryModeActions(t *testing.T) {
	for _, tc := range []struct {
		mode     LibraryMode
		want     string
		unwanted string
	}{
		{LibraryModeAll, "Library is empty", "No listening"},
		{LibraryModeRecent, "No recently added tracks", "Library is empty"},
		{LibraryModeHistory, "No listening history yet", "rescan folders"},
	} {
		v := View{Mode: tc.mode, Width: 100, Height: 25}
		got := v.Render(styles.NewStyles(true), nil, nil, nil, "", "")
		if !strings.Contains(got, tc.want) || strings.Contains(got, tc.unwanted) {
			t.Fatalf("mode %d: %s", tc.mode, got)
		}
	}
}
