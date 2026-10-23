# List available recipes (default when running `just` with no args)
default:
    @just --list

# Build the muse binary
build:
    go build -o muse .

# Install muse to the Go bin directory
install:
    go install .

# Run all tests
test:
    go test -race -count=1 ./...

# Fast local feedback; reuse passing results and leave race checking to test/CI
test-fast:
    go test ./...

# CI-style race run: stop all packages at the first failure and retain timings
test-ci:
    python3 scripts/ci_test.py --json-output "${TMPDIR:-/tmp}/muse-ci-tests.jsonl"

# Verify real mpv continuity, queue replacement and gapless advance (silent output)
test-playback:
    MUSE_REQUIRE_MPV=1 go test -race -count=1 -tags integration -run RealMPV -timeout 5m ./internal/player/

# Measure typing, individual keystrokes, rendering and sorting at 10k/100k tracks
bench-ui:
    go test ./internal/tui -run '^$' -bench '^BenchmarkLibraryInteraction$' -benchmem

# Run tests with verbose output
test-v:
    go test -race -count=1 -v ./...

# Run tests for a specific package (e.g., just test-pkg internal/tui)
test-pkg pkg:
    go test -race -count=1 -v ./{{pkg}}/...

# Pinned golangci-lint version — keep in sync with the lint job's `version` in
# .github/workflows/ci.yml
golangci_lint := "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1"

# Run the linter
lint:
    go vet ./...
    go run {{golangci_lint}} run

# Check reachable Go vulnerabilities against the current advisory database
security:
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

# Show complexity violations ignoring the debt list
lint-complexity:
    go run {{golangci_lint}} run -c .golangci-complexity.yml ./...

# Format Go files that are not gitignored (so never macos/.cache, where Zig fetches Go sources)
fmt:
    git ls-files -z --cached --others --exclude-standard '*.go' | xargs -0 gofmt -w

# Check formatting of Go files that are not gitignored, without modifying them (useful for CI)
fmt-check:
    @out="$(git ls-files -z --cached --others --exclude-standard '*.go' | xargs -0 gofmt -l)"; test -z "$out" || (echo "Files need formatting:" && echo "$out" && exit 1)

# Build with version tag (e.g., just release v1.2.3)
release version:
    go build -ldflags "-X main.Version={{version}} -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o muse .

# Build and vendor the pinned GhosttyKit.xcframework for the macOS app (outside CI)
mac-ghosttykit:
    ./macos/scripts/build-ghosttykit.sh

# Build macos/build/muse.app (Debug, or Release bundling a versioned muse; outside CI)
mac-app configuration="Debug":
    #!/usr/bin/env bash
    set -euo pipefail
    if [ "{{ configuration }}" = Release ]; then
        version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
        go build -ldflags "-X main.Version=$version -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o muse .
        export MUSE_BINARY="$PWD/muse"
    fi
    ./macos/scripts/build-app.sh {{ configuration }}

# Run the muse.app XCTest suite with xcodebuild test (outside CI)
mac-test:
    ./macos/scripts/test-app.sh

# Record the real desktop app with generated music in an isolated demo library
mac-demo:
    python3 macos/scripts/record-demo.py

# Render the README's animated vector tour (requires uv; no desktop UI)
readme-demo: build
    python3 scripts/capture_readme_demo.py
    uv run scripts/render_terminal_svg.py --animate macos/build/demo/readme-frames.json docs/demo/muse-tour.svg

# Build the Release muse.app and install it to /Applications (outside CI)
mac-install: (mac-app "Release")
    ./macos/scripts/install-app.sh

# Run the application
run *args:
    go run . {{args}}

# Generate test coverage report
cover:
    go test -race -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    @go tool cover -func=coverage.out | tail -1
    @echo "coverage.html written"

# Tidy module dependencies
tidy:
    go mod tidy

# Install git pre-commit hook (runs fmt-check, vet, and lint)
hooks:
    @printf '#!/bin/sh\njust fmt-check && go vet ./... && go run {{golangci_lint}} run\n' > .git/hooks/pre-commit
    @chmod +x .git/hooks/pre-commit
    @echo "pre-commit hook installed"

# Remove build artifacts and coverage reports
clean:
    rm -f muse coverage.out coverage.html

# Check crowded UI scenarios and export an SVG review gallery (requires uv)
review-ui: build
    uv run scripts/test_render_terminal_svg.py
    uv run scripts/review_tui.py
