//go:build linux

package notify

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNotifySendArgs(t *testing.T) {
	args := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}.notifySendArgs()

	wantFlags := []string{
		"--app-name=muse",
		"--expire-time=5000",
		"--hint=string:x-canonical-private-synchronous:muse",
	}
	for _, flag := range wantFlags {
		if !slices.Contains(args, flag) {
			t.Errorf("%s missing from %v", flag, args)
		}
	}

	// The two trailing positional arguments are the summary and the body.
	summary, body, ok := notifySendPositional(args)
	if !ok {
		t.Fatalf("could not read summary/body from %v", args)
	}
	if summary != "Sunrise" {
		t.Errorf("summary = %q, want %q", summary, "Sunrise")
	}
	// notify-send has no subtitle, so the artist is folded into the body.
	if body != "Aurora — Dawn" {
		t.Errorf("body = %q, want %q", body, "Aurora — Dawn")
	}
}

func TestNotifySendArgsOmitsBlankArtist(t *testing.T) {
	// Only the mpv-idle / not-running states carry a blank artist.
	args := Track{Title: "muse", Degraded: "Nothing playing"}.notifySendArgs()

	summary, body, ok := notifySendPositional(args)
	if !ok {
		t.Fatalf("could not read summary/body from %v", args)
	}
	if summary != "muse" {
		t.Errorf("summary = %q, want %q", summary, "muse")
	}
	if body != "Nothing playing" {
		t.Errorf("body = %q, want %q (no leading separator)", body, "Nothing playing")
	}
	if strings.HasPrefix(body, " — ") {
		t.Errorf("body %q starts with a dangling artist separator", body)
	}
}

func TestNotifySendArgsBodyNeverEmpty(t *testing.T) {
	states := []Track{
		{Title: "muse", Degraded: "Nothing playing"},
		{Title: "muse", Degraded: "muse is not running"},
		{Title: "Unknown track", Artist: "Unknown artist", Degraded: "Not in library"},
		{Title: "A", Paused: true},
	}

	for _, n := range states {
		args := n.notifySendArgs()
		summary, body, ok := notifySendPositional(args)
		if !ok {
			t.Errorf("could not read summary/body for %+v (args %v)", n, args)
			continue
		}
		if summary == "" {
			t.Errorf("empty summary for %+v (args %v)", n, args)
		}
		if body == "" {
			t.Errorf("empty body for %+v (args %v)", n, args)
		}
	}
}

// notifySendPositional returns the trailing summary and body arguments, i.e.
// the last two entries that are not --flag=value options.
func notifySendPositional(args []string) (string, string, bool) {
	var positional []string
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			continue
		}
		positional = append(positional, a)
	}
	if len(positional) != 2 {
		return "", "", false
	}
	return positional[0], positional[1], true
}

// TestSendIsFireAndForget puts a notify-send stub on $PATH that records its
// argv and then sleeps. Send must return before the sleep finishes, proving it
// never blocks on Wait.
func TestSendIsFireAndForget(t *testing.T) {
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	doneFile := filepath.Join(dir, "argv.done")
	stub := filepath.Join(dir, "notify-send")

	// The stub records its argv immediately, then sleeps far longer than the
	// deadline below, so a Send that waited on it could not return in time.
	// The argv file is followed by a separate marker file, so the poll below
	// can never observe a partially flushed argv list. Only shell builtins are
	// used: $PATH is reduced to the temp dir so no real notify-send can
	// shadow the stub, which also means no external binary is resolvable.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvFile + "\n: > " + doneFile + "\nsleep 30\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	track := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}
	want := track.notifySendArgs()

	start := time.Now()
	if err := Send(track); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Send took %v, want it to return without waiting on the stub", elapsed)
	}

	// The stub writes its argv before sleeping. Poll generously: process spawn
	// is slow under -race with the whole suite running in parallel.
	var recorded string
	for range 500 {
		if _, err := os.Stat(doneFile); err == nil {
			data, err := os.ReadFile(argvFile)
			if err != nil {
				t.Fatalf("reading %s after the stub signalled completion: %v", argvFile, err)
			}
			recorded = string(data)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if recorded == "" {
		t.Fatalf("stub %s never recorded its argv to %s", stub, argvFile)
	}

	got := strings.Split(strings.TrimSuffix(recorded, "\n"), "\n")
	if !slices.Equal(got, want) {
		t.Fatalf("stub argv = %q, want %q", got, want)
	}
}

// TestSendReapsBackend puts a notify-send stub on $PATH that exits at once,
// then checks that the exited child does not linger as a zombie: Send must
// Wait on the backend in the background rather than Release it, or every
// track-change banner in a TUI session leaks a process-table entry until muse
// exits.
func TestSendReapsBackend(t *testing.T) {
	dir := t.TempDir()
	doneFile := filepath.Join(dir, "done")
	stub := filepath.Join(dir, "notify-send")
	script := "#!/bin/sh\n: > " + doneFile + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	if err := Send(Track{Title: "Sunrise"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Poll generously: process spawn and the background Wait are slow under
	// -race with the whole suite running in parallel.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(doneFile); err == nil && len(zombieChildren(t, "notify-send")) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("notify-send stub not reaped: zombie children %v", zombieChildren(t, "notify-send"))
}

// zombieChildren returns the PIDs of this process's children named comm that
// have exited but not been waited on (state Z in /proc/<pid>/stat).
func zombieChildren(t *testing.T, comm string) []string {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("reading /proc: %v", err)
	}
	self := strconv.Itoa(os.Getpid())
	var zombies []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue // the process exited between ReadDir and ReadFile
		}
		// Format: "pid (comm) state ppid ...". comm may contain spaces or
		// parentheses, so split on the last ')'.
		stat := string(data)
		open, closing := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
		if open < 0 || closing < open {
			continue
		}
		fields := strings.Fields(stat[closing+1:])
		if len(fields) < 2 {
			continue
		}
		if stat[open+1:closing] == comm && fields[0] == "Z" && fields[1] == self {
			zombies = append(zombies, e.Name())
		}
	}
	return zombies
}

func TestSendMissingBinary(t *testing.T) {
	// Empty PATH so the LookPath cannot resolve a real notify-send.
	t.Setenv("PATH", "")

	err := Send(Track{Title: "muse"})
	if err == nil {
		t.Fatal("Send with no notify-send available succeeded, want error")
	}
	if !strings.Contains(err.Error(), "notify-send") {
		t.Errorf("error should mention notify-send, got: %v", err)
	}
	if !strings.Contains(err.Error(), "libnotify") {
		t.Errorf("error should include install instructions, got: %v", err)
	}
	if err := Available(); err == nil {
		t.Fatal("Available with no notify-send succeeded, want error")
	}
}
