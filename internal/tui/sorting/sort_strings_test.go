package sorting

import "testing"

func TestSortStrings(t *testing.T) {
	items := []string{"Charlie", "Alpha", "", "Bravo"}

	t.Run("sort asc", func(t *testing.T) {
		sorted := SortStrings(items, SortAsc)
		if len(sorted) != 4 {
			t.Fatalf("expected 4, got %d", len(sorted))
		}
		if sorted[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Charlie" {
			t.Errorf("third should be Charlie, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("sort desc", func(t *testing.T) {
		sorted := SortStrings(items, SortDesc)
		if sorted[0] != "Charlie" {
			t.Errorf("first should be Charlie, got %q", sorted[0])
		}
		if sorted[1] != "Bravo" {
			t.Errorf("second should be Bravo, got %q", sorted[1])
		}
		if sorted[2] != "Alpha" {
			t.Errorf("third should be Alpha, got %q", sorted[2])
		}
		if sorted[3] != "" {
			t.Errorf("last should be empty (unknown), got %q", sorted[3])
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		if SortStrings(nil, SortAsc) != nil {
			t.Error("expected nil")
		}
	})

	t.Run("original not modified", func(t *testing.T) {
		_ = SortStrings(items, SortAsc)
		if items[0] != "Charlie" {
			t.Error("original slice was modified")
		}
	})

	t.Run("sentinel unknown artist sorts last asc", func(t *testing.T) {
		items := []string{"Charlie", "(unknown artist)", "Alpha"}
		sorted := SortStrings(items, SortAsc)
		if sorted[0] != "Alpha" {
			t.Errorf("first should be Alpha, got %q", sorted[0])
		}
		if sorted[1] != "Charlie" {
			t.Errorf("second should be Charlie, got %q", sorted[1])
		}
		if sorted[2] != "(unknown artist)" {
			t.Errorf("last should be (unknown artist), got %q", sorted[2])
		}
	})

	t.Run("sentinel unknown album sorts last desc", func(t *testing.T) {
		items := []string{"(unknown album)", "Bravo", "Alpha"}
		sorted := SortStrings(items, SortDesc)
		if sorted[0] != "Bravo" {
			t.Errorf("first should be Bravo, got %q", sorted[0])
		}
		if sorted[1] != "Alpha" {
			t.Errorf("second should be Alpha, got %q", sorted[1])
		}
		if sorted[2] != "(unknown album)" {
			t.Errorf("last should be (unknown album), got %q", sorted[2])
		}
	})

	t.Run("sentinel unknown genre sorts last", func(t *testing.T) {
		items := []string{"(unknown genre)", "Rock", "Jazz"}
		sorted := SortStrings(items, SortAsc)
		if sorted[0] != "Jazz" {
			t.Errorf("first should be Jazz, got %q", sorted[0])
		}
		if sorted[1] != "Rock" {
			t.Errorf("second should be Rock, got %q", sorted[1])
		}
		if sorted[2] != "(unknown genre)" {
			t.Errorf("last should be (unknown genre), got %q", sorted[2])
		}
	})
}

func TestCompareStringsUnknownLast(t *testing.T) {
	tests := []struct {
		a, b string
		dir  SortDir
		want bool
	}{
		{"Alpha", "Bravo", SortAsc, true},
		{"Alpha", "Bravo", SortDesc, false},
		{"", "Alpha", SortAsc, false},
		{"", "Alpha", SortDesc, false},
		{"Alpha", "", SortAsc, true},
		{"Alpha", "", SortDesc, true},
		{"", "", SortAsc, false},
		{"Unknown Artist", "Alpha", SortAsc, false},
		{"Unknown Artist", "Alpha", SortDesc, false},
	}
	for _, tt := range tests {
		got := CompareStringsUnknownLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("CompareStringsUnknownLast(%q, %q, %d) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestCompareNumericZeroLast_Float64(t *testing.T) {
	tests := []struct {
		a, b float64
		dir  SortDir
		want bool
	}{
		{100, 200, SortAsc, true},
		{200, 100, SortAsc, false},
		{100, 200, SortDesc, false},
		{200, 100, SortDesc, true},
		{0, 100, SortAsc, false},  // zero after known
		{0, 100, SortDesc, false}, // zero after known
		{100, 0, SortAsc, true},   // known before zero
		{100, 0, SortDesc, true},  // known before zero
		{0, 0, SortAsc, false},    // stable
		{0, 0, SortDesc, false},   // stable
	}
	for _, tt := range tests {
		got := CompareNumericZeroLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("CompareNumericZeroLast(%v, %v, %v) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}

func TestCompareNumericZeroLast_Int64(t *testing.T) {
	tests := []struct {
		a, b int64
		dir  SortDir
		want bool
	}{
		{1000, 2000, SortAsc, true},
		{2000, 1000, SortAsc, false},
		{1000, 2000, SortDesc, false},
		{2000, 1000, SortDesc, true},
		{0, 1000, SortAsc, false},
		{0, 1000, SortDesc, false},
		{1000, 0, SortAsc, true},
		{1000, 0, SortDesc, true},
		{0, 0, SortAsc, false},
		{0, 0, SortDesc, false},
	}
	for _, tt := range tests {
		got := CompareNumericZeroLast(tt.a, tt.b, tt.dir)
		if got != tt.want {
			t.Errorf("CompareNumericZeroLast(%v, %v, %v) = %v, want %v",
				tt.a, tt.b, tt.dir, got, tt.want)
		}
	}
}
