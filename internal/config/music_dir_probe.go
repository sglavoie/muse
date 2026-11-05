package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// MusicDirSuggestion is the platform's conventional music folder, offered on
// the first-run screen as a one-key starting point. Path is absolute; Label is
// the same folder written for display, with the home directory shortened to ~.
type MusicDirSuggestion struct {
	Path  string
	Label string
}

// MusicDirEnv is everything DetectMusicDirectory reads from the outside world.
// Tests and the TUI fixtures pass fakes so no probe touches the real disk.
type MusicDirEnv struct {
	GOOS    string
	Getenv  func(string) string
	HomeDir func() (string, error)
	IsDir   func(string) bool
}

// OSMusicDirEnv returns the environment of the running process.
func OSMusicDirEnv() MusicDirEnv {
	return MusicDirEnv{
		GOOS:    runtime.GOOS,
		Getenv:  os.Getenv,
		HomeDir: os.UserHomeDir,
		IsDir: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
	}
}

// DetectMusicDirectory finds the folder where this platform keeps music:
// ~/Music on macOS, and on other systems $XDG_MUSIC_DIR when it is set,
// falling back to ~/Music. It reports false when that folder does not exist.
//
// XDG_MUSIC_DIR is written in user-dirs.dirs style, so a leading $HOME is
// expanded like the desktop tools do.
func DetectMusicDirectory(env MusicDirEnv) (MusicDirSuggestion, bool) {
	home, err := env.HomeDir()
	if err != nil || home == "" {
		return MusicDirSuggestion{}, false
	}
	candidates := []string{}
	if env.GOOS != "darwin" {
		if xdg := strings.TrimSpace(env.Getenv("XDG_MUSIC_DIR")); xdg != "" {
			candidates = append(candidates, expandHomePrefix(xdg, home))
		}
	}
	candidates = append(candidates, filepath.Join(home, "Music"))
	for _, dir := range candidates {
		dir = filepath.Clean(dir)
		if !filepath.IsAbs(dir) || !env.IsDir(dir) {
			continue
		}
		return MusicDirSuggestion{Path: dir, Label: shortenHome(dir, home)}, true
	}
	return MusicDirSuggestion{}, false
}

func expandHomePrefix(path, home string) string {
	for _, prefix := range []string{"$HOME", "${HOME}", "~"} {
		if path == prefix {
			return home
		}
		if rest, ok := strings.CutPrefix(path, prefix+"/"); ok {
			return filepath.Join(home, rest)
		}
	}
	return path
}

func shortenHome(path, home string) string {
	home = filepath.Clean(home)
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}
