package radio

// allocationScale is the fixed-point denominator for the carried remainder.
// Discovery targets are whole percentages of a whole batch, so hundredths of a
// track represent every reachable value exactly and the carry never drifts the
// way a float accumulator would over a long listening session.
const allocationScale = 100

// maxAllocationCarry bounds the stored remainder to half a track in either
// direction. A batch that cannot be filled with discovery tracks is a shortage,
// not a debt: the missing tracks are reported through Shortages and forgotten,
// so a sparse library can never accumulate an allocation the DJ tries to repay
// forever.
const maxAllocationCarry = allocationScale / 2

// Allocation is the discovery-percentage rounding state carried between
// accepted batches.
//
// A five-track batch at a 30% target wants 1.5 discovery tracks. Rounding each
// batch independently would deliver 2 every time — an effective 40%. Carrying
// the remainder alternates 2 and 1 so the long-run share matches the target the
// user set.
//
// The zero value is the state a fresh DJ session starts from. Callers reset to
// it when a new session captures a seed and when settings are applied, because
// a remainder earned under one target says nothing about the next. Only an
// accepted batch adopts PlanResult.NextAllocation; previews and rejected
// batches discard it, so looking at a preview never shifts live selection.
type Allocation struct {
	// CarryUnits is the signed remainder in hundredths of a track. Positive
	// means the DJ owes discovery tracks, negative means it ran ahead.
	CarryUnits int
}

// IsZero reports whether nothing is carried.
func (a Allocation) IsZero() bool { return a.CarryUnits == 0 }

// Carry returns the carried remainder in tracks.
func (a Allocation) Carry() float64 { return float64(a.CarryUnits) / allocationScale }

// Reset returns the allocation a new session or a settings change starts from.
func (a Allocation) Reset() Allocation { return Allocation{} }

// Normalized clamps a carry that was persisted by an older build, hand-edited,
// or produced under a different batch size back into the bounded range.
func (a Allocation) Normalized() Allocation {
	if a.CarryUnits > maxAllocationCarry {
		a.CarryUnits = maxAllocationCarry
	}
	if a.CarryUnits < -maxAllocationCarry {
		a.CarryUnits = -maxAllocationCarry
	}
	return a
}

// TargetFor returns how many of the next batchSize selections should be
// discovery tracks, and the allocation state to adopt if that batch is
// accepted. The returned target is always within [0, batchSize].
func (a Allocation) TargetFor(batchSize, discoveryPercent int) (int, Allocation) {
	normalized := a.Normalized()
	if batchSize <= 0 {
		return 0, normalized
	}

	desired := batchSize*ClampDiscoveryPercent(discoveryPercent) + normalized.CarryUnits
	target := roundAllocationUnits(desired)
	if target < 0 {
		target = 0
	}
	if target > batchSize {
		target = batchSize
	}
	return target, Allocation{CarryUnits: desired - target*allocationScale}.Normalized()
}

// roundAllocationUnits converts fixed-point units to whole tracks, rounding
// halves up (toward positive infinity) so the rounding direction matches
// DiscoveryTargetCount.
func roundAllocationUnits(units int) int {
	if units >= 0 {
		return (units + allocationScale/2) / allocationScale
	}
	return -((-units + allocationScale/2 - 1) / allocationScale)
}
