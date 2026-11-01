package mpris

import (
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"go.uber.org/goleak"

	"github.com/sglavoie/muse/internal/control"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type signal struct {
	name   string
	values []any
}

// harness is a service with a recorded bus, a scripted TUI and a manual clock.
type harness struct {
	*Service
	mu       sync.Mutex
	signals  []signal
	requests []control.Request
	reply    control.Reply
	clock    time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{reply: control.Reply{OK: true}, clock: time.Unix(1000, 0)}
	dispatch := func(req control.Request) <-chan control.Reply {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.requests = append(h.requests, req)
		ch := make(chan control.Reply, 1)
		ch <- h.reply
		return ch
	}
	emit := func(name string, values ...any) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.signals = append(h.signals, signal{name, values})
		return nil
	}
	h.Service = newService(dispatch, emit, func() time.Time {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.clock
	})
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.clock = h.clock.Add(d)
	h.mu.Unlock()
}

// publish feeds a state and flushes synchronously, as the emitter would.
func (h *harness) publish(st control.State) {
	h.Publish(control.Event{Event: control.EventState, State: &st})
	<-h.wake
	h.flush()
}

func (h *harness) takeSignals() []signal {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.signals
	h.signals = nil
	return s
}

var song = &control.Track{ID: "abc123", QueueID: 7, Title: "Song", Artist: "Band", Album: "Record", Duration: 200}

func TestPublish_SignalsChangedPropertiesOnly(t *testing.T) {
	h := newHarness(t)
	h.publish(control.State{Playback: control.PlaybackPlaying, Track: song, Elapsed: 3})
	sigs := h.takeSignals()
	if len(sigs) != 1 || sigs[0].name != ifaceProps+".PropertiesChanged" {
		t.Fatalf("signals = %+v, want one PropertiesChanged", sigs)
	}
	if sigs[0].values[0] != ifacePlayer {
		t.Fatalf("interface = %v, want %s", sigs[0].values[0], ifacePlayer)
	}
	changed := sigs[0].values[1].(map[string]dbus.Variant)
	for _, name := range []string{"PlaybackStatus", "Metadata", "CanSeek"} {
		if _, ok := changed[name]; !ok {
			t.Errorf("%s missing from %v", name, changed)
		}
	}
	if _, ok := changed["CanGoNext"]; ok {
		t.Error("an unchanged property was signalled")
	}
	if got := changed["PlaybackStatus"].Value(); got != "Playing" {
		t.Errorf("PlaybackStatus = %v, want Playing", got)
	}

	h.advance(time.Second)
	h.publish(control.State{Playback: control.PlaybackPaused, Track: song, Elapsed: 4})
	sigs = h.takeSignals()
	if len(sigs) != 1 {
		t.Fatalf("pause signals = %+v, want one", sigs)
	}
	changed = sigs[0].values[1].(map[string]dbus.Variant)
	if len(changed) != 1 || changed["PlaybackStatus"].Value() != "Paused" {
		t.Fatalf("pause changed %v, want only PlaybackStatus=Paused", changed)
	}
}

func TestMetadata(t *testing.T) {
	md := metadata(song)
	want := map[string]any{
		"mpris:trackid": dbus.ObjectPath(trackPathRoot + "q7"),
		"xesam:title":   "Song",
		"xesam:album":   "Record",
		"mpris:length":  int64(200_000_000),
	}
	for k, v := range want {
		if got := md[k].Value(); got != v {
			t.Errorf("%s = %#v, want %#v", k, got, v)
		}
	}
	if a := md["xesam:artist"].Value().([]string); len(a) != 1 || a[0] != "Band" {
		t.Errorf("xesam:artist = %v, want [Band]", a)
	}
	if none := metadata(nil); len(none) != 1 || none["mpris:trackid"].Value() != noTrack {
		t.Errorf("metadata(nil) = %v, want only the NoTrack id", none)
	}
	if p := trackPath(&control.Track{ID: "abc"}); p != dbus.ObjectPath(trackPathRoot+"habc") || !p.IsValid() {
		t.Errorf("hash track path = %q", p)
	}
	if p := trackPath(&control.Track{ID: "not/valid-"}); !p.IsValid() {
		t.Errorf("invalid hash produced an invalid path %q", p)
	}
}

func TestPosition_ExtrapolatesWhilePlaying(t *testing.T) {
	h := newHarness(t)
	h.publish(control.State{Playback: control.PlaybackPlaying, Track: song, Elapsed: 10})
	h.advance(2500 * time.Millisecond)
	if got := h.position(); got != 12_500_000 {
		t.Fatalf("playing position = %d, want 12500000", got)
	}
	h.publish(control.State{Playback: control.PlaybackPaused, Track: song, Elapsed: 12.5})
	h.advance(time.Minute)
	if got := h.position(); got != 12_500_000 {
		t.Fatalf("paused position = %d, want 12500000", got)
	}
	h.publish(control.State{Playback: control.PlaybackPlaying, Track: song, Elapsed: 199})
	h.advance(time.Minute)
	if got := h.position(); got != 200_000_000 {
		t.Fatalf("position past the end = %d, want the track length", got)
	}
}

func TestSeeked_EmittedOnJumpWithinTrack(t *testing.T) {
	h := newHarness(t)
	h.publish(control.State{Playback: control.PlaybackPlaying, Track: song, Elapsed: 10})
	h.takeSignals()

	h.advance(3 * time.Second)
	h.publish(control.State{Playback: control.PlaybackPaused, Track: song, Elapsed: 13})
	for _, s := range h.takeSignals() {
		if s.name == ifacePlayer+".Seeked" {
			t.Fatal("steady playback emitted Seeked")
		}
	}

	h.publish(control.State{Playback: control.PlaybackPaused, Track: song, Elapsed: 120})
	sigs := h.takeSignals()
	if len(sigs) != 1 || sigs[0].name != ifacePlayer+".Seeked" || sigs[0].values[0] != int64(120_000_000) {
		t.Fatalf("signals after a jump = %+v, want Seeked(120000000)", sigs)
	}

	other := *song
	other.QueueID = 8
	h.publish(control.State{Playback: control.PlaybackPlaying, Track: &other, Elapsed: 0})
	for _, s := range h.takeSignals() {
		if s.name == ifacePlayer+".Seeked" {
			t.Fatal("a track change emitted Seeked")
		}
	}
}

func TestPlayerMethods_DispatchOps(t *testing.T) {
	h := newHarness(t)
	p := playerIface{h.Service}
	calls := []struct {
		call func() *dbus.Error
		op   string
	}{
		{p.Next, control.OpNext},
		{p.Previous, control.OpPrevious},
		{p.Pause, control.OpPause},
		{p.PlayPause, control.OpTogglePlay},
		{p.Play, control.OpPlay},
		{p.Stop, control.OpPause},
	}
	for _, c := range calls {
		if err := c.call(); err != nil {
			t.Fatalf("%s: %v", c.op, err)
		}
	}
	if len(h.requests) != len(calls) {
		t.Fatalf("requests = %+v", h.requests)
	}
	for i, c := range calls {
		if h.requests[i].Op != c.op {
			t.Errorf("request %d = %q, want %q", i, h.requests[i].Op, c.op)
		}
	}

	h.reply = control.Reply{OK: false, Message: "Queue is empty"}
	if err := p.Play(); err == nil || err.Name != errFailed {
		t.Fatalf("refused op = %v, want %s", err, errFailed)
	}
	if err := p.OpenUri("file:///x.mp3"); err == nil || err.Name != errNotSupport {
		t.Fatalf("OpenUri = %v, want NotSupported", err)
	}
}

func TestSeekAndSetPosition(t *testing.T) {
	h := newHarness(t)
	p := playerIface{h.Service}
	if err := p.SeekBy(5_000_000); err != nil || len(h.requests) != 0 {
		t.Fatalf("seek with nothing loaded = %v / %+v, want a silent no-op", err, h.requests)
	}
	h.publish(control.State{Playback: control.PlaybackPaused, Track: song, Elapsed: 50})

	_ = p.SeekBy(-10_000_000)
	_ = p.SeekBy(-100_000_000)
	_ = p.SeekBy(500_000_000)
	_ = p.SetPosition(trackPath(song), 30_000_000)
	_ = p.SetPosition(noTrack, 30_000_000)
	_ = p.SetPosition(trackPath(song), 300_000_000)

	want := []struct {
		op  string
		pos float64
	}{
		{control.OpSeek, 40},
		{control.OpSeek, 0},
		{control.OpNext, 0},
		{control.OpSeek, 30},
	}
	if len(h.requests) != len(want) {
		t.Fatalf("requests = %+v, want %d", h.requests, len(want))
	}
	for i, w := range want {
		r := h.requests[i]
		if r.Op != w.op || (w.op == control.OpSeek && (r.Position == nil || *r.Position != w.pos)) {
			t.Errorf("request %d = %+v, want %s %v", i, r, w.op, w.pos)
		}
	}
}

func TestProperties(t *testing.T) {
	h := newHarness(t)
	props := propsIface{h.Service}
	if v, err := props.Get(ifaceRoot, "Identity"); err != nil || v.Value() != "muse" {
		t.Fatalf("Identity = %v, %v", v, err)
	}
	all, err := props.GetAll(ifacePlayer)
	if err != nil {
		t.Fatal(err)
	}
	if all["PlaybackStatus"].Value() != "Stopped" || all["CanSeek"].Value() != false {
		t.Fatalf("idle player props = %v", all)
	}
	if _, ok := all["Position"]; !ok {
		t.Fatal("GetAll must include Position")
	}
	if _, err := props.Get(ifacePlayer, "Volume"); err == nil || err.Name != errUnknownProp {
		t.Fatalf("Get(Volume) = %v, want UnknownProperty", err)
	}
	if _, err := props.GetAll("org.example.Nope"); err == nil || err.Name != errUnknownIfc {
		t.Fatalf("GetAll(unknown) = %v, want UnknownInterface", err)
	}
	if err := props.Set(ifacePlayer, "Rate", dbus.MakeVariant(2.0)); err == nil || err.Name != errReadOnly {
		t.Fatalf("Set(Rate) = %v, want PropertyReadOnly", err)
	}
}

func TestRequest_FailsOnceClosed(t *testing.T) {
	h := newHarness(t)
	_ = h.Close()
	if err := (playerIface{h.Service}).Next(); err == nil {
		t.Fatal("a closed service must refuse methods")
	}
	if len(h.requests) != 0 {
		t.Fatal("a closed service must not reach the TUI")
	}
}

func TestEmitterRunsAndStops(t *testing.T) {
	h := newHarness(t)
	h.run()
	st := control.State{Playback: control.PlaybackPlaying, Track: song}
	h.Publish(control.Event{Event: control.EventState, State: &st})
	deadline := time.Now().Add(2 * time.Second)
	for len(h.takeSignalsPeek()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the emitter never signalled")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) takeSignalsPeek() []signal {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.signals
}

func TestSessionBusAddress(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	tests := []struct {
		name   string
		env    map[string]string
		exists func(string) bool
		want   string
	}{
		{"explicit address wins", map[string]string{"DBUS_SESSION_BUS_ADDRESS": "unix:path=/x", "XDG_RUNTIME_DIR": "/run/user/1"}, yes, "unix:path=/x"},
		{"runtime dir bus", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}, yes, "unix:path=/run/user/1/bus"},
		{"runtime dir without a bus", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1"}, no, ""},
		{"nothing", nil, yes, ""},
	}
	for _, tt := range tests {
		if got := sessionBusAddress(env(tt.env), tt.exists); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestServe_RefusesCallsUntilServing(t *testing.T) {
	var got []control.Request
	s := newService(nil, func(string, ...any) error { return nil }, time.Now)
	t.Cleanup(func() { _ = s.Close() })
	if err := (playerIface{s}).Next(); err == nil {
		t.Fatal("a call before Serve must be refused")
	}
	s.Serve(func(req control.Request) <-chan control.Reply {
		got = append(got, req)
		ch := make(chan control.Reply, 1)
		ch <- control.Reply{OK: true}
		return ch
	})
	if err := (playerIface{s}).Next(); err != nil || len(got) != 1 {
		t.Fatalf("after Serve: err %v, requests %+v", err, got)
	}
}
