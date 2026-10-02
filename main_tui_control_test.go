package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/control"
)

// shortSocketPath returns a socket path short enough for macOS's sun_path,
// which t.TempDir's nested names can exceed.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "musem")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "c.sock")
}

type recordingPublisher struct{ events []control.Event }

func (r *recordingPublisher) Publish(e control.Event) { r.events = append(r.events, e) }

func TestControlNotifier_PublishesNotifyEvent(t *testing.T) {
	pub := &recordingPublisher{}
	if err := (controlNotifier{pub: pub}).NotifyTrack("Song", "Band", "Record"); err != nil {
		t.Fatalf("NotifyTrack: %v", err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("published %d events, want 1", len(pub.events))
	}
	e := pub.events[0]
	if e.Event != control.EventNotify || e.Notify == nil {
		t.Fatalf("event = %+v, want a notify event", e)
	}
	want := control.Notification{Title: "Song", Subtitle: "Band", Body: "Record"}
	if *e.Notify != want {
		t.Fatalf("notification = %+v, want %+v", *e.Notify, want)
	}
}

func TestNotifierFor_UnhostedUsesDesktop(t *testing.T) {
	if _, ok := notifierFor(nil).(desktopNotifier); !ok {
		t.Fatalf("unhosted notifier = %T, want desktopNotifier", notifierFor(nil))
	}
	if opts := controlModelOptions(nil); len(opts) != 0 {
		t.Fatalf("unhosted model options = %d, want none", len(opts))
	}
}

func TestListenControl_UnhostedIsNoOp(t *testing.T) {
	var deps app.ShutdownDeps
	srv, err := listenControl(tuiOptions{}, &deps)
	if err != nil || srv != nil {
		t.Fatalf("listenControl unhosted = (%v, %v), want (nil, nil)", srv, err)
	}
	if deps.DrainControl != nil || deps.CloseControl != nil {
		t.Fatal("unhosted session must not wire control shutdown steps")
	}
	if err := serveControl(nil, func(tea.Msg) {}); err != nil {
		t.Fatalf("serveControl(nil) = %v, want nil", err)
	}
}

func TestListenControl_BindFailureIsReturned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var deps app.ShutdownDeps
	if _, err := listenControl(tuiOptions{ControlPath: path}, &deps); err == nil {
		t.Fatal("binding over a regular file must fail")
	}
	if deps.CloseControl != nil {
		t.Fatal("a failed bind must not wire control shutdown steps")
	}
}

// TestHostedControl_HelloStateAndRequest drives the socket the way a host
// would: hello first, the published state next, and a request reaching the TUI
// through the program's Send.
func TestHostedControl_HelloStateAndRequest(t *testing.T) {
	path := shortSocketPath(t)
	var deps app.ShutdownDeps
	srv, err := listenControl(tuiOptions{ControlPath: path}, &deps)
	if err != nil {
		t.Fatalf("listenControl: %v", err)
	}
	if deps.DrainControl == nil || deps.CloseControl == nil {
		t.Fatal("hosted session must wire control drain and close")
	}
	t.Cleanup(deps.CloseControl)

	if n := len(controlModelOptions(srv)); n != 1 {
		t.Fatalf("hosted model options = %d, want 1", n)
	}
	if _, ok := notifierFor(srv).(controlNotifier); !ok {
		t.Fatalf("hosted notifier = %T, want controlNotifier", notifierFor(srv))
	}

	srv.Publish(control.Event{Event: control.EventState, State: &control.State{Playback: control.PlaybackPaused}})

	sent := make(chan tea.Msg, 1)
	if err := serveControl(srv, func(msg tea.Msg) { sent <- msg }); err != nil {
		t.Fatalf("serveControl: %v", err)
	}

	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewScanner(c)

	var hello control.Event
	readLine(t, r, &hello)
	if hello.Event != control.EventHello || hello.Hello == nil ||
		hello.Hello.PID != os.Getpid() || hello.Hello.Protocol != control.ProtocolVersion {
		t.Fatalf("first line = %+v, want this process's hello", hello)
	}
	var state control.Event
	readLine(t, r, &state)
	if state.Event != control.EventState || state.State == nil || state.State.Playback != control.PlaybackPaused {
		t.Fatalf("second line = %+v, want the published state", state)
	}

	if _, err := c.Write([]byte(`{"id":7,"op":"toggle-play"}` + "\n")); err != nil {
		t.Fatalf("write request: %v", err)
	}
	select {
	case msg := <-sent:
		if msg == nil {
			t.Fatal("dispatch sent a nil message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request never reached the program")
	}
}

func readLine(t *testing.T, r *bufio.Scanner, v any) {
	t.Helper()
	if !r.Scan() {
		t.Fatalf("read: %v", r.Err())
	}
	if err := json.Unmarshal(r.Bytes(), v); err != nil {
		t.Fatalf("decode %q: %v", r.Text(), err)
	}
}
