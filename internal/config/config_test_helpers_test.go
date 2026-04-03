package config

import (
	"bytes"
	"log/slog"
)

// captureLog returns a logger that writes to a buffer and the buffer itself.
func captureLog() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(handler), buf
}
