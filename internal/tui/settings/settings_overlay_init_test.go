package settings

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

func TestNewSettingsOverlayInitializesFromConfig(t *testing.T) {
	cfg := config.Defaults()
	currentVolume := 75
	o := NewSettingsOverlay(cfg, currentVolume, cfg.MusicDirectories)

	pb := o.Fields[TabPlayback]
	if len(pb) != 8 {
		t.Fatalf("expected 8 playback fields, got %d", len(pb))
	}
	if pb[0].Value != cfg.VolumeIncrement {
		t.Errorf("VolumeIncrement: expected %d, got %d", cfg.VolumeIncrement, pb[0].Value)
	}
	if pb[1].Value != int(cfg.SeekOffsetShort) {
		t.Errorf("SeekOffsetShort: expected %d, got %d", int(cfg.SeekOffsetShort), pb[1].Value)
	}
	if pb[2].Value != int(cfg.SeekOffsetLong) {
		t.Errorf("SeekOffsetLong: expected %d, got %d", int(cfg.SeekOffsetLong), pb[2].Value)
	}
	if pb[3].Value != currentVolume {
		t.Errorf("DefaultVolume: expected %d, got %d", currentVolume, pb[3].Value)
	}
	if pb[4].Value != boolToInt(cfg.NormalizeVolume) {
		t.Errorf("NormalizeVolume: expected %d, got %d", boolToInt(cfg.NormalizeVolume), pb[4].Value)
	}
	if pb[5].Label != "Gapless" {
		t.Errorf("Gapless label: expected %q, got %q", "Gapless", pb[5].Label)
	}
	if pb[5].Default != 1 {
		t.Errorf("Gapless Default: expected 1, got %d", pb[5].Default)
	}
	if pb[5].Min != 0 || pb[5].Max != 1 {
		t.Errorf("Gapless Min/Max: expected 0/1, got %d/%d", pb[5].Min, pb[5].Max)
	}
	if pb[5].Value != boolToInt(cfg.GaplessAudio) {
		t.Errorf("GaplessAudio: expected %d, got %d", boolToInt(cfg.GaplessAudio), pb[5].Value)
	}

	disp := o.Fields[TabDisplay]
	if len(disp) != 3 {
		t.Fatalf("expected 3 display fields, got %d", len(disp))
	}
	if disp[0].Value != cfg.TransientDurationSecs {
		t.Errorf("TransientDurationSecs: expected %d, got %d", cfg.TransientDurationSecs, disp[0].Value)
	}
	if disp[1].Value != cfg.VolumeFeedbackDurationSecs {
		t.Errorf("VolumeFeedbackDurationSecs: expected %d, got %d", cfg.VolumeFeedbackDurationSecs, disp[1].Value)
	}
	if disp[2].Label != "Track notify" {
		t.Errorf("Track notify label: expected %q, got %q", "Track notify", disp[2].Label)
	}
	if disp[2].Default != 0 || disp[2].Min != 0 || disp[2].Max != 1 {
		t.Errorf("Track notify Default/Min/Max: expected 0/0/1, got %d/%d/%d",
			disp[2].Default, disp[2].Min, disp[2].Max)
	}
	if disp[2].Value != boolToInt(cfg.NotifyOnTrackChange) {
		t.Errorf("NotifyOnTrackChange: expected %d, got %d", boolToInt(cfg.NotifyOnTrackChange), disp[2].Value)
	}
}

func TestNewSettingsOverlayUsesCurrentVolume(t *testing.T) {
	cfg := config.Defaults()
	cfg.DefaultVolume = 100
	currentVolume := 75
	o := NewSettingsOverlay(cfg, currentVolume, cfg.MusicDirectories)
	pb := o.Fields[TabPlayback]
	if pb[3].Value != 75 {
		t.Errorf("expected Default volume field to show currentVolume 75, got %d", pb[3].Value)
	}
}

func TestNewSettingsOverlayDefaultsTab(t *testing.T) {
	o := NewSettingsOverlay(config.Defaults(), config.Defaults().DefaultVolume, config.Defaults().MusicDirectories)
	if o.Tab != TabLibrary {
		t.Errorf("expected initial tab=settingsTabLibrary, got %v", o.Tab)
	}
	if o.cursor != 0 {
		t.Errorf("expected initial cursor=0, got %d", o.cursor)
	}
}

func TestStereoWideFieldsInitializeFromConfig(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 20
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	pb := o.Fields[TabPlayback]
	if pb[6].Value != 1 {
		t.Errorf("StereoWideEnabled field: expected 1, got %d", pb[6].Value)
	}
	if pb[7].Value != 20 {
		t.Errorf("StereoWideAmount field: expected 20, got %d", pb[7].Value)
	}
}

func TestStereoWideStateHelper(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 25
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	enabled, amount := o.StereoWideState()
	if !enabled {
		t.Error("StereoWideState: expected enabled=true")
	}
	if amount != 25 {
		t.Errorf("StereoWideState: expected amount=25, got %d", amount)
	}
}
