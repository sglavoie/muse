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
	if got != 1 {
		t.Errorf("expected minimum 1, got %d", got)
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
	if got != 1 {
		t.Errorf("expected minimum 1 when height equals reserve, got %d", got)
	}
}
