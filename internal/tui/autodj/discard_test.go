package autodj

import "testing"

func TestDirtyDJDraftRequiresDiscardAcrossTabs(t *testing.T) {
	for _, tab := range []panelTab{tabTune, tabPreview, tabRecipes} {
		p, _ := press(t, newTestPanel(true), "right")
		p.tab = tab
		draft := p.Draft()
		p, action := press(t, p, "esc")
		if action != actionNone || !p.discard.Active {
			t.Fatal("dirty draft closed without a choice")
		}
		p, action = press(t, p, "enter")
		if action != actionNone || p.discard.Active || p.tab != tab || !p.Draft().Equal(draft) {
			t.Fatal("default choice lost the draft or tab")
		}
		p, _ = press(t, p, "esc", "esc")
		if p.discard.Active || !p.Draft().Equal(draft) {
			t.Fatal("Esc must keep editing")
		}
		_, action = press(t, p, "esc", "tab", "enter")
		if action != ActionCancel {
			t.Fatal("explicit discard did not close")
		}
	}
}

func TestAppliedDJDraftClosesWithoutPrompt(t *testing.T) {
	p, _ := press(t, newTestPanel(true), "right")
	p = p.Submit(1)
	p = p.Ack(Applied{Token: 1, Settings: p.Draft(), Active: true})
	_, action := press(t, p, "esc")
	if action != ActionCancel {
		t.Fatal("applied draft should close immediately")
	}
}
