package mpris

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/sglavoie/muse/internal/control"
)

// privateBus starts a throwaway session bus and points DBUS_SESSION_BUS_ADDRESS
// at it, skipping the test when dbus-daemon is not installed.
func privateBus(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed")
	}
	cmd := exec.Command(bin, "--session", "--nofork", "--print-address")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("read bus address: %v", err)
	}
	addr = strings.TrimSpace(addr)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", addr)
	return addr
}

// TestBus_EndToEnd drives the exported player the way playerctl or a desktop
// widget would: find it by name, read its properties, call a method, and hear
// PropertiesChanged.
func TestBus_EndToEnd(t *testing.T) {
	addr := privateBus(t)
	svc, err := Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = svc.Close() }()
	if svc.BusName() != "org.mpris.MediaPlayer2.muse" {
		t.Fatalf("bus name = %q", svc.BusName())
	}

	var mu sync.Mutex
	var ops []string
	svc.Serve(func(req control.Request) <-chan control.Reply {
		mu.Lock()
		ops = append(ops, req.Op)
		mu.Unlock()
		ch := make(chan control.Reply, 1)
		ch <- control.Reply{OK: true}
		return ch
	})

	client, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if err := client.AddMatchSignal(dbus.WithMatchInterface(ifaceProps), dbus.WithMatchMember("PropertiesChanged")); err != nil {
		t.Fatal(err)
	}
	signals := make(chan *dbus.Signal, 8)
	client.Signal(signals)

	obj := client.Object(svc.BusName(), objectPath)
	var identity dbus.Variant
	if err := obj.Call(ifaceProps+".Get", 0, ifaceRoot, "Identity").Store(&identity); err != nil || identity.Value() != "muse" {
		t.Fatalf("Identity = %v, %v", identity, err)
	}

	st := control.State{Playback: control.PlaybackPlaying, Track: song, Elapsed: 1}
	svc.Publish(control.Event{Event: control.EventState, State: &st})
	select {
	case sig := <-signals:
		changed := sig.Body[1].(map[string]dbus.Variant)
		if changed["PlaybackStatus"].Value() != "Playing" {
			t.Fatalf("PropertiesChanged = %v", changed)
		}
		md := changed["Metadata"].Value().(map[string]dbus.Variant)
		if md["xesam:title"].Value() != "Song" {
			t.Fatalf("Metadata = %v", md)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no PropertiesChanged after a state event")
	}

	if call := obj.Call(ifacePlayer+".PlayPause", 0); call.Err != nil {
		t.Fatalf("PlayPause: %v", call.Err)
	}
	if call := obj.Call(ifacePlayer+".Seek", 0, int64(10_000_000)); call.Err != nil {
		t.Fatalf("Seek: %v", call.Err)
	}
	mu.Lock()
	got := strings.Join(ops, ",")
	mu.Unlock()
	if got != "toggle-play,seek" {
		t.Fatalf("ops = %s, want toggle-play,seek", got)
	}

	var xml string
	if err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil || !strings.Contains(xml, ifacePlayer) {
		t.Fatalf("Introspect: %v", err)
	}
}
