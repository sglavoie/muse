# Public launch checklist

Review started on 2026-10-21. This separates publishing source from distributing
prebuilt desktop binaries. The GitHub repository is still private; no release,
visibility change, or history rewrite has been performed by this review.

## Already present

- MIT license, contributor guide, changelog, example configuration, setup and
  user guides, and architecture documentation.
- Linux CI with build, formatting, lint, race tests, and real mpv tests.
- Pinned Ghostty, Zig, XcodeGen, and linter versions; checksums for downloaded
  Zig and XcodeGen archives; a documented macOS source build.
- Ghostty and theme attribution, and app icons generated from committed code.

## Added during this review

- Security reporting policy, bug and feature issue forms, and a PR template.
- Monthly grouped Dependabot updates for Go modules and GitHub Actions.
- Read-only CI repository permissions and a reachable-vulnerability check.
- `golang.org/x/text` v0.39.0 and its required dependency updates, addressing
  [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970). A fresh local
  `govulncheck` reports no reachable vulnerabilities; it still reports one
  advisory in a required module whose vulnerable code is not called.
- Fixed the stale `TTApp/Ghostty` attribution path to `MuseApp/Ghostty`.
- A reproducible desktop recording workflow using generated audio and a
  disposable library. See [demo instructions](demo/README.md).
- Explicit Option-as-Alt settings in the macOS keyboard tests, including
  coverage with that setting disabled, instead of relying on host defaults.

## Before making the source public

- [x] **Confirm the notification sounds' provenance.** Sébastien Lavoie
  confirmed on 2026-10-21 that he generated all four sounds. Their authorship
  and project license are recorded in the
  [sound asset notes](../internal/player/sounds/README.md).
- [ ] **Review the history you will publish.** Gitleaks v8.30.0 scanned all
  locally available refs (1,004 commits reported by the scanner) and found no leaks.
  This is a scanner result, not a guarantee. Commit author identities and
  email addresses remain part of the history. Remote-only branches, GitHub
  issues, attachments, and Actions artifacts were not included in that scan.
- [x] **Capture the desktop demo.** Recorded on a MacBook over SSH; all nine
  scene checks and the live playback check pass. The README links its desktop
  still and embeds a separate 22-second animated SVG generated from the real
  TUI with in-memory fixture data. The captured screens were visually reviewed;
  include the media in the final publication review.
- [x] **Run local validation.** Formatting, lint with Go 1.26.8, the full Go
  race suite, the reachable-vulnerability scan, and all 129 macOS tests pass.
  The Release app builds successfully. The pinned linter cannot read Go 1.27
  export data; use Go 1.26 for the current lint gate.
- [ ] **Review and commit the diff.** The changes are local and uncommitted.
- [ ] **Choose the launch release.** There is a `v0.1.0` tag but no GitHub
  Releases. Check whether that tag represents the version you want new users
  to get from `go install ...@latest`; otherwise tag a new release after the
  changes land and write release notes.
- [ ] **Make the visibility change explicitly**, then enable private
  vulnerability reporting, secret scanning/push protection where available,
  and Dependabot alerts. Verify the security policy's private reporting link.
  GitHub documents [private vulnerability reporting setup](https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository).
- [ ] **Verify from a fresh public clone.** Check installation, README media,
  relative links, issue forms, and CI as an unauthenticated visitor.

## Before distributing prebuilt desktop binaries

- [ ] Generate and bundle third-party license notices for the actual Go and
  Ghostty dependency graph, including embedded fonts and native libraries.
  `NOTICE-ghostty` covers the ported Swift and themes; it is not a complete
  inventory of everything linked into GhosttyKit.
- [ ] Choose a release pipeline and supported architectures. Build and test
  the artifacts, provide checksums, and use the documented Developer ID
  signing/notarization flow for downloaded macOS apps.
- [ ] Test a downloaded app on a clean Mac, including runtime dependency
  detection. The app still requires separately installed mpv and a metadata
  reader; the current build does not bundle those programs.

## Useful follow-ups

- Add native macOS CI once the Ghostty build/cache cost is understood.
- Protect `main` with the CI checks you want to require; it is unprotected now.
- Choose whether to add Discussions, a support policy, or a code of conduct as
  the contributor community grows. These are project decisions, not prerequisites
  for publishing the source.
