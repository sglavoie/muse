package artwork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif" // embedded covers are occasionally GIFs
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrNoArt reports that a track's album has no usable cover art.
var ErrNoArt = errors.New("artwork: no cover art")

// Request identifies the track whose album art is wanted.
type Request struct {
	Path        string
	Album       string
	AlbumArtist string
	Artist      string
}

// Key returns the cache key for the request's album. Tracks of one album in
// one folder share a cover, so the folder and album name identify it; a track
// without an album is its own key.
func (r Request) Key() string {
	h := sha256.New()
	if strings.TrimSpace(r.Album) == "" {
		_, _ = io.WriteString(h, "track\x00"+r.Path)
	} else {
		artist := r.AlbumArtist
		if artist == "" {
			artist = r.Artist
		}
		_, _ = io.WriteString(h, "album\x00"+filepath.Dir(r.Path)+"\x00"+strings.ToLower(r.Album))
		// Compilations in one folder still share a cover; the artist only
		// separates genuinely different albums that happen to share a name.
		if r.AlbumArtist != "" {
			_, _ = io.WriteString(h, "\x00"+strings.ToLower(artist))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Loader resolves a track's cover art.
type Loader interface {
	Load(ctx context.Context, req Request) (*Art, error)
}

// sidecarNames are cover image basenames checked next to the track, in order.
var sidecarNames = []string{"cover", "folder", "front", "album", "albumart", "artwork", "albumartsmall"}

var sidecarExts = []string{".jpg", ".jpeg", ".png", ".gif"}

// negativeTTL is how long a "no art" result is trusted before the album is
// looked at again, so art added later is picked up without manual action.
const negativeTTL = 24 * time.Hour

// maxImageBytes bounds what is read from a sidecar or an extractor.
const maxImageBytes = 24 << 20

// FileLoader reads art from disk and caches normalized PNGs under CacheDir.
type FileLoader struct {
	CacheDir string
	// ExifTool and FFmpeg are executable paths; empty disables that source.
	ExifTool string
	FFmpeg   string
}

// NewFileLoader returns a loader caching in the platform cache directory,
// using whichever extractors are installed.
func NewFileLoader() *FileLoader {
	l := &FileLoader{CacheDir: DefaultCacheDir()}
	if p, err := exec.LookPath("exiftool"); err == nil {
		l.ExifTool = p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		l.FFmpeg = p
	}
	return l
}

// DefaultCacheDir returns $XDG_CACHE_HOME/muse/artwork, falling back to
// ~/Library/Caches on macOS and ~/.cache elsewhere.
func DefaultCacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" || !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), "muse-artwork")
		}
		base = filepath.Join(home, ".cache")
		if runtime.GOOS == "darwin" {
			base = filepath.Join(home, "Library", "Caches")
		}
	}
	return filepath.Join(base, "muse", "artwork")
}

// Load returns the album's art from the cache, or finds, normalizes and
// caches it. It returns ErrNoArt when the album has none.
func (l *FileLoader) Load(ctx context.Context, req Request) (*Art, error) {
	key := req.Key()
	if art, err := l.cached(key); err == nil || errors.Is(err, ErrNoArt) {
		return art, err
	}
	img, err := l.find(ctx, req)
	if err != nil {
		if errors.Is(err, ErrNoArt) {
			l.remember(key, nil)
		}
		return nil, err
	}
	art := New(img)
	l.remember(key, art)
	return art, nil
}

func (l *FileLoader) paths(key string) (pngPath, nonePath string) {
	dir := filepath.Join(l.CacheDir, key[:2])
	return filepath.Join(dir, key+".png"), filepath.Join(dir, key+".none")
}

// cached returns the cached art, ErrNoArt for a fresh negative entry, or
// os.ErrNotExist when the album has not been looked at yet.
func (l *FileLoader) cached(key string) (*Art, error) {
	if l.CacheDir == "" {
		return nil, os.ErrNotExist
	}
	pngPath, nonePath := l.paths(key)
	if f, err := os.Open(pngPath); err == nil {
		defer f.Close()
		img, err := png.Decode(io.LimitReader(f, maxImageBytes))
		if err == nil {
			return New(img), nil
		}
	}
	if info, err := os.Stat(nonePath); err == nil && time.Since(info.ModTime()) < negativeTTL {
		return nil, ErrNoArt
	}
	return nil, os.ErrNotExist
}

// remember writes art (or a negative marker when art is nil) to the cache.
// Cache failures are ignored: the art is simply looked up again next time.
func (l *FileLoader) remember(key string, art *Art) {
	if l.CacheDir == "" {
		return
	}
	pngPath, nonePath := l.paths(key)
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		return
	}
	if art == nil {
		_ = os.WriteFile(nonePath, nil, 0o644)
		return
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, art.Image); err != nil {
		return
	}
	tmp := pngPath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, pngPath); err != nil {
		_ = os.Remove(tmp)
		return
	}
	_ = os.Remove(nonePath)
}

// find tries a sidecar image, then the picture embedded in the track.
func (l *FileLoader) find(ctx context.Context, req Request) (image.Image, error) {
	if img, ok := findSidecar(filepath.Dir(req.Path)); ok {
		return img, nil
	}
	if req.Path == "" {
		return nil, ErrNoArt
	}
	if l.ExifTool != "" {
		for _, tag := range []string{"-Picture", "-CoverArt"} {
			if img, ok := decodeOutput(ctx, l.ExifTool, "-b", tag, "--", req.Path); ok {
				return img, nil
			}
		}
	}
	if l.FFmpeg != "" {
		if img, ok := decodeOutput(ctx, l.FFmpeg, "-v", "error", "-nostdin", "-i", req.Path,
			"-an", "-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "-"); ok {
			return img, nil
		}
	}
	return nil, ErrNoArt
}

// findSidecar looks for a conventionally named cover image in dir, matching
// names case-insensitively.
func findSidecar(dir string) (image.Image, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	byName := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Type().IsRegular() {
			byName[strings.ToLower(e.Name())] = e.Name()
		}
	}
	for _, name := range sidecarNames {
		for _, ext := range sidecarExts {
			real, ok := byName[name+ext]
			if !ok {
				continue
			}
			if img, ok := decodeFile(filepath.Join(dir, real)); ok {
				return img, true
			}
		}
	}
	return nil, false
}

func decodeFile(path string) (image.Image, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	img, _, err := image.Decode(io.LimitReader(f, maxImageBytes))
	return img, err == nil && validSize(img)
}

// decodeOutput runs an extractor and decodes the image it writes to stdout.
func decodeOutput(ctx context.Context, name string, args ...string) (image.Image, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &out, n: maxImageBytes}
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		return nil, false
	}
	img, _, err := image.Decode(&out)
	return img, err == nil && validSize(img)
}

func validSize(img image.Image) bool {
	b := img.Bounds()
	return b.Dx() >= 16 && b.Dy() >= 16
}

// limitedWriter discards everything past n bytes so a runaway extractor
// cannot exhaust memory; the truncated image then fails to decode.
type limitedWriter struct {
	w io.Writer
	n int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.n <= 0 {
		return len(p), nil
	}
	keep := p
	if len(keep) > lw.n {
		keep = keep[:lw.n]
	}
	lw.n -= len(keep)
	if _, err := lw.w.Write(keep); err != nil {
		return 0, err
	}
	return len(p), nil
}
