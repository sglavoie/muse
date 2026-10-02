package main

import (
	"fmt"
	"io"
	"runtime/debug"
)

// BuildTime is set at build time via -ldflags "-X main.BuildTime=…" by
// `just release`. Builds without it fall back to the VCS time Go embeds.
var BuildTime string

// buildInfoReport is what the hidden `muse build-info` prints.
type buildInfoReport struct {
	Version  string
	Revision string
	Time     string
}

// String formats the report as one line of key=value pairs. A value muse
// cannot know is left empty rather than omitted, so the keys are always there.
func (r buildInfoReport) String() string {
	return fmt.Sprintf("version=%s revision=%s time=%s", r.Version, r.Revision, r.Time)
}

// printBuildInfo writes the build-info line muse.app's binary resolver reads.
func printBuildInfo(w io.Writer) {
	_, _ = fmt.Fprintln(w, resolveBuildInfo(Version, BuildTime, debug.ReadBuildInfo))
}

// resolveBuildInfo combines the -ldflags values with the embedded build info.
// The version follows resolveVersionFrom; the time prefers the -ldflags value
// and falls back to vcs.time, which both `go install` from a checkout and
// `go build` record.
func resolveBuildInfo(ldflagsVersion, ldflagsTime string, readBuildInfo func() (*debug.BuildInfo, bool)) buildInfoReport {
	report := buildInfoReport{
		Version: resolveVersionFrom(ldflagsVersion, readBuildInfo),
		Time:    ldflagsTime,
	}
	info, ok := readBuildInfo()
	if !ok {
		return report
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			report.Revision = setting.Value
		case "vcs.time":
			if report.Time == "" {
				report.Time = setting.Value
			}
		}
	}
	return report
}
