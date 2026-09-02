package settings

import (
	"testing"

	"github.com/sglavoie/muse/internal/config"
)

func TestSettingsToConfigRoundTrip(t *testing.T) {
	base := config.Defaults()
	o := NewSettingsOverlay(base, base.DefaultVolume, base.MusicDirectories)

	o.Fields[TabPlayback][0].Value = 10
	o.Fields[TabPlayback][1].Value = 15
	o.Fields[TabPlayback][2].Value = 60
	o.Fields[TabPlayback][3].Value = 80
	o.Fields[TabPlayback][4].Value = 1
	o.Fields[TabPlayback][5].Value = 0
	o.Fields[TabDisplay][0].Value = 8
	o.Fields[TabDisplay][1].Value = 4
	o.Fields[TabDisplay][2].Value = 1

	got := o.ToConfig(base)

	if got.VolumeIncrement != 10 {
		t.Errorf("VolumeIncrement: expected 10, got %d", got.VolumeIncrement)
	}
	if got.SeekOffsetShort != 15.0 {
		t.Errorf("SeekOffsetShort: expected 15.0, got %f", got.SeekOffsetShort)
	}
	if got.SeekOffsetLong != 60.0 {
		t.Errorf("SeekOffsetLong: expected 60.0, got %f", got.SeekOffsetLong)
	}
	if got.DefaultVolume != 80 {
		t.Errorf("DefaultVolume: expected 80, got %d", got.DefaultVolume)
	}
	if !got.NormalizeVolume {
		t.Errorf("NormalizeVolume: expected true, got false")
	}
	if got.GaplessAudio {
		t.Errorf("GaplessAudio: expected false, got true")
	}
	if got.TransientDurationSecs != 8 {
		t.Errorf("TransientDurationSecs: expected 8, got %d", got.TransientDurationSecs)
	}
	if got.VolumeFeedbackDurationSecs != 4 {
		t.Errorf("VolumeFeedbackDurationSecs: expected 4, got %d", got.VolumeFeedbackDurationSecs)
	}
	if !got.NotifyOnTrackChange {
		t.Errorf("NotifyOnTrackChange: expected true, got false")
	}
}

func TestSettingsToConfigPreservesOtherFields(t *testing.T) {
	base := config.Defaults()
	base.MusicDirectories = []string{"/my/music"}
	base.MPVSocketPath = "/tmp/mpv.sock"

	o := NewSettingsOverlay(base, base.DefaultVolume, base.MusicDirectories)
	got := o.ToConfig(base)

	if len(got.MusicDirectories) == 0 || got.MusicDirectories[0] != "/my/music" {
		t.Errorf("MusicDirectories[0]: expected %q, got %v", "/my/music", got.MusicDirectories)
	}
	if got.MPVSocketPath != base.MPVSocketPath {
		t.Errorf("MPVSocketPath: expected %q, got %q", base.MPVSocketPath, got.MPVSocketPath)
	}
}

func TestStereoWideToConfigRoundTrip(t *testing.T) {
	cfg := config.Defaults()
	cfg.StereoWideEnabled = true
	cfg.StereoWideAmount = 30
	o := NewSettingsOverlay(cfg, cfg.DefaultVolume, cfg.MusicDirectories)
	got := o.ToConfig(cfg)
	if !got.StereoWideEnabled {
		t.Error("ToConfig: expected StereoWideEnabled=true")
	}
	if got.StereoWideAmount != 30 {
		t.Errorf("ToConfig: expected StereoWideAmount=30, got %d", got.StereoWideAmount)
	}
}
