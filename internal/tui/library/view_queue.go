package library

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
)

var errNilQueueAppender = errors.New("nil queue appender")

// EnqueueTrackCmd builds a tea.Cmd that appends track to the play queue.
// Returns nil when ok is false (no focused track). sourcePlaylist identifies
// the playlist the track was enqueued from; pass "" for non-playlist views.
// insertAfter controls positional insertion: -1 appends, >= 0 inserts after
// that queue index.
func EnqueueTrackCmd(track musiclib.Track, ok bool, qa QueueAppender, prevQueueLen int, sourcePlaylist string, insertAfter int) tea.Cmd {
	if !ok {
		return nil
	}
	wasEmpty := prevQueueLen == 0
	return func() tea.Msg {
		if qa == nil {
			return TrackEnqueuedMsg{Track: track, Err: errNilQueueAppender, WasEmpty: wasEmpty, SourcePlaylist: sourcePlaylist, InsertAfter: insertAfter}
		}
		id, err := qa.AppendTrack(track, sourcePlaylist)
		return TrackEnqueuedMsg{Track: track, QueueID: id, Err: err, WasEmpty: wasEmpty, SourcePlaylist: sourcePlaylist, InsertAfter: insertAfter}
	}
}

// EnqueueFocused dispatches a tea.Cmd that appends the focused track to the
// play queue via qa. prevQueueLen is the number of entries in the queue before
// this call; it is forwarded in TrackEnqueuedMsg.WasEmpty so the root model
// can decide whether to trigger immediate playback. Returns nil Cmd when the
// library is empty or the cursor is out of range.
func (v Panel) EnqueueFocused(qa QueueAppender, prevQueueLen int) (Panel, tea.Cmd) {
	track, ok := v.FocusedTrack()
	return v, EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", -1)
}

// PlayNextFocused dispatches a tea.Cmd that inserts the focused track into the
// play queue at position insertAfter (immediately after the currently playing
// track). The DB append is identical to EnqueueFocused; the root model handles
// positional insertion via the InsertAfter field on TrackEnqueuedMsg.
func (v Panel) PlayNextFocused(qa QueueAppender, prevQueueLen int, insertAfter int) (Panel, tea.Cmd) {
	track, ok := v.FocusedTrack()
	return v, EnqueueTrackCmd(track, ok, qa, prevQueueLen, "", insertAfter)
}
