package library

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
