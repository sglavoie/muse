# Contributing to muse

Thanks for your interest in muse. It is a personal project published in the
hope it is useful to others, so please open an issue to discuss anything
substantial before writing code — that way nobody spends an evening on a change
that does not fit.

Bug reports and small, focused fixes are always welcome.

## Getting set up

You need:

- **Go 1.26+**.
- **[`just`](https://github.com/casey/just)** — the task runner every command
  below goes through.
- **mpv** — required to run the app.
- **ExifTool** (recommended, fastest) or **ffprobe** — the metadata backend.
  ExifTool is preferred because it reads a batch in one process, about 2.4x
  faster than ffprobe on a real library (300 mp3/flac files: 819 ms vs 1.94 s);
  ffprobe is required only if ExifTool is absent or broken. See
  [Requirements](README.md#requirements) in the README. `just test` does not
  need any of them, but running the app does — and tests that exercise a real
  backend skip themselves when the binary is missing.

```sh
git clone https://github.com/sglavoie/muse
cd muse
just          # list every recipe
just build    # build ./muse
just run      # go run .
```

## Build, test, lint

Everything goes through `just`; there is no hidden setup step.

| Recipe | What it runs |
| --- | --- |
| `just build` | `go build -o muse .` |
| `just install` | `go install .` |
| `just run [args]` | `go run . [args]` |
| `just test` | `go test -race -count=1 ./...` |
| `just test-v` | the same, verbose |
| `just test-pkg internal/tui` | tests for one package, verbose |
| `just lint` | `go vet ./...` plus a pinned `golangci-lint run` |
| `just fmt` | `gofmt -w .` |
| `just fmt-check` | fails if anything needs formatting |
| `just cover` | coverage profile, HTML report, and the total |
| `just tidy` | `go mod tidy` |
| `just clean` | remove the binary and coverage reports |

Before opening a pull request, run:

```sh
just fmt-check
just lint
just test
```

The full suite runs with `-race`; keep it that way. golangci-lint is pinned to
an exact version in the `golangci_lint` variable at the top of the `justfile`,
so everyone lints against the same rules — bump it there and nowhere else.

### Pre-commit hook

Install the hook once and those checks run automatically on every commit:

```sh
just hooks
```

It writes `.git/hooks/pre-commit`, which runs `just fmt-check`, `go vet ./...`,
and the pinned `golangci-lint run`. The hook is generated from the `hooks`
recipe in the `justfile`, so re-run `just hooks` whenever that recipe changes
(for example after a golangci-lint version bump) to pick up the new command.

## Changes ship with tests

muse has roughly 1.7 lines of test code per line of source, and that ratio is
deliberate — it is what makes a TUI this size safe to change. **Every behaviour
change is expected to come with tests**, and every bug fix with a test that
fails before the fix and passes after it.

Practical guidance:

- The TUI model talks to narrow interfaces rather than concrete types, so
  player, database, config, and notifier dependencies are all fakeable. Look at
  the existing fakes in the package you are touching before writing a new one.
- Tests must not touch the real config file, database, music library, or
  network. Use `t.TempDir()` and the in-memory fakes.
- Goroutine-leak checking (`goleak`) is in place in several packages; if your
  change starts a goroutine, make sure it is stopped on shutdown.

### TUI snapshot tests

Layout changes are covered by a bespoke in-process harness (`muse tui-test`)
that drives the real Bubble Tea model, renders a normalized screen, and reports
layout problems as JSON. It never touches real state — no config, no database,
no mpv.

```sh
just build
./muse tui-test fixtures
./muse tui-test run --scenario docs/tui-testing/scenarios/baseline-layout.json
```

Read [`docs/tui-testing/README.md`](docs/tui-testing/README.md) for the
fixtures, the scenario format, the assertion set, and the interactive NDJSON
session mode. If you change rendering, add or update a scenario there; the
stable scenarios are also gated in `go test`, so `just test` will catch
regressions.

## Pull requests

- Keep commits focused and use [Conventional
  Commits](https://www.conventionalcommits.org) subjects (`feat:`, `fix:`,
  `docs:`, `test:`, `refactor:`, `chore:`) — the existing history follows this.
- Explain *why* in the commit body or the PR description; the diff already
  shows the *what*.
- Update the README, `config.example.toml`, or `docs/` when your change alters
  user-visible behaviour or configuration.
- Add a line to the `Unreleased` section of [`CHANGELOG.md`](CHANGELOG.md) for
  anything users would notice.
- There is no CI pipeline running on pull requests today, so the checks above
  are the only gate — please actually run them locally on both the code and the
  tests you touched.

## Reporting bugs

Open an issue with the output of `muse check`, your OS and terminal, the muse
version (`muse --version`), and the steps to reproduce. If the TUI rendered
wrongly, a `muse tui-test` scenario that reproduces it is the fastest possible
bug report.

## License

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE) that covers the project.
