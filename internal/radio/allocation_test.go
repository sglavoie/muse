package radio

import "testing"

func TestAllocation_PresetTargetsAreWholeTracks(t *testing.T) {
	tests := []struct {
		preset Preset
		want   int
	}{
		{PresetFamiliar, 1},
		{PresetBalanced, 2},
		{PresetExplore, 4},
	}

	for _, tc := range tests {
		t.Run(string(tc.preset), func(t *testing.T) {
			pct, ok := tc.preset.DiscoveryPercent()
			if !ok {
				t.Fatalf("preset %q has no stock target", tc.preset)
			}
			target, next := Allocation{}.TargetFor(5, pct)
			if target != tc.want {
				t.Fatalf("target = %d, want %d", target, tc.want)
			}
			if !next.IsZero() {
				t.Fatalf("stock preset carried %v, want no carry", next.Carry())
			}
		})
	}
}

func TestAllocation_CarriesFractionAcrossBatches(t *testing.T) {
	// 30% of a five-track batch is 1.5 tracks. Rounding each batch on its own
	// would deliver 2 every time — an effective 40%.
	alloc := Allocation{}
	targets := make([]int, 0, 6)
	for range 6 {
		var target int
		target, alloc = alloc.TargetFor(5, 30)
		targets = append(targets, target)
	}

	want := []int{2, 1, 2, 1, 2, 1}
	total := 0
	for i, got := range targets {
		if got != want[i] {
			t.Fatalf("batch %d target = %d, want %d (all: %v)", i, got, want[i], targets)
		}
		total += got
	}
	if total*100 != 30*5*len(targets) {
		t.Fatalf("delivered %d discovery slots over %d batches, want %d", total, len(targets), 30*5*len(targets)/100)
	}
}

func TestAllocation_TenPercentCarriesOverFourBatches(t *testing.T) {
	alloc := Allocation{}
	total := 0
	for range 4 {
		var target int
		target, alloc = alloc.TargetFor(5, 10)
		total += target
	}
	if total != 2 {
		t.Fatalf("delivered %d discovery slots at 10%% over 4 batches, want 2", total)
	}
}

func TestAllocation_NotAdoptedUntilAccepted(t *testing.T) {
	// Planning the same batch repeatedly without adopting the returned state
	// must keep returning the same target: a preview cannot shift live
	// selection.
	alloc := Allocation{}
	for range 5 {
		target, next := alloc.TargetFor(5, 30)
		if target != 2 {
			t.Fatalf("unadopted allocation drifted to target %d", target)
		}
		if next.CarryUnits != -50 {
			t.Fatalf("next carry = %d, want -50", next.CarryUnits)
		}
	}
}

func TestAllocation_ResetClearsCarry(t *testing.T) {
	_, next := Allocation{}.TargetFor(5, 30)
	if next.IsZero() {
		t.Fatalf("expected a carry to reset")
	}
	if got := next.Reset(); !got.IsZero() {
		t.Fatalf("Reset() = %+v, want zero", got)
	}
}

func TestAllocation_CarryStaysBounded(t *testing.T) {
	// A shortage never reaches the allocation, so the only thing that can grow
	// the carry is rounding. Drive many batches across targets and batch sizes
	// and confirm the stored remainder stays within half a track.
	alloc := Allocation{}
	for _, pct := range []int{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100} {
		for _, size := range []int{1, 3, 5, 7} {
			for range 20 {
				var target int
				target, alloc = alloc.TargetFor(size, pct)
				if target < 0 || target > size {
					t.Fatalf("target %d out of range for batch size %d", target, size)
				}
				if alloc.CarryUnits > maxAllocationCarry || alloc.CarryUnits < -maxAllocationCarry {
					t.Fatalf("carry %d escaped the bound at pct=%d size=%d", alloc.CarryUnits, pct, size)
				}
			}
		}
	}
}

func TestAllocation_NormalizedClampsPersistedGarbage(t *testing.T) {
	tests := []struct {
		name  string
		carry int
		want  int
	}{
		{"far positive", 9_000, maxAllocationCarry},
		{"far negative", -9_000, -maxAllocationCarry},
		{"in range", 17, 17},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Allocation{CarryUnits: tc.carry}).Normalized(); got.CarryUnits != tc.want {
				t.Fatalf("Normalized() = %d, want %d", got.CarryUnits, tc.want)
			}
		})
	}
}

func TestAllocation_NonPositiveBatchTargetsNothing(t *testing.T) {
	target, next := Allocation{CarryUnits: 40}.TargetFor(0, 80)
	if target != 0 {
		t.Fatalf("target = %d, want 0", target)
	}
	if next.CarryUnits != 40 {
		t.Fatalf("carry = %d, want it preserved at 40", next.CarryUnits)
	}
}

func TestRoundAllocationUnits(t *testing.T) {
	tests := []struct {
		units int
		want  int
	}{
		{0, 0},
		{49, 0},
		{50, 1},
		{150, 2},
		{-49, 0},
		{-50, 0},
		{-51, -1},
		{-150, -1},
		{-151, -2},
	}

	for _, tc := range tests {
		if got := roundAllocationUnits(tc.units); got != tc.want {
			t.Fatalf("roundAllocationUnits(%d) = %d, want %d", tc.units, got, tc.want)
		}
	}
}
