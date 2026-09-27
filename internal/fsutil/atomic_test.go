package fsutil

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func writeString(s string) func(*os.File) error {
	return func(f *os.File) error {
		_, err := io.WriteString(f, s)
		return err
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// assertNoTempFiles fails if a ".tmp-*" leftover remains in dir.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(leftovers) > 0 {
		t.Errorf("temporary files left behind in %s: %v", dir, leftovers)
	}
}

func TestWriteAtomicallyCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml")

	if err := WriteAtomically(path, 0o600, writeString("first")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if got := readFile(t, path); got != "first" {
		t.Errorf("content = %q, want %q", got, "first")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600", perm)
	}

	if err := WriteAtomically(path, 0o600, writeString("second")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if got := readFile(t, path); got != "second" {
		t.Errorf("content = %q, want %q", got, "second")
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

func TestWriteAtomicallyFailedWriteKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	boom := errors.New("boom")
	err := WriteAtomically(path, 0o600, func(f *os.File) error {
		_, _ = io.WriteString(f, "partial")
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if got := readFile(t, path); got != "original" {
		t.Errorf("content = %q, want the original to survive a failed write", got)
	}
	assertNoTempFiles(t, dir)
}

// TestWriteAtomicallyPreservesSymlink covers a dotfile-managed config: the
// path is a symlink into another directory, and a save must update the target
// rather than replace the link with a regular file.
func TestWriteAtomicallyPreservesSymlink(t *testing.T) {
	linkDir := t.TempDir()
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "config.toml")
	link := filepath.Join(linkDir, "config.toml")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if err := WriteAtomically(link, 0o600, writeString("new")); err != nil {
		t.Fatalf("write through symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s was replaced by a regular file (mode %v), want it to stay a symlink", link, info.Mode())
	}
	if dest, err := os.Readlink(link); err != nil || dest != target {
		t.Errorf("readlink = %q, %v; want %q", dest, err, target)
	}
	if got := readFile(t, target); got != "new" {
		t.Errorf("target content = %q, want %q", got, "new")
	}
	assertNoTempFiles(t, linkDir)
	assertNoTempFiles(t, targetDir)
}
