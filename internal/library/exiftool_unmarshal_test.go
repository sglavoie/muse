package library

import (
	"encoding/json"
	"math"
	"testing"
)

func TestExifRecord_UnmarshalJSON_NumericDuration(t *testing.T) {
	input := `{"SourceFile":"/a.flac","Artist":"Bob","Title":"Song","Album":"LP","Genre":"Rock","Duration":123.45}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.SourceFile != "/a.flac" {
		t.Errorf("SourceFile: want %q, got %q", "/a.flac", r.SourceFile)
	}
	if r.Artist != "Bob" {
		t.Errorf("Artist: want %q, got %q", "Bob", r.Artist)
	}
	if r.Duration != 123.45 {
		t.Errorf("Duration: want 123.45, got %f", r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_StringDurationHHMMSS(t *testing.T) {
	input := `{"SourceFile":"/b.wav","Duration":"1:02:03"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	want := 1*3600.0 + 2*60.0 + 3.0
	if r.Duration != want {
		t.Errorf("Duration: want %f, got %f", want, r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_StringDurationMMSS(t *testing.T) {
	input := `{"SourceFile":"/c.wav","Duration":"3:45"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	want := 3*60.0 + 45.0
	if r.Duration != want {
		t.Errorf("Duration: want %f, got %f", want, r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_StringDurationWithMillis(t *testing.T) {
	input := `{"SourceFile":"/d.wav","Duration":"0:03:45.500"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	want := 3*60.0 + 45.5
	if math.Abs(r.Duration-want) > 0.001 {
		t.Errorf("Duration: want %f, got %f", want, r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_StringDurationPlainNumeric(t *testing.T) {
	input := `{"SourceFile":"/e.wav","Duration":"99.5"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Duration != 99.5 {
		t.Errorf("Duration: want 99.5, got %f", r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_StringDurationUnparseable(t *testing.T) {
	input := `{"SourceFile":"/f.wav","Duration":"unknown"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Duration != 0 {
		t.Errorf("Duration: want 0, got %f", r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_ArrayGenre(t *testing.T) {
	input := `{"SourceFile":"/g.flac","Genre":["Rock","Pop"]}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Genre != "Rock; Pop" {
		t.Errorf("Genre: want %q, got %q", "Rock; Pop", r.Genre)
	}
}

func TestExifRecord_UnmarshalJSON_ArrayArtist(t *testing.T) {
	input := `{"SourceFile":"/h.flac","Artist":["Alice","Bob"]}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Artist != "Alice; Bob" {
		t.Errorf("Artist: want %q, got %q", "Alice; Bob", r.Artist)
	}
}

func TestExifRecord_UnmarshalJSON_MissingFields(t *testing.T) {
	input := `{"SourceFile":"/i.flac"}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Artist != "" {
		t.Errorf("Artist: want empty, got %q", r.Artist)
	}
	if r.Duration != 0 {
		t.Errorf("Duration: want 0, got %f", r.Duration)
	}
}

func TestExifRecord_UnmarshalJSON_NumericArtistField(t *testing.T) {
	// Some files have numeric track/artist fields.
	input := `{"SourceFile":"/j.flac","Artist":42}`
	var r exifRecord
	if err := json.Unmarshal([]byte(input), &r); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if r.Artist != "42" {
		t.Errorf("Artist: want %q, got %q", "42", r.Artist)
	}
}

func TestExifRecord_UnmarshalJSON_BatchWithMixedTypes(t *testing.T) {
	// Simulate a batch where one record has Duration as string and another
	// has it as float64. Both should unmarshal without error.
	input := `[
		{"SourceFile":"/a.flac","Duration":120.5,"Genre":"Rock"},
		{"SourceFile":"/b.wav","Duration":"3:45","Genre":["Pop","Jazz"]},
		{"SourceFile":"/c.mp3","Duration":"unknown","Artist":["X","Y"]}
	]`
	var records []exifRecord
	if err := json.Unmarshal([]byte(input), &records); err != nil {
		t.Fatalf("batch unmarshal error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	if records[0].Duration != 120.5 {
		t.Errorf("[0] Duration: want 120.5, got %f", records[0].Duration)
	}
	if records[0].Genre != "Rock" {
		t.Errorf("[0] Genre: want %q, got %q", "Rock", records[0].Genre)
	}
	want1 := 3*60.0 + 45.0
	if records[1].Duration != want1 {
		t.Errorf("[1] Duration: want %f, got %f", want1, records[1].Duration)
	}
	if records[1].Genre != "Pop; Jazz" {
		t.Errorf("[1] Genre: want %q, got %q", "Pop; Jazz", records[1].Genre)
	}
	if records[2].Duration != 0 {
		t.Errorf("[2] Duration: want 0, got %f", records[2].Duration)
	}
	if records[2].Artist != "X; Y" {
		t.Errorf("[2] Artist: want %q, got %q", "X; Y", records[2].Artist)
	}
}

func TestParseDurationString(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"  ", 0},
		{"0:00:00", 0},
		{"0:01:30", 90},
		{"1:00:00", 3600},
		{"1:30:00", 5400},
		{"3:45", 225},
		{"0:30", 30},
		{"123.45", 123.45},
		{"not a number", 0},
		{"0:03:45.500", 225.5},
	}
	for _, tt := range tests {
		got := parseDurationString(tt.input)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("parseDurationString(%q): want %f, got %f", tt.input, tt.want, got)
		}
	}
}

// TestCoerceFloat_NegativeNaNInfNormalized verifies that pathological
// numeric durations emitted by exiftool (negative, NaN, +Inf, -Inf) are
// normalised to 0 so they cannot corrupt downstream duration arithmetic.
func TestCoerceFloat_NegativeNaNInfNormalized(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"negative", `-1.5`},
		{"negative string", `"-1.5"`},
		{"negative HHMMSS", `"-1:00:00"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceFloat([]byte(c.in))
			if got != 0 {
				t.Errorf("coerceFloat(%s): want 0, got %v", c.in, got)
			}
		})
	}
}

// TestSanitizeDuration verifies the float guard rejects non-finite and
// negative inputs.
func TestSanitizeDuration(t *testing.T) {
	if got := sanitizeDuration(math.NaN()); got != 0 {
		t.Errorf("NaN: want 0, got %v", got)
	}
	if got := sanitizeDuration(math.Inf(1)); got != 0 {
		t.Errorf("+Inf: want 0, got %v", got)
	}
	if got := sanitizeDuration(math.Inf(-1)); got != 0 {
		t.Errorf("-Inf: want 0, got %v", got)
	}
	if got := sanitizeDuration(-0.0001); got != 0 {
		t.Errorf("negative: want 0, got %v", got)
	}
	if got := sanitizeDuration(0); got != 0 {
		t.Errorf("zero: want 0, got %v", got)
	}
	if got := sanitizeDuration(123.4); got != 123.4 {
		t.Errorf("positive: want 123.4, got %v", got)
	}
}

// TestCoerceString_HeterogeneousArrayPreservesNonEmpty verifies that a mixed
// JSON array (string + number + null) is joined element-by-element instead
// of being discarded entirely.
func TestCoerceString_HeterogeneousArrayPreservesNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"mixed string and number", `["Rock", 42, "Pop"]`, "Rock; 42; Pop"},
		{"array with null", `["A", null, "B"]`, "A; B"},
		{"array with bool", `["A", true]`, "A; true"},
		{"all strings still works", `["Rock", "Jazz"]`, "Rock; Jazz"},
		{"empty array", `[]`, ""},
		{"single number", `42`, "42"},
		{"plain string", `"hello"`, "hello"},
		{"bool", `true`, "true"},
		{"null", `null`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := coerceString([]byte(c.in))
			if got != c.want {
				t.Errorf("coerceString(%s): got %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestParseTrackNumber_RejectsNegative verifies that negative track or disc
// numbers are normalised to 0 (the unknown sentinel) rather than propagating
// into sorting/display.
func TestParseTrackNumber_RejectsNegative(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"-3", 0},
		{"-1/12", 0},
		{"3", 3},
		{"3/12", 3},
		{"", 0},
		{"  ", 0},
		{"abc", 0},
		{"0", 0},
	}
	for _, c := range cases {
		got := parseTrackNumber(c.in)
		if got != c.want {
			t.Errorf("parseTrackNumber(%q): got %d, want %d", c.in, got, c.want)
		}
	}
}
