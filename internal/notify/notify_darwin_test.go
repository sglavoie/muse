//go:build darwin

package notify

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAlerterArgs(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	args := Track{Title: "Sunrise", Artist: "Aurora", Album: "Dawn"}.alerterArgs()

	wantPairs := map[string]string{
		"--title":    "Sunrise",
		"--subtitle": "Aurora",
		"--message":  "Dawn",
		"--group":    alerterGroup,
		"--sender":   "com.apple.Terminal",
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

// TestAlerterSender covers the $TERM_PROGRAM detection that replaced the
// hardcoded com.apple.Terminal constant.
func TestAlerterSender(t *testing.T) {
	tests := []struct {
		termProgram string
		want        string
	}{
		{"Apple_Terminal", "com.apple.Terminal"},
		{"iTerm.app", "com.googlecode.iterm2"},
		{"ghostty", "com.mitchellh.ghostty"},
		{"WezTerm", "com.github.wez.wezterm"},
		{"", defaultSender},
		{"SomethingUnheardOf", defaultSender},
		// An unknown value that already looks like a bundle ID is used as-is.
		{"com.example.MyTerm", "com.example.MyTerm"},
	}

	for _, tt := range tests {
		t.Run(tt.termProgram, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", tt.termProgram)
			if got := alerterSender(); got != tt.want {
				t.Errorf("alerterSender() with TERM_PROGRAM=%q = %q, want %q", tt.termProgram, got, tt.want)
			}
		})
	}
}

// TestResolveAlerterPrefersAbsolutePaths pins the Karabiner-critical ordering:
// the absolute Homebrew paths win over $PATH, and the Intel prefix is tried
// after the Apple Silicon one.
func TestResolveAlerterPrefersAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	silicon := filepath.Join(dir, "opt-homebrew-alerter")
	intel := filepath.Join(dir, "usr-local-alerter")
	for _, p := range []string{silicon, intel} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	// A real alerter on $PATH must not shadow the absolute paths.
	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "alerter"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write path stub: %v", err)
	}
	t.Setenv("PATH", pathDir)

	original := alerterPaths
	t.Cleanup(func() { alerterPaths = original })

	alerterPaths = []string{silicon, intel}
	if got, err := resolveAlerter(); err != nil || got != silicon {
		t.Fatalf("resolveAlerter() = %q, %v; want %q", got, err, silicon)
	}

	// With the Apple Silicon prefix absent, the Intel prefix is next.
	alerterPaths = []string{filepath.Join(dir, "missing"), intel}
	if got, err := resolveAlerter(); err != nil || got != intel {
		t.Fatalf("resolveAlerter() = %q, %v; want %q", got, err, intel)
	}

	// With neither absolute path present, $PATH is the fallback.
	alerterPaths = []string{filepath.Join(dir, "missing")}
	got, err := resolveAlerter()
	if err != nil {
		t.Fatalf("resolveAlerter() with only a PATH install: %v", err)
	}
	if filepath.Dir(got) != pathDir {
		t.Fatalf("resolveAlerter() = %q, want a binary under %q", got, pathDir)
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

// TestSendIsFireAndForget points alerterPaths at a stub that records its argv
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

	original := alerterPaths
	alerterPaths = []string{stub}
	t.Cleanup(func() { alerterPaths = original })

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
	original := alerterPaths
	alerterPaths = []string{filepath.Join(t.TempDir(), "does-not-exist")}
	t.Cleanup(func() { alerterPaths = original })

	// Empty PATH so the LookPath fallback cannot resolve a real alerter.
	t.Setenv("PATH", "")

	if err := Send(Track{Title: "muse"}); err == nil {
		t.Fatal("Send with no alerter available succeeded, want error")
	}
	if err := Available(); err == nil {
		t.Fatal("Available with no alerter succeeded, want error")
	}
}
