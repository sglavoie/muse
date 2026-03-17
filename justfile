# Build the muse binary
build:
    go build -o muse .

install:
    go install .

test:
    go test ./...
