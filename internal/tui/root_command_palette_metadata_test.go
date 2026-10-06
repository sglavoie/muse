package tui

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/metadata"
)

func TestMetadataPaletteMatchesKeyboardTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(Model) Model
		mode   metadata.Mode
		count  int
		detail string
	}{
		{"artist", func(m Model) Model { return pressKey(m, "4") }, metadata.ModeRenameArtist, 3, "Rename artist across 3 tracks: Alice"},
		{"artist album", func(m Model) Model { return pressEnter(pressKey(m, "4")) }, metadata.ModeRenameAlbum, 2, "Rename album across 2 tracks:"},
		{"album", switchToAlbumsMode, metadata.ModeRenameAlbum, 2, "Rename album across 2 tracks:"},
		{"track", func(m Model) Model { return m }, metadata.ModeDefault, 0, "Edit metadata:"},
		{"hidden selection", func(m Model) Model {
			m.library.SelectedHashes = map[string]struct{}{"h1": {}, "h2": {}}
			m = pressSlash(m)
			m = typeText(m, "no-matching-track")
			return pressEnter(m)
		}, metadata.ModeDefault, 2, "Edit metadata for 2 tracks"},
		{"data health", func(m Model) Model {
			m.ActiveView = viewInsights
			m.insights.Panel = instui.PanelDataHealth
			m.insights.Health = []instui.HealthRow{{Hashes: []string{"h1", "h2"}}}
			return m
		}, metadata.ModeDefault, 2, "Edit metadata for 2 tracks"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.setup(newModelWithTracks())
			cmd := polishCommand(t, m, "track.metadata.edit")
			if !cmd.IsEnabled() || !strings.Contains(cmd.Detail, tc.detail) {
				t.Fatalf("metadata command: enabled=%v detail=%q", cmd.IsEnabled(), cmd.Detail)
			}
			keyboard := pressMKey(m).overlay.metadataEditor
			viaPalette := runPaletteCommandForTest(t, m, cmd.Name).overlay.metadataEditor
			if keyboard == nil || viaPalette == nil {
				t.Fatal("both entry points must open an editor")
			}
			if viaPalette.Mode() != tc.mode || len(viaPalette.Hashes()) != tc.count {
				t.Fatalf("mode=%v hashes=%v", viaPalette.Mode(), viaPalette.Hashes())
			}
			kh, ph := slices.Clone(keyboard.Hashes()), slices.Clone(viaPalette.Hashes())
			slices.Sort(kh)
			slices.Sort(ph)
			if !reflect.DeepEqual(kh, ph) || keyboard.Hash() != viaPalette.Hash() {
				t.Fatal("keyboard and palette targeted different tracks")
			}
		})
	}
}

func TestMetadataPaletteIgnoresInactiveHealthFinding(t *testing.T) {
	m := New(context.Background())
	m.insights.Panel = instui.PanelDataHealth
	m.insights.Health = []instui.HealthRow{{Hashes: []string{"old-track"}}}
	cmd := polishCommand(t, m, "track.metadata.edit")
	if cmd.IsEnabled() || cmd.DisabledReason == "" {
		t.Fatal("an empty library must not target an inactive health finding")
	}
	if pressMKey(m).overlay.metadataEditor != nil {
		t.Fatal("keyboard opened an inactive health finding")
	}
}
