package config

import (
	"fmt"
	"hash/crc32"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

// setEnv sets an environment variable and returns a cleanup function.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

// unsetEnv removes an environment variable for the duration of the test.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, had := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		}
	})
}

func wantFallbackRuntimeDir(t *testing.T) string {
	t.Helper()

	if current, err := user.Current(); err == nil && current.Uid != "" {
		return filepath.Join(os.TempDir(), appName+"-"+current.Uid)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%08x", appName, crc32.ChecksumIEEE([]byte(home))))
	}
	for _, key := range []string{"USER", "LOGNAME", "USERNAME"} {
		if v := os.Getenv(key); v != "" {
			return filepath.Join(os.TempDir(), fmt.Sprintf("%s-%08x", appName, crc32.ChecksumIEEE([]byte(v))))
		}
	}
	return filepath.Join(os.TempDir(), appName+"-runtime")
}

func TestResolveDirs_EnvOverrides(t *testing.T) {
	setEnv(t, "XDG_CONFIG_HOME", "/tmp/test-config")
	setEnv(t, "XDG_DATA_HOME", "/tmp/test-data")
	setEnv(t, "XDG_STATE_HOME", "/tmp/test-state")
	setEnv(t, "XDG_RUNTIME_DIR", "/tmp/test-runtime")

	dirs, err := ResolveDirs()
	if err != nil {
		t.Fatalf("ResolveDirs: %v", err)
	}

	want := map[string]string{
		"Config":  "/tmp/test-config/muse",
		"Data":    "/tmp/test-data/muse",
		"State":   "/tmp/test-state/muse",
		"Runtime": "/tmp/test-runtime/muse",
	}
	got := map[string]string{
		"Config":  dirs.Config,
		"Data":    dirs.Data,
		"State":   dirs.State,
		"Runtime": dirs.Runtime,
	}
	for field, wantPath := range want {
		if got[field] != wantPath {
			t.Errorf("%s = %q, want %q", field, got[field], wantPath)
		}
	}
}

func TestResolveDirs_Defaults(t *testing.T) {
	// Clear all XDG env vars so we exercise platform defaults.
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		unsetEnv(t, key)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	dirs, err := ResolveDirs()
	if err != nil {
		t.Fatalf("ResolveDirs: %v", err)
	}

	// Config: ~/.config/muse on both Linux and macOS.
	wantConfig := filepath.Join(home, ".config", "muse")
	if dirs.Config != wantConfig {
		t.Errorf("Config = %q, want %q", dirs.Config, wantConfig)
	}

	// Data: platform-specific default.
	var wantData string
	if runtime.GOOS == "darwin" {
		wantData = filepath.Join(home, "Library", "Application Support", "muse")
	} else {
		wantData = filepath.Join(home, ".local", "share", "muse")
	}
	if dirs.Data != wantData {
		t.Errorf("Data = %q, want %q", dirs.Data, wantData)
	}

	// State: ~/.local/state/muse on all platforms.
	wantState := filepath.Join(home, ".local", "state", "muse")
	if dirs.State != wantState {
		t.Errorf("State = %q, want %q", dirs.State, wantState)
	}

	// Runtime: user-specific temp-based fallback.
	wantRuntime := wantFallbackRuntimeDir(t)
	if dirs.Runtime != wantRuntime {
		t.Errorf("Runtime = %q, want %q", dirs.Runtime, wantRuntime)
	}
}

func TestResolveRuntimeDir_DefaultFallbackIsUserSpecific(t *testing.T) {
	unsetEnv(t, "XDG_RUNTIME_DIR")

	want := wantFallbackRuntimeDir(t)
	if got := ResolveRuntimeDir(); got != want {
		t.Fatalf("ResolveRuntimeDir() = %q, want %q", got, want)
	}
}

// TestResolveDirs_RelativeXDGPathsIgnored verifies that XDG env vars set to a
// non-absolute value are ignored per the XDG Base Directory Specification:
// otherwise muse would write config/state/runtime files under whatever the
// current working directory happens to be at launch time.
func TestResolveDirs_RelativeXDGPathsIgnored(t *testing.T) {
	setEnv(t, "XDG_CONFIG_HOME", "relative/cfg")
	setEnv(t, "XDG_DATA_HOME", "relative/data")
	setEnv(t, "XDG_STATE_HOME", "relative/state")
	setEnv(t, "XDG_RUNTIME_DIR", "relative/run")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	dirs, err := ResolveDirs()
	if err != nil {
		t.Fatalf("ResolveDirs: %v", err)
	}

	wantConfig := filepath.Join(home, ".config", "muse")
	if dirs.Config != wantConfig {
		t.Errorf("Config = %q, want %q (relative XDG_CONFIG_HOME must be ignored)", dirs.Config, wantConfig)
	}

	var wantData string
	if runtime.GOOS == "darwin" {
		wantData = filepath.Join(home, "Library", "Application Support", "muse")
	} else {
		wantData = filepath.Join(home, ".local", "share", "muse")
	}
	if dirs.Data != wantData {
		t.Errorf("Data = %q, want %q (relative XDG_DATA_HOME must be ignored)", dirs.Data, wantData)
	}

	wantState := filepath.Join(home, ".local", "state", "muse")
	if dirs.State != wantState {
		t.Errorf("State = %q, want %q (relative XDG_STATE_HOME must be ignored)", dirs.State, wantState)
	}

	wantRuntime := wantFallbackRuntimeDir(t)
	if dirs.Runtime != wantRuntime {
		t.Errorf("Runtime = %q, want %q (relative XDG_RUNTIME_DIR must be ignored)", dirs.Runtime, wantRuntime)
	}
}

func TestResolveRuntimeDir_RelativeEnvIgnored(t *testing.T) {
	setEnv(t, "XDG_RUNTIME_DIR", "not/absolute")
	want := wantFallbackRuntimeDir(t)
	if got := ResolveRuntimeDir(); got != want {
		t.Errorf("ResolveRuntimeDir() = %q, want %q (relative XDG_RUNTIME_DIR must be ignored)", got, want)
	}
}

func TestResolveDirs_PartialOverride(t *testing.T) {
	// Only override config; the rest should use defaults.
	for _, key := range []string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"} {
		unsetEnv(t, key)
	}
	setEnv(t, "XDG_CONFIG_HOME", "/custom/cfg")

	dirs, err := ResolveDirs()
	if err != nil {
		t.Fatalf("ResolveDirs: %v", err)
	}

	if dirs.Config != "/custom/cfg/muse" {
		t.Errorf("Config = %q, want %q", dirs.Config, "/custom/cfg/muse")
	}

	home, _ := os.UserHomeDir()
	var wantData string
	if runtime.GOOS == "darwin" {
		wantData = filepath.Join(home, "Library", "Application Support", "muse")
	} else {
		wantData = filepath.Join(home, ".local", "share", "muse")
	}
	if dirs.Data != wantData {
		t.Errorf("Data = %q, want %q", dirs.Data, wantData)
	}
}
