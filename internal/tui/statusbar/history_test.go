package statusbar

import (
	"fmt"
	"testing"
	"time"
)

func TestHistorySurvivesExpiryAndEviction(t *testing.T) {
	var s StatusBar
	for i := 0; i < 60; i++ {
		s.PushMsg(fmt.Sprintf("result %d", i), -time.Second, NotifNormal)
	}
	s.NotifyInfo("Scanning 100 files")
	s.PruneNotifications()
	history := s.RecentMessages()
	if len(history) != 50 || history[0].Text != "result 10" || history[49].Text != "result 59" {
		t.Fatalf("unexpected history: %+v", history)
	}
	history[0].Text = "mutated"
	if s.RecentMessages()[0].Text != "result 10" {
		t.Fatal("snapshot aliases live history")
	}
	copyOfStatus := s
	s.Notify("new result")
	if copyOfStatus.RecentMessages()[49].Text != "result 59" {
		t.Fatal("append mutated copied model")
	}
}
