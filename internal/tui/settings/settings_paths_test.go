package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestLibraryDirectoryErrorsAllowCorrectionAndCancel(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "song.flac")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, path, want string }{
		{"empty", "", "enter a directory path"},
		{"missing", filepath.Join(root, "missing"), "directory does not exist"},
		{"file", file, "not a directory"},
		{"duplicate", root, "already in the library"},
		{"trailing slash", root + "/", "already in the library"},
		{"symlink", alias, "already in the library"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewSettingsOverlay(config.Defaults(), []string{root})
			o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
			o.LibDirInput.SetValue(tc.path)
			o, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			if action != actionNone || o.LibDirInput == nil || o.LibDirInput.Value() != tc.path || len(o.LibraryDirs) != 1 || o.dirty {
				t.Fatal("invalid input must remain editable without changing directories")
			}
			out := ansi.Strip(o.Render(60, 15, styles.NewStyles(true)))
			if !strings.Contains(out, tc.want) {
				t.Fatalf("missing error %q in:\n%s", tc.want, out)
			}
			for _, line := range strings.Split(out, "\n") {
				if ansi.StringWidth(line) > 60 {
					t.Fatalf("error overflows minimum width: %q", line)
				}
			}
			cancelled, action, _ := o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
			if action != actionNone || cancelled.LibDirInput != nil || cancelled.libDirError != "" {
				t.Fatal("escape must dismiss the input and its error, keeping Settings open")
			}
			corrected := t.TempDir()
			o.LibDirInput.SetValue(corrected)
			o, _, _ = o.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
			if o.LibDirInput != nil || o.libDirError != "" || len(o.LibraryDirs) != 2 || o.LibraryDirs[1] != corrected || !o.dirty {
				t.Fatal("corrected directory was not accepted")
			}
		})
	}
}

func TestLibraryDirectoryExpandsAndNormalizesPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(home)
	for _, input := range []string{"~", "~/", "."} {
		got, err := validateLibraryDir(input, nil)
		if err != nil || got != home {
			t.Fatalf("%q: got %q, %v; want %q", input, got, err, home)
		}
	}
}
