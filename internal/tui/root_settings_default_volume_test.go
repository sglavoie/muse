package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/settings"
)

func TestSettingsSavePreservesDefaultVolumeUnlessEdited(t *testing.T) {
	for _, editVolume := range []bool{false, true} {
		cfg := config.Defaults()
		cfg.DefaultVolume = 80
		m := New(context.Background(), WithConfig(cfg))
		m.playback.volume = 25
		opened, _ := m.handleSettingsOpen()
		m = opened.(Model)
		if got := m.overlay.settings.Fields[settings.TabPlayback][3].Value; got != 80 {
			t.Fatalf("default volume shows live volume: %d", got)
		}
		m.overlay.settings.Fields[settings.TabPlayback][0].Value = 10
		want := 80
		if editVolume {
			want = 60
			m.overlay.settings.Fields[settings.TabPlayback][3].Value = want
		}
		m, _ = m.handleSettingsSave()
		if m.cfg.DefaultVolume != want || m.cfg.VolumeIncrement != 10 || m.playback.volume != 25 {
			t.Fatalf("edit=%v: default=%d, increment=%d, live=%d", editVolume, m.cfg.DefaultVolume, m.cfg.VolumeIncrement, m.playback.volume)
		}
	}
}
