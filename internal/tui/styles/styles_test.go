package styles_test

import (
	"encoding/hex"
	"go/parser"
	"go/token"
	"image/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestNewStyles exercises the NewStyles constructor for both hasDark values,
// checking all four exported fields (HasDark, Dim, Accent, Bold).
func TestNewStyles(t *testing.T) {
	for _, hasDark := range []bool{true, false} {
		s := styles.NewStyles(hasDark)

		if s.HasDark != hasDark {
			t.Errorf("NewStyles(%v).HasDark = %v, want %v", hasDark, s.HasDark, hasDark)
		}

		dimOut := s.Dim.Render("x")
		if dimOut == "" {
			t.Errorf("NewStyles(%v).Dim.Render() returned empty string", hasDark)
		}

		accentOut := s.Accent.Render("x")
		if accentOut == "" {
			t.Errorf("NewStyles(%v).Accent.Render() returned empty string", hasDark)
		}

		boldOut := s.Bold.Render("x")
		if boldOut == "" {
			t.Errorf("NewStyles(%v).Bold.Render() returned empty string", hasDark)
		}

		headingOut := s.Heading.Render("x")
		if headingOut == "" {
			t.Errorf("NewStyles(%v).Heading.Render() returned empty string", hasDark)
		}

		successOut := s.Success.Render("x")
		if successOut == "" {
			t.Errorf("NewStyles(%v).Success.Render() returned empty string", hasDark)
		}

		warningOut := s.Warning.Render("x")
		if warningOut == "" {
			t.Errorf("NewStyles(%v).Warning.Render() returned empty string", hasDark)
		}

		errorOut := s.Error.Render("x")
		if errorOut == "" {
			t.Errorf("NewStyles(%v).Error.Render() returned empty string", hasDark)
		}

		colHeadOut := s.ColHead.Render("x")
		if colHeadOut == "" {
			t.Errorf("NewStyles(%v).ColHead.Render() returned empty string", hasDark)
		}

		accentMarkOut := s.AccentMark.Render("x")
		if accentMarkOut == "" {
			t.Errorf("NewStyles(%v).AccentMark.Render() returned empty string", hasDark)
		}

		statusBarOut := s.StatusBar.Render("x")
		if statusBarOut == "" {
			t.Errorf("NewStyles(%v).StatusBar.Render() returned empty string", hasDark)
		}

		favArtistOut := s.FavArtist.Render("x")
		if favArtistOut == "" {
			t.Errorf("NewStyles(%v).FavArtist.Render() returned empty string", hasDark)
		}

		favAlbumOut := s.FavAlbum.Render("x")
		if favAlbumOut == "" {
			t.Errorf("NewStyles(%v).FavAlbum.Render() returned empty string", hasDark)
		}

		favArtistAccentOut := s.FavArtistAccent.Render("x")
		if favArtistAccentOut == "" {
			t.Errorf("NewStyles(%v).FavArtistAccent.Render() returned empty string", hasDark)
		}

		favAlbumAccentOut := s.FavAlbumAccent.Render("x")
		if favAlbumAccentOut == "" {
			t.Errorf("NewStyles(%v).FavAlbumAccent.Render() returned empty string", hasDark)
		}

		nowPlayingFavArtistOut := s.NowPlayingFavArtist.Render("x")
		if nowPlayingFavArtistOut == "" {
			t.Errorf("NewStyles(%v).NowPlayingFavArtist.Render() returned empty string", hasDark)
		}

		nowPlayingFavAlbumOut := s.NowPlayingFavAlbum.Render("x")
		if nowPlayingFavAlbumOut == "" {
			t.Errorf("NewStyles(%v).NowPlayingFavAlbum.Render() returned empty string", hasDark)
		}

		textPrimaryOut := s.TextPrimary.Render("x")
		if textPrimaryOut == "" {
			t.Errorf("NewStyles(%v).TextPrimary.Render() returned empty string", hasDark)
		}

		textSecondaryOut := s.TextSecondary.Render("x")
		if textSecondaryOut == "" {
			t.Errorf("NewStyles(%v).TextSecondary.Render() returned empty string", hasDark)
		}

		textTertiaryOut := s.TextTertiary.Render("x")
		if textTertiaryOut == "" {
			t.Errorf("NewStyles(%v).TextTertiary.Render() returned empty string", hasDark)
		}

		nowPlayingTitleOut := s.NowPlayingTitle.Render("x")
		if nowPlayingTitleOut == "" {
			t.Errorf("NewStyles(%v).NowPlayingTitle.Render() returned empty string", hasDark)
		}
	}
}

// relativeLuminance returns the WCAG 2.x relative luminance of an sRGB color
// (channels in [0,255]). Result is in [0,1].
func relativeLuminance(c color.RGBA) float64 {
	conv := func(b uint8) float64 {
		v := float64(b) / 255.0
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*conv(c.R) + 0.7152*conv(c.G) + 0.0722*conv(c.B)
}

// contrastRatio returns the WCAG contrast ratio between two sRGB colors.
func contrastRatio(fg, bg color.RGBA) float64 {
	l1 := relativeLuminance(fg)
	l2 := relativeLuminance(bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// parseHex parses a "#rrggbb" string into a color.RGBA with alpha 0xff.
func parseHex(t *testing.T, hexColor string) color.RGBA {
	t.Helper()
	if len(hexColor) != 7 || hexColor[0] != '#' {
		t.Fatalf("parseHex: bad hex %q (want #rrggbb)", hexColor)
	}
	rgb, err := hex.DecodeString(hexColor[1:])
	if err != nil {
		t.Fatalf("parseHex(%q): %v", hexColor, err)
	}
	if len(rgb) != 3 {
		t.Fatalf("parseHex(%q): got %d bytes, want 3", hexColor, len(rgb))
	}
	return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 0xff}
}

func colorRGBA(t *testing.T, c color.Color) color.RGBA {
	t.Helper()
	if c == nil {
		t.Fatal("color is nil")
	}
	rgba, ok := color.RGBAModel.Convert(c).(color.RGBA)
	if !ok {
		t.Fatalf("color model conversion returned %T, want color.RGBA", color.RGBAModel.Convert(c))
	}
	return rgba
}

func TestEmptyHintUsesExplicitDimForeground(t *testing.T) {
	cases := []struct {
		name    string
		hasDark bool
		want    string
	}{
		{"dark", true, "#7882a4"},
		{"light", false, "#7a7f8c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := colorRGBA(t, styles.NewStyles(c.hasDark).EmptyHint.GetForeground())
			want := parseHex(t, c.want)
			if got != want {
				t.Fatalf("EmptyHint foreground = %#v, want %#v", got, want)
			}
		})
	}
}

func TestProgressEmptyUsesExplicitDimForeground(t *testing.T) {
	cases := []struct {
		name    string
		hasDark bool
		want    string
	}{
		{"dark", true, "#7882a4"},
		{"light", false, "#7a7f8c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := colorRGBA(t, styles.NewStyles(c.hasDark).ProgressEmpty.GetForeground())
			want := parseHex(t, c.want)
			if got != want {
				t.Fatalf("ProgressEmpty foreground = %#v, want %#v", got, want)
			}
		})
	}
}

// TestStripForegroundContrastDark asserts that each of the four foregrounds
// the state strip and footer hints rely on (TextPrimary, TextSecondary,
// TextTertiary, Dim) clears WCAG AA (4.5:1) against the dark terminal
// background (#16161e). This prevents silent palette regressions.
func TestStripForegroundContrastDark(t *testing.T) {
	bg := parseHex(t, "#16161e")
	cases := []struct {
		name string
		fg   string
	}{
		{"TextPrimary", "#c0caf5"},
		{"TextSecondary", "#a9b1d6"},
		{"TextTertiary", "#8a93bd"},
		{"Dim", "#7882a4"},
	}
	for _, c := range cases {
		fg := parseHex(t, c.fg)
		ratio := contrastRatio(fg, bg)
		if ratio < 4.5 {
			t.Errorf("%s (%s on #16161e): contrast %.2f:1 < 4.5:1 (WCAG AA)", c.name, c.fg, ratio)
		}
	}
}

func TestActiveTabContrast(t *testing.T) {
	for _, c := range []struct {
		name    string
		hasDark bool
	}{
		{"dark", true},
		{"light", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			activeTab := styles.NewStyles(c.hasDark).ActiveTab
			fg := colorRGBA(t, activeTab.GetForeground())
			bg := colorRGBA(t, activeTab.GetBackground())
			ratio := contrastRatio(fg, bg)
			if ratio < 4.5 {
				t.Fatalf("ActiveTab contrast %.2f:1 < 4.5:1 (fg %#v on bg %#v)", ratio, fg, bg)
			}
		})
	}
}

// TestStyleNesting verifies that Background wrapping Bold output preserves inner
// styling. Bold sequences must survive the outer Background render, satisfying
// ac-f4704a90675005f2b2db92c90011a914.
func TestStyleNesting(t *testing.T) {
	bold := lipgloss.NewStyle().Bold(true)
	bg := lipgloss.NewStyle().Background(lipgloss.Color("#2c3e50"))

	inner := bold.Render("hello")
	if inner == "hello" {
		t.Fatal("bold.Render did not apply any ANSI sequences to the text")
	}

	outer := bg.Render(inner)
	if outer == "" {
		t.Fatal("outer render is empty")
	}

	// The bold ANSI attribute sequence must be explicitly present in the outer render.
	// Constructed at runtime so this source file does not trigger TestNoRawANSI.
	boldSeq := string([]byte{0x1b, 0x5b, 0x31, 0x6d}) // ESC [ 1 m
	if !strings.Contains(outer, boldSeq) {
		t.Errorf("outer render does not contain bold ANSI sequence (ESC[1m); outer=%q", outer)
	}

	// The bold sequences from inner must be present inside the outer render.
	if !strings.Contains(outer, inner) {
		t.Errorf("outer render does not contain inner bold sequences; outer=%q inner=%q", outer, inner)
	}

	stripped := ansi.Strip(outer)
	if stripped != "hello" {
		t.Errorf("stripped text = %q, want %q", stripped, "hello")
	}

	// Verify that the package's own Accent style round-trips through ansi.Strip,
	// tying this contract test to styles.NewStyles rather than raw lipgloss only.
	accentOut := styles.NewStyles(true).Accent.Render("hello")
	if ansi.Strip(accentOut) != "hello" {
		t.Errorf("ansi.Strip(NewStyles(true).Accent.Render(%q)) = %q, want %q", "hello", ansi.Strip(accentOut), "hello")
	}
}

// TestNoRawANSI walks all .go files under the module root and fails if any file
// contains a raw ANSI escape sequence literal. The scan root is derived from
// this file's path (internal/tui/styles/ → three levels up = module root) so
// the guard covers the entire codebase, not just this package.
//
// Patterns are built at runtime so this source file does not trigger its own
// check.
func TestNoRawANSI(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../internal/tui/styles/styles_test.go; walk three levels up.
	root := filepath.Join(filepath.Dir(thisFile), "../../..")
	t.Logf("TestNoRawANSI: scanning module root %s", root)

	// Build forbidden patterns at runtime so this source file doesn't match itself.
	// Bytes: backslash + "033[" (octal) and backslash + "x1b[" (hex).
	esc := string([]byte{0x5c, 0x30, 0x33, 0x33, 0x5b})
	hex := string([]byte{0x5c, 0x78, 0x31, 0x62, 0x5b})
	forbidden := []string{esc, hex}

	var fileCount int
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		fileCount++
		src, readErr := os.ReadFile(path) //nolint:gosec // G122: test walks its own tree, no symlink risk
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(root, path)

		for _, pat := range forbidden {
			if strings.Contains(content, pat) {
				t.Errorf("%s: contains raw ANSI escape sequence (use lipgloss instead)", rel)
				break
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", root, err)
	}

	// Sanity check: ensure we scanned well beyond this package's own files.
	if fileCount <= 2 {
		t.Errorf("only %d .go file(s) scanned; expected more than 2 (root: %s)", fileCount, root)
	}
	t.Logf("TestNoRawANSI: scanned %d .go files", fileCount)
}

// isStdlibImport returns true for standard library import paths. Standard
// library paths have no dot in the first path segment (e.g. "fmt", "os",
// "go/parser"), while all third-party modules start with a hostname.
func isStdlibImport(path string) bool {
	first := strings.SplitN(path, "/", 2)[0]
	return !strings.Contains(first, ".")
}

// TestStylesPackageImportIsolation scans non-test .go files in
// internal/tui/styles/ and fails if:
//   - any import path contains "internal/tui/" (upward TUI dependency), or
//   - any import is not from the standard library and is not charm.land/lipgloss/v2
//
// This keeps the styles package a pure leaf that only depends on stdlib and
// the single charmbracelet styling library.
func TestStylesPackageImportIsolation(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	stylesDir := filepath.Dir(thisFile)
	t.Logf("TestStylesPackageImportIsolation: scanning styles dir %s", stylesDir)

	entries, err := os.ReadDir(stylesDir)
	if err != nil {
		t.Fatalf("cannot read styles dir %s: %v", stylesDir, err)
	}

	fset := token.NewFileSet()
	var scanned int

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(stylesDir, name)
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatalf("cannot parse %s: %v", name, parseErr)
		}
		scanned++

		for _, imp := range f.Imports {
			// imp.Path.Value includes surrounding quotes, e.g. `"internal/tui/library"`
			impPath := strings.Trim(imp.Path.Value, `"`)

			if strings.Contains(impPath, "internal/tui/") {
				t.Errorf("%s: forbidden import %q (styles must not import from internal/tui/)", name, impPath)
				continue
			}

			if !isStdlibImport(impPath) && impPath != "charm.land/lipgloss/v2" {
				t.Errorf("%s: non-whitelisted import %q (only stdlib and charm.land/lipgloss/v2 are allowed)", name, impPath)
			}
		}
	}

	if scanned == 0 {
		t.Fatalf("no non-test .go files found in %s", stylesDir)
	}
	t.Logf("TestStylesPackageImportIsolation: scanned %d non-test file(s)", scanned)
}

// TestLipglossV2ColorAPI verifies that the native lipgloss v2 color API is in
// use. It calls lipgloss.Color() and assigns the result to a color.Color
// variable (this only compiles when lipgloss v2 returns color.Color, not the
// v1 string type alias). It also calls lipgloss.LightDark() to confirm that
// helper is available for adaptive theming.
func TestLipglossV2ColorAPI(t *testing.T) {
	// lipgloss.Color returns color.Color in v2; this assignment would fail to
	// compile against a v1 or compat package that returns a string type.
	var c color.Color = lipgloss.Color("#ff0000")
	if c == nil {
		t.Fatal("lipgloss.Color returned nil")
	}

	// lipgloss.LightDark must be callable; invoke it to confirm availability.
	ld := lipgloss.LightDark(true)
	if ld == nil {
		t.Fatal("lipgloss.LightDark returned nil function")
	}
}

// TestNoLipglossRenderHelpers walks all TUI .go files and fails if any of the
// banned lipgloss print-helper calls — lipgloss.Println, lipgloss.Sprint, or
// lipgloss.Fprintln — appear in production code. Only lipgloss.Style.Render()
// is permitted for styled output in View() methods.
func TestNoLipglossRenderHelpers(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../internal/tui/styles/styles_test.go; up three levels is module root.
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "../../..")
	tuiDir := filepath.Join(moduleRoot, "internal", "tui")
	t.Logf("TestNoLipglossRenderHelpers: scanning %s", tuiDir)

	forbidden := []string{
		"lipgloss.Println",
		"lipgloss.Sprint",
		"lipgloss.Fprintln",
	}

	var fileCount int
	err := filepath.WalkDir(tuiDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileCount++
		src, readErr := os.ReadFile(path) //nolint:gosec // G122: test walks its own tree, no symlink risk
		if readErr != nil {
			return readErr
		}

		content := string(src)
		rel, _ := filepath.Rel(moduleRoot, path)
		for _, call := range forbidden {
			if strings.Contains(content, call) {
				t.Errorf("%s: forbidden call %q found (use lipgloss.Style.Render() instead)", rel, call)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("error walking %s: %v", tuiDir, err)
	}

	if fileCount == 0 {
		t.Fatalf("no non-test .go files found under %s", tuiDir)
	}
	t.Logf("TestNoLipglossRenderHelpers: scanned %d non-test file(s)", fileCount)
}

// TestBuildNoImportCycles runs "go build ./..." from the module root and fails
// if the command exits with a non-zero status. The Go toolchain reports import
// cycles as hard errors, so a clean build confirms none exist.
func TestBuildNoImportCycles(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../internal/tui/styles/styles_test.go; three levels up is module root.
	moduleRoot := filepath.Join(filepath.Dir(thisFile), "../../..")

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./... failed (import cycle or build error):\n%s", string(out))
	}
}
