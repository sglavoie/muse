package statusbar

import (
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/text"
)

// TransientNotifyMsg carries a short message to display in the status bar.
type TransientNotifyMsg string

// TransientErrorMsg carries a failed action back to the update loop.
type TransientErrorMsg string

// Severity describes presentation independently of notification priority.
type Severity uint8

const (
	severityNormal Severity = iota
	SeverityError
)

// NotifPriority controls which notification wins when multiple are queued.
// Higher values win; the queue evicts the lowest-priority entry when full.
type NotifPriority uint8

const (
	NotifInfo   NotifPriority = iota // scan progress, sort, volume, mode toggles
	NotifNormal                      // action results, errors, undo prompts
)

// notificationFadeWindow is the trailing slice of a notification's lifetime
// during which it renders dim (the "fade" frame) before vanishing.
const notificationFadeWindow = 250 * time.Millisecond

// transientDuration is how long a transient notification stays visible before
// the 1-second tick clears it.
const transientDuration = 5 * time.Second

// TransientNotification holds a single pending notification for the status bar
// right zone. expiresAt is the time after which the notification should be
// cleared by the tick handler. A zero expiresAt means no notification is set.
type TransientNotification struct {
	Msg           string
	ExpiresAt     time.Time
	priority      NotifPriority
	Severity      Severity
	undoExpiresAt time.Time
}

// PushMsg enqueues a notification with an explicit duration and priority.
// The queue is capped at 5 entries; when full the lowest-priority oldest entry
// is evicted. If all existing entries outrank the incoming one it is dropped.
func (s *StatusBar) PushMsg(msg string, dur time.Duration, priority NotifPriority) {
	s.pushNotification(TransientNotification{Msg: msg, ExpiresAt: time.Now().Add(dur), priority: priority})
}

func (s *StatusBar) pushNotification(n TransientNotification) {
	if n.priority == NotifNormal {
		s.rememberMessage(n.Msg, n.Severity)
	}
	const maxQueue = 5
	s.PruneNotifications()
	if len(s.Notifications) < maxQueue {
		s.Notifications = append(s.Notifications, n)
		return
	}

	evictIdx := -1
	var evictPrio NotifPriority
	for i, existing := range s.Notifications {
		if evictIdx < 0 || existing.priority < evictPrio {
			evictIdx = i
			evictPrio = existing.priority
		}
	}
	if evictIdx >= 0 && evictPrio <= n.priority {
		s.Notifications = append(s.Notifications[:evictIdx], s.Notifications[evictIdx+1:]...)
		s.Notifications = append(s.Notifications, n)
	}
}

// isExpired reports whether n has expired. A zero expiresAt is treated as
// "no expiry" and is never expired (used for direct-assignment test helpers).
func (n TransientNotification) isExpired() bool {
	return !n.ExpiresAt.IsZero() && time.Now().After(n.ExpiresAt)
}

// isFading reports whether n is within notificationFadeWindow of expiry.
// Zero expiresAt notifications never fade (treated as persistent).
func (n TransientNotification) isFading() bool {
	if n.ExpiresAt.IsZero() {
		return false
	}
	return time.Until(n.ExpiresAt) <= notificationFadeWindow
}

// NextFadeAt returns the earliest moment (across all queued, non-expired
// notifications) at which a notification will enter its fade window. Returns a
// zero time when no future fade is pending. Notifications already inside the
// fade window are skipped — their fade frame is rendered by the current cycle.
func (s StatusBar) NextFadeAt() time.Time {
	var earliest time.Time
	for _, n := range s.Notifications {
		if n.ExpiresAt.IsZero() || n.isExpired() {
			continue
		}
		fadeAt := n.ExpiresAt.Add(-notificationFadeWindow)
		if !time.Now().Before(fadeAt) {
			continue
		}
		if earliest.IsZero() || fadeAt.Before(earliest) {
			earliest = fadeAt
		}
	}
	return earliest
}

// PruneNotifications removes expired entries from the notifications queue.
func (s *StatusBar) PruneNotifications() {
	i := 0
	for _, notif := range s.Notifications {
		if !notif.isExpired() {
			s.Notifications[i] = notif
			i++
		}
	}
	s.Notifications = s.Notifications[:i]
}

// TopNotification returns the highest-priority non-expired notification, or
// a zero value when the queue is empty or all entries have expired.
// Among entries with equal priority, the last-pushed one wins.
func (s StatusBar) TopNotification() TransientNotification {
	var top TransientNotification
	for _, notif := range s.Notifications {
		if notif.isExpired() || (!notif.undoExpiresAt.IsZero() && !notif.undoExpiresAt.Equal(s.UndoExpiresAt)) {
			continue
		}
		if top.Msg == "" || notif.priority >= top.priority {
			top = notif
		}
	}
	return top
}

// NotifyDuration is how long notify and notifyInfo keep a message on screen.
// Falls back to the default 5s constant when transientDur is zero (e.g. in
// tests that do not set config). Callers that arm a key for exactly as long as
// their notification is visible read the window from here so the two cannot
// drift apart.
func (s StatusBar) NotifyDuration() time.Duration {
	if s.TransientDur == 0 {
		return transientDuration
	}
	return s.TransientDur
}

// Notify enqueues a notifNormal notification using the configured transient
// duration.
func (s *StatusBar) Notify(msg string) {
	s.PushMsg(msg, s.NotifyDuration(), NotifNormal)
}

// NotifyError records an error with the same lifetime and priority as action results.
func (s *StatusBar) NotifyError(msg string) {
	s.pushNotification(TransientNotification{
		Msg: msg, ExpiresAt: time.Now().Add(s.NotifyDuration()),
		priority: NotifNormal, Severity: SeverityError,
	})
}

// NotifyUndo links a prompt to the exact deadline of the current undo action.
func (s *StatusBar) NotifyUndo(msg string, expiresAt time.Time) {
	s.UndoExpiresAt = expiresAt
	s.pushNotification(TransientNotification{
		Msg: msg, ExpiresAt: expiresAt, priority: NotifNormal, undoExpiresAt: expiresAt,
	})
}

// NotifyInfo enqueues a notifInfo notification (scan, sort, volume, mode
// toggles). These yield to notifNormal when both are queued.
func (s *StatusBar) NotifyInfo(msg string) {
	s.PushMsg(msg, s.NotifyDuration(), NotifInfo)
}

// HandleScanProgress sets the transient notification to a formatted progress
// string with a 5-second expiry. New calls overwrite the previous notification.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) HandleScanProgress(msg library.ScanProgressMsg) {
	var notice string
	if msg.Total > 0 {
		notice = fmt.Sprintf("Scanning… %s / %s tracks", text.FormatCount(msg.Scanned), text.FormatCount(msg.Total))
	} else {
		notice = fmt.Sprintf("Scanning… %s tracks", text.FormatCount(msg.Scanned))
	}
	s.NotifyInfo(notice)
}

// HandleMissingTrack sets the transient notification to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) HandleMissingTrack(path string) {
	s.NotifyError(fmt.Sprintf("Error: track not found, skipped: %s", path))
}
