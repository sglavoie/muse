package config

import "testing"

func TestSave_EQ_RoundTrip(t *testing.T) {
	cfg := defaults()
	cfg.EQEnabled = true
	cfg.EQPreset = "bass_boost"
	cfg.EQBands = [10]int{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}
	cfg.EQCustomPresets = []EQCustomPreset{
		{Name: "My Preset", Key: "my_preset", Bands: [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
	}

	path := saveEQConfig(t, cfg)
	raw := decodeRawEQConfig(t, path)

	if raw.EQEnabled == nil || !*raw.EQEnabled {
		t.Error("EQEnabled not persisted")
	}
	if raw.EQPreset != "bass_boost" {
		t.Errorf("EQPreset = %q, want %q", raw.EQPreset, "bass_boost")
	}
	if raw.EQBands == nil {
		t.Fatal("EQBands not persisted")
	}
	if (*raw.EQBands)[0] != 6 {
		t.Errorf("EQBands[0] = %d, want 6", (*raw.EQBands)[0])
	}
	if len(raw.EQCustomPresets) != 1 {
		t.Fatalf("EQCustomPresets len = %d, want 1", len(raw.EQCustomPresets))
	}
	if raw.EQCustomPresets[0].Key != "my_preset" {
		t.Errorf("custom preset key = %q, want %q", raw.EQCustomPresets[0].Key, "my_preset")
	}
}
