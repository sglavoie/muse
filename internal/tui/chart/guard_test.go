package chart

import (
	"math"
	"testing"
)

func TestSafeFloat(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"NaN", math.NaN(), 0},
		{"positive infinity", math.Inf(1), 0},
		{"negative infinity", math.Inf(-1), 0},
		{"negative is preserved", -3.5, -3.5},
		{"zero", 0, 0},
		{"finite", 12.25, 12.25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SafeFloat(c.in); got != c.want {
				t.Fatalf("SafeFloat(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSafeMagnitude(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"NaN", math.NaN(), 0},
		{"negative infinity", math.Inf(-1), 0},
		{"positive infinity", math.Inf(1), 0},
		{"negative clamps to zero", -7, 0},
		{"positive", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SafeMagnitude(c.in); got != c.want {
				t.Fatalf("SafeMagnitude(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSafeFraction(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"NaN", math.NaN(), 0},
		{"positive infinity is missing data not a full bar", math.Inf(1), 0},
		{"negative infinity", math.Inf(-1), 0},
		{"below range", -0.5, 0},
		{"above range", 1.5, 1},
		{"in range", 0.615, 0.615},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SafeFraction(c.in); got != c.want {
				t.Fatalf("SafeFraction(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestSafeWidth(t *testing.T) {
	for _, c := range []struct{ in, want int }{{-5, 0}, {0, 0}, {7, 7}} {
		if got := SafeWidth(c.in); got != c.want {
			t.Fatalf("SafeWidth(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestSafeRepeat(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"negative count", "a", -1, ""},
		{"zero count", "a", 0, ""},
		{"positive count", "ab", 3, "ababab"},
		{"empty string", "", 5, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SafeRepeat(c.s, c.n); got != c.want {
				t.Fatalf("SafeRepeat(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
			}
		})
	}
}

func TestSafeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"clean text is untouched", "Toumani Diabaté", "Toumani Diabaté"},
		{"tab becomes a space", "a\tb", "a b"},
		{"newline is dropped", "a\nb", "ab"},
		{"carriage return is dropped", "a\r\nb", "ab"},
		{"escape is dropped", "a\x1b[31mb", "a[31mb"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SafeText(c.in); got != c.want {
				t.Fatalf("SafeText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
