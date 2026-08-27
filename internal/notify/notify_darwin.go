//go:build darwin

package notify

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// alerterPaths are the absolute locations tried before falling back to a PATH
// lookup, covering both Apple Silicon (/opt/homebrew) and Intel (/usr/local)
// Homebrew prefixes. Declared as a var so tests can point it at a stub.
var alerterPaths = []string{
	"/opt/homebrew/bin/alerter",
	"/usr/local/bin/alerter",
}

const (
	alerterGroup   = "com.sglavoie.muse.now-playing"
	defaultSender  = "com.apple.Terminal"
	alerterTimeout = "5"
)

// terminalBundleIDs maps $TERM_PROGRAM values to the bundle identifier alerter
// should attribute the banner to. Clicking a banner activates the sender, so
// getting this right means the click returns to the terminal muse runs in.
var terminalBundleIDs = map[string]string{
	"Apple_Terminal": "com.apple.Terminal",
	"iTerm.app":      "com.googlecode.iterm2",
	"ghostty":        "com.mitchellh.ghostty",
	"Ghostty":        "com.mitchellh.ghostty",
	"alacritty":      "org.alacritty",
	"Alacritty":      "org.alacritty",
	"WezTerm":        "com.github.wez.wezterm",
	"kitty":          "net.kovidgoyal.kitty",
	"Hyper":          "co.zeit.hyper",
	"WarpTerminal":   "dev.warp.Warp-Stable",
	"vscode":         "com.microsoft.VSCode",
	"tmux":           defaultSender,
}

// alerterSender resolves the --sender bundle ID from $TERM_PROGRAM, falling
// back to Terminal.app for unknown or unset terminals. An unrecognised
// TERM_PROGRAM that already looks like a bundle ID (contains a dot) is used
// verbatim, so a terminal muse has never heard of can still be attributed.
func alerterSender() string {
	term := os.Getenv("TERM_PROGRAM")
	if id, ok := terminalBundleIDs[term]; ok {
		return id
	}
	if strings.Contains(term, ".") {
		return term
	}
	return defaultSender
}

// alerterArgs builds the argv passed to alerter. --group makes repeated
// notifications replace the existing banner instead of stacking, and --timeout
// auto-dismisses it (alerter otherwise blocks waiting for interaction).
func (t Track) alerterArgs() []string {
	args := []string{"--title", t.Title}
	// An empty --subtitle renders as a blank line, so omit the flag entirely.
	if t.Artist != "" {
		args = append(args, "--subtitle", t.Artist)
	}
	return append(args,
		"--message", t.Message(),
		"--group", alerterGroup,
		"--sender", alerterSender(),
		"--timeout", alerterTimeout,
	)
}

func backendCommand(t Track) (string, []string, error) {
	bin, err := resolveAlerter()
	if err != nil {
		return "", nil, err
	}
	return bin, t.alerterArgs(), nil
}

// resolveAlerter prefers the known absolute paths because Karabiner's
// shell_command runs with a minimal PATH, falling back to a PATH lookup for
// installs outside either Homebrew prefix.
func resolveAlerter() (string, error) {
	for _, p := range alerterPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	bin, err := exec.LookPath("alerter")
	if err != nil {
		return "", fmt.Errorf(
			"alerter not found at %s or on $PATH\n"+
				"Install alerter:\n"+
				"  macOS:  brew install alerter",
			strings.Join(alerterPaths, ", "),
		)
	}
	return bin, nil
}
