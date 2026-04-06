package tui

import (
	"context"
	"testing"
)

func TestOpenDirReturnsNonNilCmd(t *testing.T) {
	cmd := openDir(context.Background(), "/tmp")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}
