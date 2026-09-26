package mixes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestOpenMixHeadingNamesTheMix guards that an open mix's track list is
// headed by the mix, "Mixes — <name>", instead of the embedded library
// view's fallback "Library".
func TestOpenMixHeadingNamesTheMix(t *testing.T) {
	v := MixesView{
		Mixes: []recommendation.MixInfo{{ID: db.MixRediscover, Name: "Rediscover"}},
		CachedMixes: map[db.MixID][]library.Track{
			db.MixRediscover: {{Hash: "h1", Title: "Song", Artist: "A"}},
		},
	}
	v.SetSize(80, 10)
	v.OpenMix(db.MixRediscover)

	out := ansi.Strip(v.Render(styles.NewStyles(true), nil, nil, nil, "", ""))
	first, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(first, "Mixes — Rediscover") {
		t.Errorf("expected the strip to name the mix, got %q", first)
	}
}
