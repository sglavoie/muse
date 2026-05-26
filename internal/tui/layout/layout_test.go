package layout

import "testing"

func TestVisibleRows_Normal(t *testing.T) {
	got := VisibleRows(20, 3)
	if got != 17 {
		t.Errorf("expected 17, got %d", got)
	}
}

func TestVisibleRows_ZeroHeight(t *testing.T) {
	got := VisibleRows(0, 3)
	if got != 50 {
		t.Errorf("expected default 50 for zero height, got %d", got)
	}
}

func TestVisibleRows_NegativeHeight(t *testing.T) {
	got := VisibleRows(-5, 3)
	if got != 50 {
		t.Errorf("expected default 50 for negative height, got %d", got)
	}
}

func TestVisibleRows_ReserveExceedsHeight(t *testing.T) {
	got := VisibleRows(2, 10)
	if got != 0 {
		t.Errorf("expected 0 rows when reserve exceeds height, got %d", got)
	}
}

func TestVisibleRows_ZeroReserve(t *testing.T) {
	got := VisibleRows(15, 0)
	if got != 15 {
		t.Errorf("expected 15, got %d", got)
	}
}

func TestVisibleRows_HeightEqualsReserve(t *testing.T) {
	got := VisibleRows(5, 5)
	if got != 0 {
		t.Errorf("expected 0 rows when height equals reserve, got %d", got)
	}
}

func TestVisibleRows_NegativeReserveClampedToZero(t *testing.T) {
	got := VisibleRows(10, -3)
	if got != 10 {
		t.Errorf("expected negative reserve to be ignored, got %d", got)
	}
}

func TestVisibleWindow_Normal(t *testing.T) {
	start, end := VisibleWindow(2, 10, 4)
	if start != 2 || end != 6 {
		t.Errorf("expected [2,6), got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_ClampsNegativeScroll(t *testing.T) {
	start, end := VisibleWindow(-5, 10, 4)
	if start != 0 || end != 4 {
		t.Errorf("expected [0,4), got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_ClampsScrollPastEnd(t *testing.T) {
	start, end := VisibleWindow(50, 10, 4)
	if start != 6 || end != 10 {
		t.Errorf("expected [6,10), got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_MaxRowsLargerThanItems(t *testing.T) {
	start, end := VisibleWindow(2, 3, 10)
	if start != 0 || end != 3 {
		t.Errorf("expected [0,3), got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_PreventsIntOverflowOnEnd(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	start, end := VisibleWindow(1, 5, maxInt)
	if start != 0 || end != 5 {
		t.Errorf("expected [0,5), got [%d,%d)", start, end)
	}
}
