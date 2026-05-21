package tui

import (
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// notifPriority controls which notification wins when multiple are queued.
// Higher values win; the queue evicts the lowest-priority entry when full.
type notifPriority uint8

const (
	notifInfo   notifPriority = iota // scan progress, sort, volume, mode toggles
	notifNormal                      // action results, errors, undo prompts
	notifUndo                        // reserved: never-overwritten slot (unused — kept for future use)
)

// notificationFadeWindow is the trailing slice of a notification's lifetime
// during which it renders dim (the "fade" frame) before vanishing.
const notificationFadeWindow = 250 * time.Millisecond

// transientNotification holds a single pending notification for the status bar
// right zone. expiresAt is the time after which the notification should be
// cleared by the tick handler. A zero expiresAt means no notification is set.
type transientNotification struct {
	msg       string
	expiresAt time.Time
	priority  notifPriority
}

// pushMsg enqueues a notification with an explicit duration and priority.
// The queue is capped at 5 entries; when full the lowest-priority oldest entry
// is evicted. If all existing entries outrank the incoming one it is dropped.
func (s *StatusBar) pushMsg(msg string, dur time.Duration, priority notifPriority) {
	const maxQueue = 5
	n := transientNotification{msg: msg, expiresAt: time.Now().Add(dur), priority: priority}
	s.pruneNotifications()
	if len(s.notifications) < maxQueue {
		s.notifications = append(s.notifications, n)
		return
	}

	evictIdx := -1
	var evictPrio notifPriority
	for i, existing := range s.notifications {
		if evictIdx < 0 || existing.priority < evictPrio {
			evictIdx = i
			evictPrio = existing.priority
		}
	}
	if evictIdx >= 0 && evictPrio <= priority {
		s.notifications = append(s.notifications[:evictIdx], s.notifications[evictIdx+1:]...)
		s.notifications = append(s.notifications, n)
	}
}

// isExpired reports whether n has expired. A zero expiresAt is treated as
// "no expiry" and is never expired (used for direct-assignment test helpers).
func (n transientNotification) isExpired() bool {
	return !n.expiresAt.IsZero() && time.Now().After(n.expiresAt)
}

// isFading reports whether n is within notificationFadeWindow of expiry.
// Zero expiresAt notifications never fade (treated as persistent).
func (n transientNotification) isFading() bool {
	if n.expiresAt.IsZero() {
		return false
	}
	return time.Until(n.expiresAt) <= notificationFadeWindow
}

// nextFadeAt returns the earliest moment (across all queued, non-expired
// notifications) at which a notification will enter its fade window. Returns a
// zero time when no future fade is pending. Notifications already inside the
// fade window are skipped — their fade frame is rendered by the current cycle.
func (s StatusBar) nextFadeAt() time.Time {
	var earliest time.Time
	for _, n := range s.notifications {
		if n.expiresAt.IsZero() || n.isExpired() {
			continue
		}
		fadeAt := n.expiresAt.Add(-notificationFadeWindow)
		if !time.Now().Before(fadeAt) {
			continue
		}
		if earliest.IsZero() || fadeAt.Before(earliest) {
			earliest = fadeAt
		}
	}
	return earliest
}

// pruneNotifications removes expired entries from the notifications queue.
func (s *StatusBar) pruneNotifications() {
	i := 0
	for _, notif := range s.notifications {
		if !notif.isExpired() {
			s.notifications[i] = notif
			i++
		}
	}
	s.notifications = s.notifications[:i]
}

// topNotification returns the highest-priority non-expired notification, or
// a zero value when the queue is empty or all entries have expired.
// Among entries with equal priority, the last-pushed one wins.
func (s StatusBar) topNotification() transientNotification {
	var top transientNotification
	for _, notif := range s.notifications {
		if notif.isExpired() {
			continue
		}
		if top.msg == "" || notif.priority >= top.priority {
			top = notif
		}
	}
	return top
}

// notify enqueues a notifNormal notification using the configured transient
// duration. Falls back to the default 5s constant when transientDur is zero
// (e.g. in tests that do not set config).
func (s *StatusBar) notify(msg string) {
	dur := s.transientDur
	if dur == 0 {
		dur = transientDuration
	}
	s.pushMsg(msg, dur, notifNormal)
}

// notifyInfo enqueues a notifInfo notification (scan, sort, volume, mode
// toggles). These yield to notifNormal when both are queued.
func (s *StatusBar) notifyInfo(msg string) {
	dur := s.transientDur
	if dur == 0 {
		dur = transientDuration
	}
	s.pushMsg(msg, dur, notifInfo)
}

// handleScanProgress sets the transient notification to a formatted progress
// string with a 5-second expiry. New calls overwrite the previous notification.
// When msg.Total is known (> 0) the format is "Scanning… X / Y tracks";
// before the count arrives it falls back to "Scanning… X tracks".
func (s *StatusBar) handleScanProgress(msg library.ScanProgressMsg) {
	var text string
	if msg.Total > 0 {
		text = fmt.Sprintf("Scanning… %s / %s tracks", formatCount(msg.Scanned), formatCount(msg.Total))
	} else {
		text = fmt.Sprintf("Scanning… %s tracks", formatCount(msg.Scanned))
	}
	s.notifyInfo(text)
}

// handleMissingTrack sets the transient notification to an error message
// indicating that a track could not be found and was skipped.
func (s *StatusBar) handleMissingTrack(path string) {
	s.notify(fmt.Sprintf("Error: track not found, skipped: %s", path))
}
