package notify

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMessage(t *testing.T) {
	tests := []struct {
		name  string
		track Track
		want  string
	}{
		{"album", Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}, "Dawn"},
		{"paused album", Track{Title: "Sunrise", Album: "Dawn", Paused: true}, "Paused — Dawn"},
		{"no album", Track{Title: "Sunrise", Artist: "Aurora"}, "Now playing"},
		{"degraded wins", Track{Title: "Sunrise", Album: "Dawn", Degraded: "Not in library"}, "Not in library"},
		{"paused degraded", Track{Title: "Sunrise", Degraded: "Not in library", Paused: true}, "Paused — Not in library"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.track.Message(); got != tt.want {
				t.Errorf("Message() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummaryLine(t *testing.T) {
	n := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn", Paused: true}

	want := "Sunrise — Aurora (Paused — Dawn)"
	if got := n.SummaryLine(); got != want {
		t.Fatalf("SummaryLine() = %q, want %q", got, want)
	}

	idle := Track{Title: "muse", Degraded: "Nothing playing"}
	if got := idle.SummaryLine(); got != "muse (Nothing playing)" {
		t.Fatalf("idle SummaryLine() = %q, want %q", got, "muse (Nothing playing)")
	}
}

func TestAlerterArgs(t *testing.T) {
	args := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}.alerterArgs()

	wantPairs := map[string]string{
		"--title":    "Sunrise",
		"--subtitle": "Aurora",
		"--message":  "Dawn",
		"--group":    alerterGroup,
		"--sender":   alerterSender,
		"--timeout":  alerterTimeout,
	}
	for flag, want := range wantPairs {
		got, ok := argValue(args, flag)
		if !ok {
			t.Errorf("%s missing from %v", flag, args)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
}

func TestAlerterArgsOmitsBlankSubtitle(t *testing.T) {
	// Only the mpv-idle / not-running states carry a blank artist.
	args := Track{Title: "muse", Degraded: "Nothing playing"}.alerterArgs()

	if slices.Contains(args, "--subtitle") {
		t.Errorf("--subtitle present for a blank artist: %v", args)
	}
	if got, _ := argValue(args, "--message"); got != "Nothing playing" {
		t.Errorf("--message = %q, want %q", got, "Nothing playing")
	}
}

func TestAlerterArgsMessageNeverEmpty(t *testing.T) {
	states := []Track{
		{Title: "muse", Degraded: "Nothing playing"},
		{Title: "muse", Degraded: "muse is not running"},
		{Title: "Unknown track", Artist: "Unknown artist", Degraded: "Not in library"},
		{Title: "A", Paused: true},
	}

	for _, n := range states {
		args := n.alerterArgs()
		got, ok := argValue(args, "--message")
		if !ok || got == "" {
			t.Errorf("empty --message for %+v (args %v)", n, args)
		}
		if title, ok := argValue(args, "--title"); !ok || title == "" {
			t.Errorf("empty --title for %+v (args %v)", n, args)
		}
	}
}

// argValue returns the value following flag in an argv slice.
func argValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// TestSendIsFireAndForget points alerterPath at a stub that records its argv
// and then sleeps. Send must return before the sleep finishes, proving it
// never calls Wait.
func TestSendIsFireAndForget(t *testing.T) {
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv")
	stub := filepath.Join(dir, "alerter-stub")

	// The stub records its argv immediately, then sleeps far longer than the
	// deadline below, so a Send that waited on it could not return in time.
	// The record is written to a scratch file and renamed so the poll below
	// can never observe a partially flushed argv list.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argvFile + ".tmp\nmv " + argvFile + ".tmp " + argvFile + "\nsleep 30\n"
	if err := os.WriteFile(stub, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	original := alerterPath
	alerterPath = stub
	t.Cleanup(func() { alerterPath = original })

	track := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}
	want := track.alerterArgs()

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
		data, err := os.ReadFile(argvFile)
		if err == nil && len(data) > 0 {
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

func TestSendMissingBinary(t *testing.T) {
	original := alerterPath
	alerterPath = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { alerterPath = original })

	// Empty PATH so the LookPath fallback cannot resolve a real alerter.
	t.Setenv("PATH", "")

	if err := Send(Track{Title: "muse"}); err == nil {
		t.Fatal("Send with no alerter available succeeded, want error")
	}
}
