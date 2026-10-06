package settings

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
)

func TestSettingsDiscardDefaultsToKeepAndRestoredValuesClose(t *testing.T) {
	cfg := config.Defaults()
	cfg.SeekOffsetShort = 2.5
	o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
	if o.hasChanges() {
		t.Fatal("new form is dirty")
	}
	o.Tab = TabPlayback
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyRight})
	o, act, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if act == ActionCancel || !o.discard.Active {
		t.Fatal("edited settings closed")
	}
	o, act, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if act == ActionCancel || o.Fields[TabPlayback][0].Value != cfg.VolumeIncrement+1 {
		t.Fatal("default did not preserve edit")
	}
	o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if o.hasChanges() {
		t.Fatal("reverted settings should not prompt")
	}
	_, act, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if act != ActionCancel {
		t.Fatal("unchanged settings should close")
	}
}

func TestSettingsDiscardCoversLibraryAndPresets(t *testing.T) {
	for _, change := range []func(*SettingsOverlay){
		func(o *SettingsOverlay) { o.LibraryDirs = append(o.LibraryDirs, "/another") },
		func(o *SettingsOverlay) { o.ExcludedPaths = append(o.ExcludedPaths, "*/skip/*") },
		func(o *SettingsOverlay) {
			o.EQPresets = append(o.EQPresets, config.EQPresetDef{Name: "Mine", Key: "mine"})
		},
		func(o *SettingsOverlay) { o.primaryColor = "#abcdef" },
	} {
		cfg := config.Defaults()
		o := NewSettingsOverlay(cfg, cfg.MusicDirectories)
		change(&o)
		o, act, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		if act == ActionCancel || !o.discard.Active {
			t.Fatal("changed settings not protected")
		}
		o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
		_, act, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		if act != ActionCancel {
			t.Fatal("explicit discard should close")
		}
	}
}
