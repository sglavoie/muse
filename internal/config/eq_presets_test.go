package config

import "testing"

func TestBuiltinEQPresets_Count(t *testing.T) {
	if len(BuiltinEQPresets) != 7 {
		t.Errorf("expected 7 built-in presets, got %d", len(BuiltinEQPresets))
	}
}

func TestBuiltinEQPresets_AllBuiltin(t *testing.T) {
	for _, p := range BuiltinEQPresets {
		if !p.Builtin {
			t.Errorf("preset %q has Builtin=false", p.Key)
		}
	}
}

func TestBuiltinEQPresets_FlatIsZero(t *testing.T) {
	flat := BuiltinEQPresets[0]
	if flat.Key != "flat" {
		t.Fatalf("first preset key = %q, want %q", flat.Key, "flat")
	}
	for i, v := range flat.Bands {
		if v != 0 {
			t.Errorf("flat band[%d] = %d, want 0", i, v)
		}
	}
}

func TestBuiltinEQPresets_UniqueKeys(t *testing.T) {
	seen := make(map[string]bool)
	for _, p := range BuiltinEQPresets {
		if seen[p.Key] {
			t.Errorf("duplicate preset key %q", p.Key)
		}
		seen[p.Key] = true
	}
}

func TestEQPresetIndex_Found(t *testing.T) {
	idx := EQPresetIndex("flat", BuiltinEQPresets)
	if idx != 0 {
		t.Errorf("index of 'flat' = %d, want 0", idx)
	}

	idx = EQPresetIndex("rock", BuiltinEQPresets)
	if idx < 0 {
		t.Errorf("'rock' not found in built-in presets")
	}
}

func TestEQPresetIndex_NotFound(t *testing.T) {
	idx := EQPresetIndex("does_not_exist", BuiltinEQPresets)
	if idx != -1 {
		t.Errorf("expected -1 for unknown key, got %d", idx)
	}
}

func TestEQPresetIndex_EmptySlice(t *testing.T) {
	idx := EQPresetIndex("flat", nil)
	if idx != -1 {
		t.Errorf("expected -1 for empty slice, got %d", idx)
	}
}

func TestSlugFromName_Basic(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"My Warm Sound", "my_warm_sound"},
		{"Late Night", "late_night"},
		{"Rock & Roll", "rock_roll"},
		{"  leading and trailing  ", "leading_and_trailing"},
		{"ALL CAPS", "all_caps"},
		{"already_slug", "already_slug"},
		{"hello-world", "hello_world"},
		{"123 Bass", "123_bass"},
		{"", ""},
		{"   ", ""},
	}
	for _, tc := range cases {
		got := SlugFromName(tc.in)
		if got != tc.want {
			t.Errorf("SlugFromName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugFromName_ConsecutiveSpecialChars(t *testing.T) {
	got := SlugFromName("foo   bar")
	if got != "foo_bar" {
		t.Errorf("SlugFromName(\"foo   bar\") = %q, want %q", got, "foo_bar")
	}
}
