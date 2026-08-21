package metaexport

import (
	"errors"
	"strings"
	"testing"
)

func FuzzFieldRoundTrip(f *testing.F) {
	seeds := []string{
		"", "plain", "tab\there", "line\nbreak", "carriage\rreturn",
		`already\tescaped`, `back\slash`, "\\", "🎺 Miles", "  padded  ",
		"\t\n\r\\", "Rock/Pop, Jazz;Funk",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		esc := escapeField(s)
		if strings.ContainsAny(esc, "\t\n\r") {
			t.Fatalf("escapeField(%q) = %q, still contains a raw TAB, LF or CR", s, esc)
		}
		got, err := unescapeField(esc)
		if err != nil {
			t.Fatalf("unescapeField(escapeField(%q)) failed: %v", s, err)
		}
		if got != s {
			t.Fatalf("round trip of %q gave %q", s, got)
		}
	})
}

func TestUnescapeFieldRejectsUnknownEscape(t *testing.T) {
	for _, in := range []string{`bad\x`, `trailing\`, `\0`, `mid\ dle`} {
		if _, err := unescapeField(in); !errors.Is(err, ErrBadEscape) {
			t.Fatalf("unescapeField(%q) err = %v, want ErrBadEscape", in, err)
		}
	}
}

func TestCell(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Miles Davis", want: "Miles Davis"},
		{name: "empty", in: "", want: ""},
		{name: "spaces trimmed", in: "  Miles Davis \t", want: "Miles Davis"},
		{name: "nbsp and zero width trimmed", in: "\u00a0\u200bMiles\u200d\ufeff", want: "Miles"},
		{name: "straight double quotes unwrapped", in: `"Kind of Blue"`, want: "Kind of Blue"},
		{name: "single quotes unwrapped", in: "'Kind of Blue'", want: "Kind of Blue"},
		{name: "curly quotes unwrapped", in: "“Kind of Blue”", want: "Kind of Blue"},
		{name: "padding outside quotes", in: "  \"Kind of Blue\" ", want: "Kind of Blue"},
		{name: "asymmetric quotes kept", in: `"Kind of Blue`, want: `"Kind of Blue`},
		{name: "inner quotes kept", in: `He said "no"`, want: `He said "no"`},
		{name: "lone quote kept", in: `"`, want: `"`},
		{name: "tab inside keeps quotes", in: "\"a\tb\"", want: "\"a\tb\""},
		{name: "one unwrap only", in: `""nested""`, want: `"nested"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Cell(tt.in); got != tt.want {
				t.Fatalf("Cell(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePosition(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
		ok   bool
	}{
		{name: "empty is unknown", in: "", want: 0, ok: true},
		{name: "dash is unknown", in: "-", want: 0, ok: true},
		{name: "em dash is unknown", in: "—", want: 0, ok: true},
		{name: "plain number", in: "7", want: 7, ok: true},
		{name: "zero", in: "0", want: 0, ok: true},
		{name: "fraction keeps numerator", in: "3/12", want: 3, ok: true},
		{name: "padded fraction", in: " 3 / 12 ", want: 3, ok: true},
		{name: "leading zeros", in: "03", want: 3, ok: true},
		{name: "non numeric fails", in: "three", want: 0, ok: false},
		{name: "negative fails", in: "-2", want: 0, ok: false},
		{name: "empty numerator fails", in: "/12", want: 0, ok: false},
		{name: "trailing junk fails", in: "3a", want: 0, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParsePosition(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("ParsePosition(%q) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
