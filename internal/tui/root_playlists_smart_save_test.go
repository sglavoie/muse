package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/smartrules"
)

type smartSaveStub struct {
	creates, updates  int
	writeErr, readErr error
	name, query       string
}

func (s *smartSaveStub) CreateSmartPlaylist(name, query string) (int64, error) {
	s.creates++
	s.name = name
	s.query = query
	return 7, s.writeErr
}
func (s *smartSaveStub) UpdateSmartPlaylistQuery(_ int64, name, query string) error {
	s.updates++
	s.name = name
	s.query = query
	return s.writeErr
}
func (s *smartSaveStub) ReadPlaylists() ([]db.PlaylistRow, error) {
	return []db.PlaylistRow{{ID: 7, Name: s.name, IsSmart: true, Query: s.query}}, s.readErr
}

func smartSaveModel(edit bool, writer smartPlaylistWriter) Model {
	m := New(context.Background())
	m.smartPlaylistWriter = writer
	editor := smartrules.NewSmartRuleEditor()
	if edit {
		editor = smartrules.NewSmartRuleEditorEdit(7, "Draft", db.SmartPlaylistQuery{Root: db.SmartRule{Conjunction: db.ConjAnd}})
	} else {
		for _, r := range "Draft" {
			editor, _, _ = editor.HandleKey(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	editor, _, _ = editor.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}) // rules
	editor, _, _ = editor.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter}) // sort, empty rule list
	m.overlay.smartRuleEditor = &editor
	return m
}

func TestSmartSaveFailureRetainsDraftAndRetry(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "edit"}[edit], func(t *testing.T) {
			stub := &smartSaveStub{writeErr: errors.New("disk full")}
			m := smartSaveModel(edit, stub)
			m, cmd, _ := m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || m.overlay.smartRuleEditor == nil {
				t.Fatal("save should retain pending form")
			}
			m, duplicate, _ := m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
			if duplicate != nil {
				t.Fatal("pending save duplicated")
			}
			m, _, _ = m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.overlay.smartRuleEditor == nil {
				t.Fatal("pending form closed")
			}
			next, _ := m.Update(cmd())
			m = next.(Model)
			if m.overlay.smartRuleEditor == nil || m.overlay.smartRuleEditor.Name() != "Draft" {
				t.Fatal("write failure lost draft")
			}
			if !strings.Contains(m.statusBar.TopNotification().Msg, "disk full") {
				t.Fatal("missing error history")
			}
			stub.writeErr = nil
			m, cmd, _ = m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("retry unavailable")
			}
			next, _ = m.Update(cmd())
			m = next.(Model)
			if m.overlay.smartRuleEditor != nil || len(m.playlists.Playlists) != 1 {
				t.Fatal("successful retry did not close and refresh")
			}
			if stub.creates+stub.updates != 2 {
				t.Fatal("wrong number of writes")
			}
		})
	}
}

func TestSmartSaveRefreshFailureDoesNotOfferDuplicateCreate(t *testing.T) {
	stub := &smartSaveStub{readErr: errors.New("read failed")}
	m := smartSaveModel(false, stub)
	m, cmd, _ := m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	next, _ := m.Update(cmd())
	m = next.(Model)
	if m.overlay.smartRuleEditor != nil || stub.creates != 1 {
		t.Fatal("successful write must close form")
	}
	if !strings.Contains(m.statusBar.TopNotification().Msg, "saved, but list refresh failed") {
		t.Fatal("must distinguish saved from failed write")
	}
}

func TestSmartSaveWithoutStorageKeepsDraft(t *testing.T) {
	m := smartSaveModel(false, nil)
	m, cmd, _ := m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.overlay.smartRuleEditor == nil {
		t.Fatal("unavailable storage lost draft")
	}
}

func TestSmartEditorReceivesTagCatalog(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewPlaylists
	m.tags = []db.TagRow{{Name: "Focus"}}
	m, _, _ = m.handlePlaylistKeyAction(keys.ActionSmartPlaylistCreate)
	for _, r := range "Draft" {
		m, _, _ = m.handleSmartRuleEditorKeyPress(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'a', Text: "a"}, {Code: tea.KeyLeft}, {Code: tea.KeyTab}, {Code: tea.KeyTab}, {Code: tea.KeyEnter}, {Code: tea.KeyEnter}, {Code: tea.KeyEnter}} {
		m, _, _ = m.handleSmartRuleEditorKeyPress(key)
	}
	rules := m.overlay.smartRuleEditor.BuildQuery().Root.Rules
	if len(rules) != 1 || rules[0].Field != db.FieldTag || rules[0].Value != "Focus" {
		t.Fatalf("catalog not wired: %+v", rules)
	}
}
