package insights

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestComputeEmptyInputIsZeroSnapshot(t *testing.T) {
	got := Compute(Input{})
	if !reflect.DeepEqual(got, Snapshot{}) {
		t.Errorf("Compute(Input{}) = %+v, want the zero Snapshot", got)
	}
}

func TestComputeNilLocationDoesNotPanic(t *testing.T) {
	loc := testLoc(t)
	in := Input{
		Now:   time.Unix(at(t, loc, "2026-08-20 12:00:00"), 0),
		Since: 0,
		Plays: []PlayRow{play(at(t, loc, "2026-08-20 11:00:00"), 600, "A")},
		Timeline: []TimelineRow{
			{StartAt: at(t, loc, "2026-08-20 10:50:00"), ListenedSecs: 600, FirstEver: true},
		},
		Coverage: []CoverageRow{{Hash: "a", Artist: "A", DurationSecs: 300, PlayCount: 1}},
	}
	got := Compute(in) // Loc deliberately nil
	if got.TotalPlays != 1 {
		t.Errorf("TotalPlays = %d, want 1", got.TotalPlays)
	}
}

func TestComputeTotalsAndSpan(t *testing.T) {
	loc := testLoc(t)
	in := Input{
		Loc:         loc,
		Now:         time.Unix(at(t, loc, "2026-08-20 18:00:00"), 0),
		WindowLabel: "All Time",
		Since:       0,
		Plays: []PlayRow{
			play(at(t, loc, "2026-03-27 10:10:00"), 600, "A"),
			play(at(t, loc, "2026-08-20 09:30:00"), 1800, "B"),
		},
	}

	got := Compute(in)

	if got.WindowLabel != "All Time" {
		t.Errorf("WindowLabel = %q, want %q", got.WindowLabel, "All Time")
	}
	if got.TotalPlays != 2 {
		t.Errorf("TotalPlays = %d, want 2", got.TotalPlays)
	}
	if got.TotalSecs != 2400 {
		t.Errorf("TotalSecs = %d, want 2400", got.TotalSecs)
	}
	// 2026-03-27 through 2026-08-20 inclusive, the real library's span.
	if got.SpanDays != 147 {
		t.Errorf("SpanDays = %d, want 147", got.SpanDays)
	}
}

func TestComputeNegativeListenedSecsAreClamped(t *testing.T) {
	loc := testLoc(t)
	in := Input{
		Loc:   loc,
		Now:   time.Unix(at(t, loc, "2026-08-20 18:00:00"), 0),
		Plays: []PlayRow{{StartAt: at(t, loc, "2026-08-20 15:00:00"), ListenedSecs: -600, Artist: "A"}},
	}

	got := Compute(in)

	if got.TotalSecs != 0 {
		t.Errorf("TotalSecs = %d, want 0 for a negative listened_secs", got.TotalSecs)
	}
	if got.Rhythm.Hours[15].ListenedSecs != 0 {
		t.Errorf("hour 15 secs = %d, want 0", got.Rhythm.Hours[15].ListenedSecs)
	}
}

// TestPackageNeverReadsTheClock enforces the injection contract at the source
// level: a single time.Now or time.Local anywhere in the package would make
// every panel non-reproducible and the tests below host-dependent.
func TestPackageNeverReadsTheClock(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	banned := map[string]bool{"Now": true, "Local": true}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("ParseFile(%s): %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || !banned[sel.Sel.Name] {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "time" {
				t.Errorf("%s uses time.%s; Now and Loc must come from Input", name, sel.Sel.Name)
			}
			return true
		})
	}
}
