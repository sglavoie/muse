package main

import (
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

// TestJustFmtCheck_SkipsGitignoredGoFiles runs the real `just fmt-check`
// recipe in a scratch repo with the real .gitignore: Go files Zig fetches into
// macos/.cache are ignored, so they must never fail the check, while a new
// file that is not yet tracked still does.
func TestJustFmtCheck_SkipsGitignoredGoFiles(t *testing.T) {
	for _, tool := range []string{"just", "git", "gofmt"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}
	dir := t.TempDir()
	for _, name := range []string{"justfile", ".gitignore"} {
		content, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		writeFile(t, dir, name, string(content))
	}
	writeFile(t, dir, "ok.go", "package x\n")
	writeFile(t, dir, "macos/.cache/p/vendored.go", "package   p\n")
	runIn(t, dir, "git", "init", "-q")
	runIn(t, dir, "git", "add", "justfile", ".gitignore", "ok.go")

	if out, err := justFmtCheck(dir); err != nil {
		t.Fatalf("fmt-check failed on a gitignored file: %v\n%s", err, out)
	}

	writeFile(t, dir, "bad.go", "package   x\n")
	out, err := justFmtCheck(dir)
	if err == nil {
		t.Fatalf("fmt-check passed with an untracked unformatted file:\n%s", out)
	}
	if !strings.Contains(out, "bad.go") || strings.Contains(out, "vendored.go") {
		t.Fatalf("fmt-check output should name bad.go only:\n%s", out)
	}
}

func justFmtCheck(dir string) (string, error) {
	cmd := exec.Command("just", "fmt-check")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runIn(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// writeFile writes rel, a slash-separated path, inside dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer root.Close()
	if err := root.MkdirAll(path.Dir(rel), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := root.WriteFile(rel, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}
