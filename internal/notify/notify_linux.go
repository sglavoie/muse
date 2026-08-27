//go:build linux

package notify

import (
	"fmt"
	"os/exec"
)

const (
	// notifySendExpire matches the 5s alerter --timeout, in milliseconds.
	notifySendExpire = "5000"
	// notifySendSyncHint is the freedesktop equivalent of alerter's --group:
	// banners sharing the hint value replace each other instead of stacking.
	notifySendSyncHint = "--hint=string:x-canonical-private-synchronous:muse"
)

// notifySendArgs builds the argv passed to notify-send. notify-send has no
// subtitle concept, so the artist is folded into the body ahead of the message
// that alerter would render as --message.
func (t Track) notifySendArgs() []string {
	body := t.Message()
	if t.Artist != "" {
		body = t.Artist + " — " + body
	}
	return []string{
		"--app-name=muse",
		"--expire-time=" + notifySendExpire,
		notifySendSyncHint,
		t.Title,
		body,
	}
}

func backendCommand(t Track) (string, []string, error) {
	bin, err := exec.LookPath("notify-send")
	if err != nil {
		return "", nil, fmt.Errorf(
			"notify-send not found on $PATH\n" +
				"Install libnotify:\n" +
				"  Debian: apt install libnotify-bin\n" +
				"  Fedora: dnf install libnotify\n" +
				"  Arch:   pacman -S libnotify",
		)
	}
	return bin, t.notifySendArgs(), nil
}
