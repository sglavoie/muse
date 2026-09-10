// Package autodj renders the Tune panel: the overlay where a user reads and
// edits the Auto-DJ settings that decide what the DJ plays next.
//
// The panel is a draft editor, not a live control surface. Every keystroke here
// changes a private copy of the settings, and nothing reaches the running
// session until the user commits it. That separation is the whole point: the
// applied snapshot is what the planner, the refill, and the persisted session
// row all agree on, so a half-typed filter must never be visible to any of them.
//
// The other half of the design is the commit round trip. Committing hands the
// draft to the root model and marks the panel busy; the outcome comes back as an
// Applied acknowledgement carrying the token the commit was issued with. An
// acknowledgement only ever moves the *applied* baseline. It never writes the
// draft, because the user is free to keep editing while a commit is in flight
// and their newer edits must survive the answer to the older question.
package autodj

import (
	"sort"
	"strconv"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// Catalog supplies the values the inclusion and exclusion pickers offer. It is
// a snapshot taken when the panel opens: the pickers are a modal gesture, so a
// library rescan landing mid-edit must not renumber the rows under the cursor.
type Catalog struct {
	Tags    []db.TagRow
	Genres  []string
	Artists []string
}

// Options is everything the panel needs to open.
type Options struct {
	// Applied is the settings snapshot currently in force.
	Applied radio.Settings
	// Catalog is the choices the filter pickers offer.
	Catalog Catalog
	// Active reports whether a DJ session is running, which is what decides
	// whether the commit is an Apply or a Start.
	Active bool
}

// Action is what a key press asks the root model to do.
type Action int

const (
	// actionNone means the panel handled the key on its own.
	actionNone Action = iota
	// ActionCommit asks for the draft to be applied (or started).
	ActionCommit
	// ActionCancel asks for the panel to close, discarding the draft.
	ActionCancel
)

// Applied is the outcome the root model reports for one commit.
type Applied struct {
	// Token identifies the commit this acknowledgement answers.
	Token uint64
	// Settings is the snapshot now in force. On failure it is the snapshot that
	// remained in force.
	Settings radio.Settings
	// Active reports whether a session is running after the commit.
	Active bool
	// Err is the reason the commit was refused, or nil.
	Err error
}

// TunePanel is the Tune tab of the Auto-DJ overlay.
type TunePanel struct {
	applied radio.Settings
	draft   radio.Settings
	catalog Catalog

	// active decides the commit's label and is refreshed by every
	// acknowledgement, so a session that stopped under the panel relabels its
	// button rather than offering an Apply with nothing to apply to.
	active bool

	cursor int

	// picker is the modal sub-state that owns every key while a filter field is
	// being edited.
	picker *filterPicker

	// token is the commit in flight, or 0 when idle. It is supplied by the root
	// model rather than counted here: a panel closed and reopened would restart
	// its own counter and mistake an older answer for its own.
	token uint64
	busy  bool

	err error
}

// New opens a Tune panel on a private copy of the applied settings.
func New(opts Options) TunePanel {
	applied := cloneSettings(opts.Applied)
	return TunePanel{
		applied: applied,
		draft:   cloneSettings(applied),
		catalog: opts.Catalog,
		active:  opts.Active,
	}
}

// Draft returns a copy of the edited settings. The copy is deep so a caller
// holding the result cannot reach back into the panel's own slices.
func (p TunePanel) Draft() radio.Settings { return cloneSettings(p.draft) }

// AppliedSettings returns the baseline the draft is compared against.
func (p TunePanel) AppliedSettings() radio.Settings { return cloneSettings(p.applied) }

// Dirty reports whether the draft differs from the applied snapshot.
func (p TunePanel) Dirty() bool { return !p.draft.Equal(p.applied) }

// Busy reports whether a commit is in flight.
func (p TunePanel) Busy() bool { return p.busy }

// Err returns the last refusal shown to the user, or nil.
func (p TunePanel) Err() error { return p.err }

// Cursor returns the focused row index.
func (p TunePanel) Cursor() int { return p.cursor }

// CommitLabel is the verb the primary action offers: a running session is
// re-tuned, a stopped one is started.
func (p TunePanel) CommitLabel() string {
	if p.active {
		return "Apply"
	}
	return "Start"
}

// Submit records the draft as handed to the root model under token.
//
// The draft itself is deliberately not frozen. The user keeps editing while the
// commit runs, and Ack below is what keeps those edits: nothing between here and
// the acknowledgement reads the draft again.
func (p TunePanel) Submit(token uint64) TunePanel {
	p.token = token
	p.busy = true
	p.err = nil
	return p
}

// Ack files the outcome of one commit.
//
// Two things make this safe to call from a message handler. An acknowledgement
// carrying anything but the token in flight is late — a superseded commit, or
// one issued by a previous opening of the panel — and is ignored outright. And a
// successful acknowledgement moves the applied baseline only: the draft is the
// user's, and overwriting it here would silently discard whatever they typed
// while the commit was running.
func (p TunePanel) Ack(res Applied) TunePanel {
	if !p.busy || res.Token != p.token {
		return p
	}
	p.busy = false
	p.token = 0
	p.active = res.Active
	p.err = res.Err
	if res.Err != nil {
		return p
	}
	p.applied = cloneSettings(res.Settings)
	return p
}

// Unresolved returns the tag ids the draft references that the catalog does not
// hold, in ascending order.
//
// These are blocking rather than repairable behind the user's back: dropping a
// stale id from an exclusion would widen what the DJ may play, and dropping one
// from an inclusion would widen it further still. The panel names them and
// offers the repair; it never performs it unasked.
func (p TunePanel) Unresolved() []int64 {
	known := make(map[int64]struct{}, len(p.catalog.Tags))
	for _, t := range p.catalog.Tags {
		known[t.ID] = struct{}{}
	}
	seen := make(map[int64]struct{})
	var missing []int64
	for _, ids := range [][]int64{p.draft.Include.TagIDs, p.draft.Exclude.TagIDs} {
		for _, id := range ids {
			if _, ok := known[id]; ok {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			missing = append(missing, id)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i] < missing[j] })
	return missing
}

// repairUnresolved drops every tag reference the catalog cannot resolve.
func (p TunePanel) repairUnresolved() TunePanel {
	missing := p.Unresolved()
	if len(missing) == 0 {
		return p
	}
	drop := make(map[int64]struct{}, len(missing))
	for _, id := range missing {
		drop[id] = struct{}{}
	}
	p.draft.Include.TagIDs = withoutTagIDs(p.draft.Include.TagIDs, drop)
	p.draft.Exclude.TagIDs = withoutTagIDs(p.draft.Exclude.TagIDs, drop)
	p.err = nil
	return p
}

// commitRefusal returns the reason the draft cannot be committed, or nil.
func (p TunePanel) commitRefusal() error {
	if missing := p.Unresolved(); len(missing) > 0 {
		return &radio.UnresolvedTagError{TagIDs: missing}
	}
	return p.draft.Validate()
}

// filterSet returns the draft's inclusion or exclusion set.
func (p TunePanel) filterSet(exclude bool) radio.FilterSet {
	if exclude {
		return p.draft.Exclude
	}
	return p.draft.Include
}

// withFilterSet returns a copy whose inclusion or exclusion set is set.
func (p TunePanel) withFilterSet(exclude bool, set radio.FilterSet) TunePanel {
	if exclude {
		p.draft.Exclude = set
		return p
	}
	p.draft.Include = set
	return p
}

// fieldValues returns the field's current selection as picker keys.
func fieldValues(set radio.FilterSet, f filterField) []string {
	switch f {
	case fieldTags:
		keys := make([]string, len(set.TagIDs))
		for i, id := range set.TagIDs {
			keys[i] = strconv.FormatInt(id, 10)
		}
		return keys
	case fieldGenres:
		return append([]string(nil), set.Genres...)
	case fieldArtists:
		return append([]string(nil), set.Artists...)
	}
	return nil
}

// withFieldValues returns set with field f replaced by keys.
func withFieldValues(set radio.FilterSet, f filterField, keys []string) radio.FilterSet {
	switch f {
	case fieldTags:
		ids := make([]int64, 0, len(keys))
		for _, key := range keys {
			if id, err := strconv.ParseInt(key, 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
		set.TagIDs = ids
	case fieldGenres:
		set.Genres = append([]string(nil), keys...)
	case fieldArtists:
		set.Artists = append([]string(nil), keys...)
	}
	return set
}

// cloneSettings deep-copies a settings snapshot so a draft and the applied
// state can never share a filter slice. Settings is a value type, but its filter
// sets carry slices, and an append to one of those would otherwise be visible
// through both.
func cloneSettings(s radio.Settings) radio.Settings {
	s.Include = cloneFilterSet(s.Include)
	s.Exclude = cloneFilterSet(s.Exclude)
	return s
}

func cloneFilterSet(f radio.FilterSet) radio.FilterSet {
	return radio.FilterSet{
		TagIDs:  append([]int64(nil), f.TagIDs...),
		Genres:  append([]string(nil), f.Genres...),
		Artists: append([]string(nil), f.Artists...),
	}
}

func withoutTagIDs(ids []int64, drop map[int64]struct{}) []int64 {
	kept := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, gone := drop[id]; gone {
			continue
		}
		kept = append(kept, id)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}
