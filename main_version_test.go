package main

import (
	"runtime/debug"
	"testing"
)

// buildInfo builds a *debug.BuildInfo with the given main module version and
// vcs settings, mimicking what the toolchain embeds.
func buildInfo(mainVersion string, settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		info := &debug.BuildInfo{Settings: settings}
		info.Main.Version = mainVersion
		return info, true
	}
}

func TestResolveVersionFrom_LdflagsValueWins(t *testing.T) {
	got := resolveVersionFrom("v1.2.3", buildInfo("v9.9.9"))
	if got != "v1.2.3" {
		t.Fatalf("version = %q, want v1.2.3", got)
	}
}

func TestResolveVersionFrom_ModuleVersionWhenLdflagsUnset(t *testing.T) {
	got := resolveVersionFrom("dev", buildInfo("v0.1.0"))
	if got != "v0.1.0" {
		t.Fatalf("version = %q, want v0.1.0", got)
	}
}

func TestResolveVersionFrom_RevisionWhenModuleIsDevel(t *testing.T) {
	got := resolveVersionFrom("dev", buildInfo("(devel)",
		debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123"},
		debug.BuildSetting{Key: "vcs.modified", Value: "false"},
	))
	if got != "dev+0123456789ab" {
		t.Fatalf("version = %q, want dev+0123456789ab", got)
	}
}

func TestResolveVersionFrom_DirtyRevisionIsMarked(t *testing.T) {
	got := resolveVersionFrom("dev", buildInfo("(devel)",
		debug.BuildSetting{Key: "vcs.revision", Value: "abc123"},
		debug.BuildSetting{Key: "vcs.modified", Value: "true"},
	))
	if got != "dev+abc123.dirty" {
		t.Fatalf("version = %q, want dev+abc123.dirty", got)
	}
}

func TestResolveVersionFrom_DevWhenNoModuleVersionOrRevision(t *testing.T) {
	got := resolveVersionFrom("dev", buildInfo("(devel)"))
	if got != devVersion {
		t.Fatalf("version = %q, want %q", got, devVersion)
	}
}

func TestResolveVersionFrom_DevWhenBuildInfoUnavailable(t *testing.T) {
	got := resolveVersionFrom("dev", func() (*debug.BuildInfo, bool) { return nil, false })
	if got != devVersion {
		t.Fatalf("version = %q, want %q", got, devVersion)
	}
}

func TestResolveVersionFrom_EmptyLdflagsValueFallsThrough(t *testing.T) {
	got := resolveVersionFrom("", buildInfo("v0.1.0"))
	if got != "v0.1.0" {
		t.Fatalf("version = %q, want v0.1.0", got)
	}
}

// resolveVersion is what main_cli.go prints; under `go test` the binary carries
// real build info, so this asserts only that it never returns an empty string.
func TestResolveVersion_NotEmpty(t *testing.T) {
	if resolveVersion() == "" {
		t.Fatal("resolveVersion returned an empty string")
	}
}
