package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveBuildInfo_LdflagsWin(t *testing.T) {
	got := resolveBuildInfo("v1.2.3", "2026-10-02T12:00:00Z", buildInfo("(devel)",
		debug.BuildSetting{Key: "vcs.revision", Value: "abc123"},
		debug.BuildSetting{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
	))
	want := "version=v1.2.3 revision=abc123 time=2026-10-02T12:00:00Z"
	if got.String() != want {
		t.Fatalf("build info = %q, want %q", got, want)
	}
}

func TestResolveBuildInfo_FallsBackToVCSTime(t *testing.T) {
	got := resolveBuildInfo("dev", "", buildInfo("(devel)",
		debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123"},
		debug.BuildSetting{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
	))
	want := "version=dev+0123456789ab revision=0123456789abcdef0123 time=2026-01-01T00:00:00Z"
	if got.String() != want {
		t.Fatalf("build info = %q, want %q", got, want)
	}
}

func TestResolveBuildInfo_NoBuildInfoKeepsKeys(t *testing.T) {
	got := resolveBuildInfo("dev", "", func() (*debug.BuildInfo, bool) { return nil, false })
	want := "version=dev revision= time="
	if got.String() != want {
		t.Fatalf("build info = %q, want %q", got, want)
	}
}
