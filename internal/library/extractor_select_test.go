package library

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubPATH writes each named shell script into a fresh temp dir and makes that
// dir the entire PATH, so only the stubs are reachable and whatever the host
// happens to have installed cannot leak into the assertion.
func stubPATH(t *testing.T, scripts map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

const (
	workingExifTool = "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n"
	failingExifTool = "#!/bin/sh\nexit 1\n"
	garbageExifTool = "#!/bin/sh\nprintf 'not a version\\n'\nexit 0\n"
	stubFFProbe     = "#!/bin/sh\nexit 0\n"
)

func TestSelectExtractor_PrefersExifTool(t *testing.T) {
	stubPATH(t, map[string]string{
		"exiftool": workingExifTool,
		"ffprobe":  stubFFProbe,
	})

	extractor, backend, err := SelectExtractor(context.Background())
	if err != nil {
		t.Fatalf("expected a backend when exiftool works, got: %v", err)
	}
	if backend != BackendExifTool {
		t.Errorf("backend = %q, want %q", backend, BackendExifTool)
	}
	if _, ok := extractor.(ExifToolExtractor); !ok {
		t.Errorf("extractor = %T, want ExifToolExtractor", extractor)
	}
}

func TestSelectExtractor_FallsBackToFFProbeWhenExifToolMissing(t *testing.T) {
	stubPATH(t, map[string]string{"ffprobe": stubFFProbe})

	extractor, backend, err := SelectExtractor(context.Background())
	if err != nil {
		t.Fatalf("ffprobe alone must be enough, got: %v", err)
	}
	if backend != BackendFFProbe {
		t.Errorf("backend = %q, want %q", backend, BackendFFProbe)
	}
	if _, ok := extractor.(FFProbeExtractor); !ok {
		t.Errorf("extractor = %T, want FFProbeExtractor", extractor)
	}
}

// TestSelectExtractor_FallsBackWhenExifToolIsBroken covers the subtle case:
// LookPath succeeds but the binary cannot run (missing Perl) or does not emit a
// version. Selecting ExifToolExtractor here would scan the whole library into
// empty metadata, so both broken shapes must reach ffprobe — and this is also
// what keeps SelectExtractor in agreement with config.CheckPrereqs, which warns
// and continues in exactly these two cases.
func TestSelectExtractor_FallsBackWhenExifToolIsBroken(t *testing.T) {
	cases := map[string]string{
		"exits non-zero":    failingExifTool,
		"unparseable -ver":  garbageExifTool,
		"empty -ver output": "#!/bin/sh\nexit 0\n",
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			stubPATH(t, map[string]string{
				"exiftool": script,
				"ffprobe":  stubFFProbe,
			})

			extractor, backend, err := SelectExtractor(context.Background())
			if err != nil {
				t.Fatalf("a broken exiftool must fall through to ffprobe, got: %v", err)
			}
			if backend != BackendFFProbe {
				t.Errorf("backend = %q, want %q", backend, BackendFFProbe)
			}
			if _, ok := extractor.(FFProbeExtractor); !ok {
				t.Errorf("extractor = %T, want FFProbeExtractor", extractor)
			}
		})
	}
}

func TestSelectExtractor_ErrorsWhenNeitherIsAvailable(t *testing.T) {
	stubPATH(t, nil)

	extractor, backend, err := SelectExtractor(context.Background())
	if err == nil {
		t.Fatalf("expected an error with no backend on PATH, got %T / %q", extractor, backend)
	}
	if extractor != nil {
		t.Errorf("extractor should be nil on error, got %T", extractor)
	}
	if backend != "" {
		t.Errorf("backend should be empty on error, got %q", backend)
	}
	msg := err.Error()
	for _, want := range []string{"exiftool", "ffprobe", "brew install ffmpeg", "libimage-exiftool-perl"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q so the user knows how to fix it, got: %s", want, msg)
		}
	}
}

// hangingStub returns a stub script that never returns until it is killed.
// `sleep` is spelled absolutely and reached through `exec` for two reasons:
// stubPATH replaces $PATH entirely, so a bare `sleep` would exit 127 and make
// the stub look merely broken rather than hung; and `exec` replaces the shell,
// so the kill lands on the process actually holding the probe's stdout pipe
// and leaves no orphan behind.
func hangingStub(t *testing.T) string {
	t.Helper()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}
	return "#!/bin/sh\nexec " + sleep + " 600\n"
}

// shortenBackendProbeTimeout makes the bounded probes finish fast enough for a
// unit test while leaving plenty of room for a stub shell script to start.
func shortenBackendProbeTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	original := backendProbeTimeout
	t.Cleanup(func() { backendProbeTimeout = original })
	backendProbeTimeout = d
}

// TestSelectExtractor_HungExifToolFallsBackWithinDeadline pins the reason the
// probe carries its own deadline: both callers (runTUI and muse check) pass a
// context with no deadline, so an exiftool that resolves through LookPath and
// then never returns — a wedged Perl, a filesystem stall — would otherwise hang
// startup forever with a blank terminal. It must instead be treated like any
// other broken install and fall through to ffprobe, bounded.
func TestSelectExtractor_HungExifToolFallsBackWithinDeadline(t *testing.T) {
	hung := hangingStub(t)
	stubPATH(t, map[string]string{
		"exiftool": hung,
		"ffprobe":  stubFFProbe,
	})
	shortenBackendProbeTimeout(t, time.Second)

	done := make(chan struct{})
	var (
		backend string
		err     error
	)
	go func() {
		defer close(done)
		_, backend, err = SelectExtractor(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("SelectExtractor never returned with a hung exiftool: the probe is unbounded")
	}

	if err != nil {
		t.Fatalf("a hung exiftool must fall through to ffprobe, got: %v", err)
	}
	if backend != BackendFFProbe {
		t.Errorf("backend = %q, want %q", backend, BackendFFProbe)
	}
}

// TestSelectExtractor_RejectsBrokenFFProbe covers the fallback's own broken
// shape. ffprobe was previously accepted from LookPath alone, so an ffprobe
// with missing shared libraries became the metadata backend and every track
// scanned into filename-derived placeholders with no error anywhere. With no
// usable backend at all the only honest answer is the install-instruction
// error.
func TestSelectExtractor_RejectsBrokenFFProbe(t *testing.T) {
	stubPATH(t, map[string]string{"ffprobe": "#!/bin/sh\nexit 127\n"})

	extractor, backend, err := SelectExtractor(context.Background())
	if err == nil {
		t.Fatalf("an ffprobe that cannot run must not be selected, got %T / %q", extractor, backend)
	}
	if extractor != nil {
		t.Errorf("extractor should be nil when no backend is usable, got %T", extractor)
	}
	if backend != "" {
		t.Errorf("backend should be empty when no backend is usable, got %q", backend)
	}
	if !strings.Contains(err.Error(), "ffprobe") {
		t.Errorf("error should still explain how to get a working ffprobe, got: %s", err)
	}
}

func TestFFProbeUsable(t *testing.T) {
	t.Run("working stub", func(t *testing.T) {
		stubPATH(t, map[string]string{"ffprobe": stubFFProbe})
		if err := FFProbeUsable(context.Background()); err != nil {
			t.Errorf("expected nil for an ffprobe that runs, got: %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		stubPATH(t, nil)
		err := FFProbeUsable(context.Background())
		if err == nil {
			t.Fatal("expected an error when ffprobe is absent")
		}
		if !strings.Contains(err.Error(), "brew install ffmpeg") {
			t.Errorf("error should say how to install ffprobe, got: %s", err)
		}
	})

	t.Run("present but failing", func(t *testing.T) {
		stubPATH(t, map[string]string{"ffprobe": "#!/bin/sh\nexit 127\n"})
		err := FFProbeUsable(context.Background())
		if err == nil {
			t.Fatal("expected an error when 'ffprobe -version' exits non-zero")
		}
		if !strings.Contains(err.Error(), "installed but failed to run") {
			t.Errorf("error should distinguish broken from absent, got: %s", err)
		}
	})

	t.Run("hung", func(t *testing.T) {
		stubPATH(t, map[string]string{"ffprobe": hangingStub(t)})
		shortenBackendProbeTimeout(t, time.Second)

		start := time.Now()
		err := FFProbeUsable(context.Background())
		if err == nil {
			t.Fatal("expected an error when ffprobe never returns")
		}
		if elapsed := time.Since(start); elapsed > 10*time.Second {
			t.Errorf("probe took %s, want it bounded by the probe timeout", elapsed)
		}
		if !strings.Contains(err.Error(), "did not respond") {
			t.Errorf("error should name the timeout, got: %s", err)
		}
	})
}

func TestExifToolUsable(t *testing.T) {
	t.Run("working stub", func(t *testing.T) {
		stubPATH(t, map[string]string{"exiftool": workingExifTool})
		if err := ExifToolUsable(context.Background()); err != nil {
			t.Errorf("expected nil for a stub emitting a valid version, got: %v", err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		stubPATH(t, nil)
		err := ExifToolUsable(context.Background())
		if err == nil {
			t.Fatal("expected an error when exiftool is absent")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "perl") {
			t.Errorf("error should mention the Perl dependency, got: %s", err)
		}
	})

	t.Run("unparseable version", func(t *testing.T) {
		stubPATH(t, map[string]string{"exiftool": garbageExifTool})
		err := ExifToolUsable(context.Background())
		if err == nil {
			t.Fatal("expected an error when -ver output is not a version")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "version") {
			t.Errorf("error should mention the version, got: %s", err)
		}
	})
}
