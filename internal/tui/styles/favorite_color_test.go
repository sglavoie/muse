package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestFavoriteColorAvoidsAccentHue(t *testing.T) {
	rose, gold := lipgloss.Color(colorFgFavoriteDark), lipgloss.Color(colorFgFavoriteAltDark)
	if got := favoriteColor(lipgloss.Color(colorAccentPrimaryDark), rose, gold); Hex(got) != Hex(rose) {
		t.Errorf("blue accent: favorite = %s, want rose", Hex(got))
	}
	if got := favoriteColor(lipgloss.Color("#d0508a"), rose, gold); Hex(got) != Hex(gold) {
		t.Errorf("pink accent: favorite = %s, want gold", Hex(got))
	}
	if got := favoriteColor(lipgloss.Color("#c0a0b0"), rose, gold); Hex(got) != Hex(rose) {
		t.Errorf("greyish accent: favorite = %s, want rose", Hex(got))
	}
}
