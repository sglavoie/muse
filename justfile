# Build the muse binary
build:
    go build -o muse .

test:
    go test ./...
