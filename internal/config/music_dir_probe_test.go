package config

import (
	"errors"
	"testing"
)

func fakeMusicEnv(goos string, env map[string]string, dirs ...string) MusicDirEnv {
	existing := map[string]bool{}
	for _, d := range dirs {
		existing[d] = true
	}
	return MusicDirEnv{
		GOOS:    goos,
		Getenv:  func(k string) string { return env[k] },
		HomeDir: func() (string, error) { return "/home/ada", nil },
		IsDir:   func(p string) bool { return existing[p] },
	}
}

func TestDetectMusicDirectory(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       MusicDirEnv
		wantPath  string
		wantLabel string
	}{
		{"macos home music", fakeMusicEnv("darwin", nil, "/home/ada/Music"), "/home/ada/Music", "~/Music"},
		{"macos ignores xdg", fakeMusicEnv("darwin", map[string]string{"XDG_MUSIC_DIR": "/srv/tunes"}, "/srv/tunes"), "", ""},
		{"linux xdg with $HOME", fakeMusicEnv("linux", map[string]string{"XDG_MUSIC_DIR": "$HOME/Audio"}, "/home/ada/Audio", "/home/ada/Music"), "/home/ada/Audio", "~/Audio"},
		{"linux xdg outside home", fakeMusicEnv("linux", map[string]string{"XDG_MUSIC_DIR": "/srv/tunes/"}, "/srv/tunes"), "/srv/tunes", "/srv/tunes"},
		{"linux missing xdg falls back", fakeMusicEnv("linux", map[string]string{"XDG_MUSIC_DIR": "/gone"}, "/home/ada/Music"), "/home/ada/Music", "~/Music"},
		{"linux relative xdg ignored", fakeMusicEnv("linux", map[string]string{"XDG_MUSIC_DIR": "Music"}, "Music"), "", ""},
		{"nothing exists", fakeMusicEnv("linux", nil), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DetectMusicDirectory(tc.env)
			if ok != (tc.wantPath != "") || got.Path != tc.wantPath || got.Label != tc.wantLabel {
				t.Fatalf("got %+v, %v; want %q %q", got, ok, tc.wantPath, tc.wantLabel)
			}
		})
	}
}

func TestDetectMusicDirectoryWithoutHome(t *testing.T) {
	env := fakeMusicEnv("darwin", nil, "/home/ada/Music")
	env.HomeDir = func() (string, error) { return "", errors.New("no home") }
	if _, ok := DetectMusicDirectory(env); ok {
		t.Fatal("a probe without a home directory must not suggest a folder")
	}
}
