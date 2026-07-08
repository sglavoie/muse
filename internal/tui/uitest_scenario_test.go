package tui

import (
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		w, h    int
		wantErr bool
	}{
		{in: "100x30", w: 100, h: 30},
		{in: "80X24", w: 80, h: 24},
		{in: " 120x40 ", w: 120, h: 40},
		{in: "0x30", wantErr: true},
		{in: "100x0", wantErr: true},
		{in: "-5x10", wantErr: true},
		{in: "100", wantErr: true},
		{in: "axb", wantErr: true},
		{in: "", wantErr: true},
		{in: "10x20x30", wantErr: true},
	}
	for _, c := range cases {
		w, h, err := ParseSize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseSize(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSize(%q) unexpected error: %v", c.in, err)
			continue
		}
		if w != c.w || h != c.h {
			t.Errorf("ParseSize(%q) = %dx%d, want %dx%d", c.in, w, h, c.w, c.h)
		}
	}
}

func TestParseScenarioValid(t *testing.T) {
	data := []byte(`{
		"name": "baseline",
		"fixture": "rich",
		"size": "100x30",
		"actions": [
			{"key": "tab"},
			{"text": "hello"},
			{"resize": "80x24"},
			{"wait": "50ms"},
			{"command": "help"},
			{"assert": {"view": "queue"}},
			{"assert": {"no_layout_issues": true}},
			{"label": "look", "assert": {"contains": "Queue"}}
		]
	}`)
	s, err := ParseScenario(data)
	if err != nil {
		t.Fatalf("ParseScenario error: %v", err)
	}
	if s.Name != "baseline" || s.Fixture != "rich" {
		t.Errorf("unexpected scenario header: %+v", s)
	}
	if len(s.Actions) != 8 {
		t.Fatalf("expected 8 actions, got %d", len(s.Actions))
	}
	if k, err := s.Actions[0].Kind(); err != nil || k != ActionKey {
		t.Errorf("action 0 kind = %q err %v, want key", k, err)
	}
	if s.Actions[7].Label != "look" {
		t.Errorf("action 7 label = %q, want look", s.Actions[7].Label)
	}
}

func TestParseScenarioRejectsUnknownField(t *testing.T) {
	data := []byte(`{"name":"x","actions":[{"key":"a"}],"bogus":1}`)
	if _, err := ParseScenario(data); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseScenarioRejectsUnknownNestedField(t *testing.T) {
	data := []byte(`{"name":"x","actions":[{"key":"a","mystery":true}]}`)
	if _, err := ParseScenario(data); err == nil {
		t.Fatal("expected error for unknown nested field")
	}
}

func TestParseScenarioRejectsEmpty(t *testing.T) {
	for _, data := range []string{
		`{"name":"x","actions":[]}`,
		`{"name":"","actions":[{"key":"a"}]}`,
		`{"actions":[{"key":"a"}]}`,
	} {
		if _, err := ParseScenario([]byte(data)); err == nil {
			t.Errorf("expected error for %s", data)
		}
	}
}

func TestParseScenarioRejectsTrailingContent(t *testing.T) {
	data := []byte(`{"name":"x","actions":[{"key":"a"}]} {"extra":1}`)
	if _, err := ParseScenario(data); err == nil {
		t.Fatal("expected error for trailing content")
	}
}

func TestActionValidateExactlyOneOperation(t *testing.T) {
	key := "a"
	txt := "hi"
	// No operation set.
	if _, err := (UITestAction{}).Validate(); err == nil {
		t.Error("expected error for action with no operation")
	}
	// Two operations set.
	if _, err := (UITestAction{Key: &key, Text: &txt}).Validate(); err == nil {
		t.Error("expected error for action with two operations")
	}
	// Bad key.
	bad := "notakey"
	if _, err := (UITestAction{Key: &bad}).Validate(); err == nil {
		t.Error("expected error for invalid key")
	}
	// Empty text.
	empty := ""
	if _, err := (UITestAction{Text: &empty}).Validate(); err == nil {
		t.Error("expected error for empty text")
	}
}

func TestActionWaitAllowsEmptyAndValidatesDuration(t *testing.T) {
	empty := ""
	if k, err := (UITestAction{Wait: &empty}).Validate(); err != nil || k != ActionWait {
		t.Errorf("empty wait should be valid drain, got kind %q err %v", k, err)
	}
	bad := "notaduration"
	if _, err := (UITestAction{Wait: &bad}).Validate(); err == nil {
		t.Error("expected error for invalid wait duration")
	}
}

func TestAssertionValidation(t *testing.T) {
	view := "queue"
	yes := true
	no := false
	// Valid single-field assertions.
	for _, a := range []UITestAssertion{
		{View: &view},
		{NoLayoutIssues: &yes},
	} {
		act := UITestAction{Assert: &a}
		if _, err := act.Validate(); err != nil {
			t.Errorf("assertion %+v unexpectedly invalid: %v", a, err)
		}
	}
	// no_layout_issues must be true.
	if err := (&UITestAssertion{NoLayoutIssues: &no}).Validate(); err == nil {
		t.Error("expected error for no_layout_issues:false")
	}
	// Multiple fields set.
	other := "library"
	if err := (&UITestAssertion{View: &view, Overlay: &other}).Validate(); err == nil {
		t.Error("expected error for assertion with two expectations")
	}
	// No field set.
	if err := (&UITestAssertion{}).Validate(); err == nil {
		t.Error("expected error for empty assertion")
	}
	// Empty contains.
	empty := ""
	if err := (&UITestAssertion{Contains: &empty}).Validate(); err == nil {
		t.Error("expected error for empty contains")
	}
}

func TestParseScenarioReportsActionIndex(t *testing.T) {
	data := []byte(`{"name":"x","actions":[{"key":"a"},{"key":""}]}`)
	_, err := ParseScenario(data)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "action 1") {
		t.Errorf("error should name failing action index: %v", err)
	}
}
