package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// radioState holds all mutable radio-mode fields for the root Model.
type radioState struct {
	enabled        bool
	seedHash       string
	syntheticSeed  *radio.SeedTrack // non-nil = use directly instead of hash lookup
	enqueuePending bool
	recentHashes   []string // ring buffer, cap radioRecentCap
}

const (
	radioBatchSize    = 5
	radioLowWaterMark = 2
	radioRecentCap    = 30
)

// radioToggle flips radio mode on or off, notifies the user, and kicks off an
// initial enqueue when enabling.
func (m Model) radioToggle() (tea.Model, tea.Cmd, bool) {
	m.radio.enabled = !m.radio.enabled
	if m.sessionWriter != nil {
		if err := m.sessionWriter.WriteRadioMode(m.radio.enabled); err != nil {
			slog.Warn("session: write radio mode", "err", err)
		}
	}
	label := "off"
	if m.radio.enabled {
		label = "on"
		m.radio.seedHash = m.playback.currentHash
	} else {
		m.radio.syntheticSeed = nil
	}
	m.statusBar.pushMsg("Radio mode: "+label, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, notifInfo)
	if m.radio.enabled {
		cmd := m.radioMaybeEnqueueCmd()
		if cmd != nil {
			m.radio.enqueuePending = true
		}
		return m, cmd, true
	}
	return m, nil, true
}

// radioRemainingCount returns the number of queue entries after the current
// playing index (tracks still to be played).
func (m Model) radioRemainingCount() int {
	if m.playback.currentIndex < 0 {
		return len(m.queue.entries)
	}
	return len(m.queue.entries) - m.playback.currentIndex - 1
}

// radioMaybeEnqueueCmd returns an enqueue Cmd when all guards pass, or nil.
// Guards: radio enabled, no pending enqueue, stopAfterCurrent not set,
// repeatMode != RepeatAll, library non-empty, remaining <= low-water-mark.
func (m Model) radioMaybeEnqueueCmd() tea.Cmd {
	if !m.radio.enabled {
		return nil
	}
	if m.radio.enqueuePending {
		return nil
	}
	if m.playback.stopAfterCurrent {
		return nil
	}
	if m.playback.repeatMode == RepeatAll {
		return nil
	}
	if len(m.library.index.Tracks()) == 0 {
		return nil
	}
	if m.radioRemainingCount() > radioLowWaterMark {
		return nil
	}
	return m.radioEnqueueCmd()
}

// radioEnqueueCmd captures the current scoring inputs and returns a tea.Cmd
// that scores the library, picks a batch, appends to DB, and returns
// RadioBatchEnqueuedMsg.
func (m Model) radioEnqueueCmd() tea.Cmd {
	if m.batchEnqueuer == nil {
		return nil
	}

	seed := radio.SeedTrack{}
	if m.radio.syntheticSeed != nil {
		seed = *m.radio.syntheticSeed
	} else {
		seedHash := m.radio.seedHash
		if seedHash == "" {
			seedHash = m.playback.currentHash
		}
		if t, ok := m.library.index.FindByHash(seedHash); ok {
			seed.Hash = t.Hash
			seed.Artist = t.Artist
			seed.Album = t.Album
			seed.Genres = library.SplitGenres(t.Genre)
			seed.Duration = t.Duration
			if tags, ok2 := m.trackTags[seedHash]; ok2 {
				// Snapshot the tag set: the live map is mutated by the Update
				// loop (applyTagToggle) and reading it concurrently from the
				// closure goroutine would race.
				seedTags := make(map[int64]struct{}, len(tags))
				for id := range tags {
					seedTags[id] = struct{}{}
				}
				seed.TagIDs = seedTags
			}
		} else if seedHash != "" {
			seed.Hash = seedHash
		}
	}

	recentSet := make(map[string]struct{}, len(m.radio.recentHashes))
	for _, h := range m.radio.recentHashes {
		recentSet[h] = struct{}{}
	}

	queueSet := make(map[string]struct{}, len(m.queue.entries))
	for _, e := range m.queue.entries {
		queueSet[e.Hash] = struct{}{}
	}

	wasEmpty := len(m.queue.entries) == 0
	allTracks := snapshotTracks(m.library.index.Tracks())
	trackTags := snapshotTrackTags(m.trackTags)
	favoriteHashes := snapshotStringSet(m.favoriteHashes)
	dismissedHashes := snapshotStringSet(m.dismissedHashes)
	be := m.batchEnqueuer
	recReader := m.recReader

	return func() tea.Msg {
		// Two-pass scoring: first pass with metadata only to narrow candidates,
		// then load behavioral data for top 3*batchSize pool before final score.
		completionRates := map[string]float64{}
		coOccurrences := map[string]int{}
		timeGenreBonus := map[string]float64{}
		if recReader != nil && seed.Hash != "" {
			initial := radio.ScoreAll(radio.ScoreInput{
				Seed:            seed,
				AllTracks:       allTracks,
				TrackTags:       trackTags,
				FavoriteHashes:  favoriteHashes,
				RecentHashes:    recentSet,
				QueueHashes:     queueSet,
				DismissedHashes: dismissedHashes,
			})
			topN := min(3*radioBatchSize, len(initial))
			topHashes := make([]string, topN)
			for i := 0; i < topN; i++ {
				topHashes[i] = initial[i].Track.Hash
			}
			if rates, err := recReader.ReadCompletionRates(topHashes); err == nil && rates != nil {
				completionRates = rates
			}
			if coOccs, err := recReader.ReadSessionCoOccurrences(seed.Hash, 100); err == nil {
				for _, c := range coOccs {
					coOccurrences[c.Hash] = c.Count
				}
			}
			if affinities, err := recReader.ReadHourGenreAffinities(); err == nil {
				timeGenreBonus = buildTimeGenreBonus(affinities, time.Now().Hour())
			}
		}

		scored := radio.ScoreAll(radio.ScoreInput{
			Seed:            seed,
			AllTracks:       allTracks,
			TrackTags:       trackTags,
			FavoriteHashes:  favoriteHashes,
			RecentHashes:    recentSet,
			QueueHashes:     queueSet,
			DismissedHashes: dismissedHashes,
			CompletionRates: completionRates,
			CoOccurrences:   coOccurrences,
			TimeGenreBonus:  timeGenreBonus,
		})
		picked := radio.PickBatch(scored, radioBatchSize, 0.4)
		if len(picked) == 0 {
			return RadioBatchEnqueuedMsg{}
		}
		ids, err := be.AppendTracks(picked, "Radio")
		if err != nil {
			return RadioBatchEnqueuedMsg{Err: err}
		}
		entries := newQueueEntries(picked, ids, "Radio")
		return RadioBatchEnqueuedMsg{
			Entries:  entries,
			QueueIDs: ids,
			WasEmpty: wasEmpty,
		}
	}
}

// buildTimeGenreBonus converts hour-genre affinity rows into a bonus map for
// the given hour. The top genre for the hour receives +3 and the runner-up
// receives +1. Affinities must be pre-sorted by (hour, count DESC) as returned
// by ReadHourGenreAffinities.
func buildTimeGenreBonus(affinities []db.HourGenreAffinity, hour int) map[string]float64 {
	bonus := make(map[string]float64)
	bonusValues := [2]float64{3, 1}
	rank := 0
	for _, a := range affinities {
		if a.Hour != hour {
			continue
		}
		bonus[a.Genre] = bonusValues[rank]
		rank++
		if rank >= len(bonusValues) {
			break
		}
	}
	return bonus
}

// handleRadioBatchEnqueuedMsg appends radio-picked tracks to the queue,
// triggers auto-resume if the queue was exhausted, and schedules the next
// low-water-mark check.
func (m Model) handleRadioBatchEnqueuedMsg(msg RadioBatchEnqueuedMsg) (Model, tea.Cmd) {
	m.radio.enqueuePending = false

	if msg.Err != nil {
		slog.Warn("radio enqueue failed", "err", msg.Err)
		return m, nil
	}
	if len(msg.Entries) == 0 {
		return m, nil
	}
	// Discard the batch if radio was toggled off or the queue was cleared
	// while the enqueue goroutine was in flight. The DB rows were already
	// appended by the goroutine but appending them to the empty/disabled
	// queue would resurrect playback the user just cancelled.
	if !m.radio.enabled {
		return m, nil
	}
	// Queue-cleared-in-flight guard: when the radio batch was launched the
	// queue had remaining tracks (low-water-mark trigger), so msg.WasEmpty is
	// false. If the queue is empty by the time the batch arrives the user
	// might have cleared it; silently appending the batch would then
	// resurrect a queue the user just emptied. Distinguish natural
	// exhaustion (queueExhausted=true, set by handlePlaybackEndedMsg) from a
	// user-initiated clear (queueExhausted reset by resetPlaybackState).
	// Only the former should auto-resume by appending the in-flight batch.
	if !msg.WasEmpty && len(m.queue.entries) == 0 && !m.playback.queueExhausted {
		return m, nil
	}

	// Update recent-hashes ring buffer.
	for _, e := range msg.Entries {
		m.radio.recentHashes = append(m.radio.recentHashes, e.Hash)
	}
	if len(m.radio.recentHashes) > radioRecentCap {
		m.radio.recentHashes = m.radio.recentHashes[len(m.radio.recentHashes)-radioRecentCap:]
	}

	batchStartIdx := len(m.queue.entries)
	m.queue.entries = append(m.queue.entries, msg.Entries...)
	m = m.preserveQueueSort()
	if appender, ok := m.player.(playerQueueAppender); ok {
		for _, entry := range msg.Entries {
			appender.AppendToQueue(queueTrackFromEntry(entry))
		}
	}

	// Auto-resume when the queue had run dry.
	if m.playback.queueExhausted && m.playback.state == PlaybackIdle && m.player != nil {
		if err := m.player.PlayFromIndex(batchStartIdx); err == nil {
			entry := msg.Entries[0]
			m.playback.state = PlaybackPlaying
			m.playback.currentHash = entry.Hash
			m.playback.currentIndex = batchStartIdx
			m = m.resetPlaybackProgress()
			m.statusBar.artist = entry.Artist
			m.statusBar.title = entry.Title
			m.statusBar.playlistSource = queueSourceName("Radio")
		}
		m.playback.queueExhausted = false
	} else if msg.WasEmpty && m.playback.state == PlaybackIdle {
		entry := msg.Entries[0]
		m = m.tryAutoPlay(entry.Hash, entry.Artist, entry.Title, "Radio")
	}

	return m, nil
}
