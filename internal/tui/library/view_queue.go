package library

import tea "charm.land/bubbletea/v2"

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. prevQueueLen is the number of entries in the queue before
// this call; it is forwarded in TrackEnqueuedMsg.WasEmpty so the root model
// can decide whether to trigger immediate playback. Returns nil Cmd when the
// library is empty or the cursor is out of range.
func (v View) EnqueueFocused(qa QueueAppender, prevQueueLen int) (View, tea.Cmd) {
	track, ok := v.FocusedTrack()
	if !ok {
		return v, nil
	}
	wasEmpty := prevQueueLen == 0
	return v, func() tea.Msg {
		id, err := qa.AppendTrack(track, "")
		return TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, InsertAfter: -1}
	}
}

// PlayNextFocused dispatches a tea.Cmd that inserts the focused track into the
// play queue at position insertAfter (immediately after the currently playing
// track). The DB append is identical to EnqueueFocused; the root model handles
// positional insertion via the InsertAfter field on TrackEnqueuedMsg.
func (v View) PlayNextFocused(qa QueueAppender, prevQueueLen int, insertAfter int) (View, tea.Cmd) {
	track, ok := v.FocusedTrack()
	if !ok {
		return v, nil
	}
	wasEmpty := prevQueueLen == 0
	return v, func() tea.Msg {
		id, err := qa.AppendTrack(track, "")
		return TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, InsertAfter: insertAfter}
	}
}
