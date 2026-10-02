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
