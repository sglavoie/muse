package library

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates a file at path (creating parent dirs) with the given content.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeFile MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("writeFile WriteFile: %v", err)
	}
}

// makeBytes returns a byte slice of length n filled with the given byte value.
func makeBytes(n int, b byte) []byte {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = b
	}
	return buf
}