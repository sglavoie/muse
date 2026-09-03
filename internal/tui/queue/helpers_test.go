package queue

import "charm.land/lipgloss/v2"

// makeQueue returns a View pre-loaded with the given entries.
func makeQueue(width int, entries []Entry) View {
	return View{
		Entries: entries,
		Width:   width,
	}
}

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}

// fixedTraversal is a TraversalSource handing back one prepared timeline, so a
// test can pin exactly which order the player would visit rows in.
type fixedTraversal struct{ t Traversal }

func (f fixedTraversal) QueueTraversal() Traversal { return f.t }

// shuffledTimeline builds an active shuffled timeline visiting order, with the
// player sitting at Order[current]. Pass -1 for "playing nothing".
func shuffledTimeline(current int, order ...int) TraversalSource {
	return fixedTraversal{Traversal{Order: order, CurrentPosition: current, Shuffled: true}}
}

// sequentialTimeline builds the timeline a player reports with shuffle off:
// queue order, so the view must keep behaving exactly as it did without a
// provider at all.
func sequentialTimeline(current, n int) TraversalSource {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return fixedTraversal{Traversal{Order: order, CurrentPosition: current}}
}

// entriesTitled builds one queue entry per title, giving each its own hash and
// path so identity never collapses by accident.
func entriesTitled(titles ...string) []Entry {
	entries := make([]Entry, len(titles))
	for i, title := range titles {
		entries[i] = Entry{
			Title:      title,
			Artist:     "A" + title,
			Hash:       "hash-" + title,
			Path:       "/music/" + title + ".flac",
			DurationMs: 60_000,
		}
	}
	return entries
}

// entryTitles is the display order a transformation produced, as titles.
func entryTitles(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Title
	}
	return out
}

// entryOrigIdxs is the underlying row identity behind each display row.
func entryOrigIdxs(entries []Entry) []int {
	out := make([]int, len(entries))
	for i, e := range entries {
		out[i] = e.OrigIdx
	}
	return out
}
