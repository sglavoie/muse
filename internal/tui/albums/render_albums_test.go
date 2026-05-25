package albums

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestRenderAlbumsEmpty(t *testing.T) {
	v := View{Level: AlbumLevel}
	out := v.Render()
	if !strings.Contains(out, "No albums") {
		t.Fatalf("expected albums empty hint in output, got:\n%s", out)
	}
}

func TestRenderAlbumsShowsNames(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Width:  80,
		Albums: []string{"Alpha", "Beta", "Gamma"},
	}

	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsWithUnsetWidthShowsNames(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"Alpha", "Beta"},
	}

	out := v.Render()
	for _, want := range []string{"Alpha", "Beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q with unset width, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsClampsInvalidScrollOffset(t *testing.T) {
	tests := []struct {
		name         string
		scrollOffset int
	}{
		{name: "negative", scrollOffset: -1},
		{name: "past end", scrollOffset: 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := View{
				Level:        AlbumLevel,
				Width:        80,
				Height:       20,
				Albums:       []string{"Alpha", "Beta", "Gamma"},
				ScrollOffset: tt.scrollOffset,
			}

			out := v.Render()
			for _, want := range []string{"Alpha", "Beta", "Gamma"} {
				if !strings.Contains(out, want) {
					t.Fatalf("expected %q after clamping scroll offset %d, got:\n%s", want, tt.scrollOffset, out)
				}
			}
		})
	}
}

func TestRenderAlbumsShowsCounts(t *testing.T) {
	v := View{
		Level:       AlbumLevel,
		Width:       80,
		Albums:      []string{"Alpha", "Beta"},
		AlbumCounts: map[string]int{"Alpha": 12, "Beta": 4},
	}

	out := v.Render()
	for _, want := range []string{"Alpha", "Beta", "12", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestRenderAlbumsCountDropsAtNarrowWidth(t *testing.T) {
	v := View{
		Level:       AlbumLevel,
		Width:       10,
		Albums:      []string{"Alpha"},
		AlbumCounts: map[string]int{"Alpha": 99},
	}

	out := v.Render()
	if strings.Contains(out, "99") {
		t.Errorf("expected count to drop at narrow width, got:\n%s", out)
	}
	if !strings.Contains(out, "Alpha") {
		t.Errorf("expected label visible at narrow width, got:\n%s", out)
	}
}

func TestRenderWithStyles(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Width:  80,
		Height: 20,
		Albums: []string{"Alpha"},
		Styles: styles.NewStyles(true),
	}

	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render output")
	}
}

func TestFilterNoMatch(t *testing.T) {
	v := View{
		Level:      AlbumLevel,
		Width:      80,
		Albums:     []string{"Alpha"},
		FilterText: "zzz",
	}

	out := v.Render()
	if !strings.Contains(out, "no matches") {
		t.Errorf("expected '(no matches)', got:\n%s", out)
	}
}
