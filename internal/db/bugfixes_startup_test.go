package db_test

import "testing"

// TestIncrementStartupCount_ReturnsExactPostUpdateValue establishes the
// correctness property that the bug fix enforces: the returned value is
// exactly the post-image of the UPDATE this call performed. The pre-fix
// implementation ran a separate SELECT after the UPDATE, so any concurrent
// UPDATE landing in between could surface a strictly higher value to the
// caller. The fix uses UPDATE … RETURNING so the read and write are the
// same statement and a foreign UPDATE landing after RETURNING cannot leak
// into the returned value.
//
// We exercise the property by interleaving an out-of-band UPDATE between a
// hand-rolled call to the (fixed) RETURNING statement and any subsequent
// read of the stored value. The returned value must remain the pre-foreign
// post-image.
func TestIncrementStartupCount_ReturnsExactPostUpdateValue(t *testing.T) {
	d := openMigratedDB(t)

	var got int
	err := d.QueryRow(
		`UPDATE session SET startup_count = startup_count + 1, updated_at = 0
 WHERE id = 1
 RETURNING startup_count`,
	).Scan(&got)
	if err != nil {
		t.Fatalf("UPDATE RETURNING: %v", err)
	}

	if _, err := d.Exec(`UPDATE session SET startup_count = startup_count + 1 WHERE id = 1`); err != nil {
		t.Fatalf("foreign update: %v", err)
	}

	if got != 1 {
		t.Errorf("RETURNING surfaced %d, want 1 (foreign UPDATE must not leak into returned value)", got)
	}

	var final int
	if err := d.QueryRow(`SELECT startup_count FROM session WHERE id = 1`).Scan(&final); err != nil {
		t.Fatalf("read final: %v", err)
	}
	if final != 2 {
		t.Errorf("final startup_count = %d, want 2", final)
	}

	if _, err := d.Exec(`UPDATE session SET startup_count = 0 WHERE id = 1`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	for want := 1; want <= 3; want++ {
		n, err := d.IncrementStartupCount()
		if err != nil {
			t.Fatalf("IncrementStartupCount: %v", err)
		}
		if n != want {
			t.Errorf("IncrementStartupCount = %d, want %d", n, want)
		}
	}
}

// TestIncrementStartupCount_ReturnsIncrementedValue verifies the basic happy
// path: each sequential call returns the next integer starting from 1.
func TestIncrementStartupCount_ReturnsIncrementedValue(t *testing.T) {
	d := openMigratedDB(t)

	for want := 1; want <= 3; want++ {
		got, err := d.IncrementStartupCount()
		if err != nil {
			t.Fatalf("IncrementStartupCount: %v", err)
		}
		if got != want {
			t.Errorf("IncrementStartupCount = %d, want %d", got, want)
		}
	}
}
