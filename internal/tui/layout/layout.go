package layout

// VisibleRows returns the maximum number of content rows that fit on screen
// given the total height and the number of lines reserved for chrome (headers,
// margins, etc.). When height is zero (before the first WindowSizeMsg) a safe
// default of 50 is returned.
func VisibleRows(height, reserve int) int {
	if reserve < 0 {
		reserve = 0
	}
	if height > 0 {
		return max(1, height-reserve)
	}
	return 50
}
