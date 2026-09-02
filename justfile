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

# Pinned golangci-lint version — keep in sync with .github/workflows/ci.yml
golangci_lint := "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.1"

# Run the linter
lint:
    go vet ./...
    go run {{golangci_lint}} run

# Show complexity violations ignoring the debt list
lint-complexity:
    go run {{golangci_lint}} run -c .golangci-complexity.yml ./...

# Format all Go files
fmt:
    gofmt -w .

# Check formatting without modifying files (useful for CI)
fmt-check:
    @test -z "$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

# Build with version tag (e.g., just release v1.2.3)
release version:
    go build -ldflags "-X main.Version={{version}}" -o muse .

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

# Remove build artifacts
clean:
    rm -f muse
