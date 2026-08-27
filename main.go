package main

import "runtime/debug"

// devVersion is the placeholder reported when neither -ldflags nor the
// embedded build info yields anything better.
const devVersion = "dev"

// Version is set at build time via -ldflags "-X main.Version=x.y.z".
var Version = devVersion

// resolveVersion reports the version string `muse --version` prints.
func resolveVersion() string {
	return resolveVersionFrom(Version, debug.ReadBuildInfo)
}

// resolveVersionFrom resolves the version from the -ldflags value and the
// embedded build info, in that order of precedence:
//
//  1. the -ldflags value, when `just release <version>` stamped one in;
//  2. the module version recorded by `go install github.com/sglavoie/muse@v…`;
//  3. the VCS revision recorded when installing from a local checkout, since
//     that case records "(devel)" as the module version;
//  4. "dev".
func resolveVersionFrom(ldflagsVersion string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if ldflagsVersion != "" && ldflagsVersion != devVersion {
		return ldflagsVersion
	}

	info, ok := readBuildInfo()
	if !ok {
		return devVersion
	}

	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}

	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return devVersion
	}

	if len(revision) > 12 {
		revision = revision[:12]
	}
	version := devVersion + "+" + revision
	if modified {
		version += ".dirty"
	}
	return version
}

// upsertBatchSize is the number of tracks per database upsert batch during
// library scanning and rescanning.
const upsertBatchSize = 500
