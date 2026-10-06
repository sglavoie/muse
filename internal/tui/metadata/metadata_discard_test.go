package metadata

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestEditedMetadataRequiresDiscardChoice(t *testing.T) {
	for name, editor := range map[string]Editor{
		"single": NewEditor("h", "Title", "Artist", "Album", "", 0),
		"bulk":   NewBulkEditor([]string{"h", "i"}, "Title", "", "", "", ""),
		"rename": NewArtistRenameEditor([]string{"h"}, "Artist"),
	} {
		t.Run(name, func(t *testing.T) {
			ed, _, _ := editor.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
			value, focus := ed.Fields[ed.focused].Value(), ed.focused
			ed, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if action == ActionCancel || !ed.discard.Active {
				t.Fatal("edited draft closed without a choice")
			}
			if !strings.Contains(ed.Render(60, 13, styles.NewStyles(true)), "Keep editing") {
				t.Fatal("missing choice")
			}
			ed, action, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			if action != actionNone || ed.discard.Active || ed.focused != focus || ed.Fields[focus].Value() != value {
				t.Fatal("default choice did not preserve editing context")
			}
			ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if ed.discard.Active || ed.Fields[focus].Value() != value {
				t.Fatal("Esc must keep editing")
			}
			ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
			_, action, _ = ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			if action != ActionCancel {
				t.Fatal("explicit discard did not close")
			}
		})
	}
}

func TestResetMetadataClosesWithoutPrompt(t *testing.T) {
	ed := NewEditor("h", "Title", "", "", "", 0)
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	ed, _, _ = ed.HandleKey(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	_, action, _ := ed.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if action != ActionCancel {
		t.Fatal("reset draft should close immediately")
	}
}
