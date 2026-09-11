package tui

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

type failingStartupTagReader struct {
	stubTagReader
	assignmentErr error
	catalogErr    error
}

func (r *failingStartupTagReader) ReadTrackTagIDs() (map[string]map[int64]struct{}, error) {
	return nil, r.assignmentErr
}

func (r *failingStartupTagReader) ReadTags() ([]db.TagRow, error) {
	return r.tags, r.catalogErr
}

func TestStartupTagLoadFailureBlocksDJFilters(t *testing.T) {
	loadErr := errors.New("tag read failed")
	for _, catalogFailure := range []bool{false, true} {
		name := "assignments"
		if catalogFailure {
			name = "catalog"
		}
		t.Run(name, func(t *testing.T) {
			r := &failingStartupTagReader{stubTagReader: stubTagReader{tags: []db.TagRow{{ID: 7, Name: "Focus"}}}}
			if catalogFailure {
				r.catalogErr = loadErr
			} else {
				r.assignmentErr = loadErr
			}
			m := newDJModel(WithTagReader(r))
			for _, exclude := range []bool{false, true} {
				req := djTestRequest()
				if exclude {
					req.Settings.Exclude.TagIDs = []int64{7}
				} else {
					req.Settings.Include.TagIDs = []int64{7}
				}
				plan, _, err := djPlanBatch(m.captureDJInputs(djFixedNow), req, radio.Allocation{}, nil, djFixedNow)
				if !errors.Is(err, errDJTagCatalogUnavailable) || !errors.Is(err, loadErr) || len(plan.Tracks) != 0 {
					t.Fatalf("exclude=%v: failed read produced plan %+v, err %v", exclude, plan, err)
				}
			}
			if _, _, err := djPlanBatch(m.captureDJInputs(djFixedNow), djTestRequest(), radio.Allocation{}, nil, djFixedNow); err != nil {
				t.Fatalf("unfiltered DJ unnecessarily blocked: %v", err)
			}
			if catalogFailure {
				return
			}

			// A catalog refresh or a single-track gesture cannot vouch for the
			// assignments on the rest of the library after the startup read failed.
			m = m.handleTagsRefreshed(tagsRefreshedMsg{tags: r.tags})
			m = m.applyTagToggle(7, []string{"d1"}, true)
			if err := m.captureDJInputs(djFixedNow).eligibilityErr(djTaggedSettings(7)); !errors.Is(err, loadErr) {
				t.Fatalf("partial repair cleared the assignment failure: %v", err)
			}
			m = m.handleTagsRefreshed(tagsRefreshedMsg{tags: r.tags, trackTags: map[string]map[int64]struct{}{"d1": {7: {}}}})
			req := djTestRequest()
			req.Settings = djTaggedSettings(7)
			plan, _, err := djPlanBatch(m.captureDJInputs(djFixedNow), req, radio.Allocation{}, nil, djFixedNow)
			if err != nil || len(plan.Tracks) != 1 || plan.Tracks[0].Track.Hash != "d1" {
				t.Fatalf("full refresh did not recover: %+v, %v", plan, err)
			}
		})
	}
}

func TestStartupPopulatesTrackTagsForDJFilters(t *testing.T) {
	for _, exclude := range []bool{false, true} {
		name := "include"
		if exclude {
			name = "exclude"
		}
		t.Run(name, func(t *testing.T) {
			j := newDJJourney(t)
			id := j.createTag("Focus")
			tagged := []string{"jt00", "jt02", "jt04", "jt06", "jt08", "jt10", "jt12", "jt14"}
			for _, hash := range tagged {
				if err := j.w.AddTrackTag(hash, id); err != nil {
					t.Fatal(err)
				}
			}
			s := radio.DefaultSettings()
			if exclude {
				s.Exclude.TagIDs = []int64{id}
			} else {
				s.Include.TagIDs = []int64{id}
			}
			if err := j.w.WriteDJSession(db.DJSession{
				Active: true, Settings: s, Seed: radio.WholeLibrarySeed(radio.SystemClock()),
			}); err != nil {
				t.Fatal(err)
			}
			if err := j.w.WriteRadioMode(true); err != nil {
				t.Fatal(err)
			}

			// Reopen the database and use New with the production options. No tag
			// gesture, direct cache assignment, or test-only refresh may help it.
			j.relaunch()
			j.refill()
			owned := j.djOwnedHashes()
			if len(owned) == 0 || j.m.DJPlanBlocked() {
				t.Fatalf("restored %s filter produced no batch: %v", name, j.m.djPlanBlockReasons())
			}
			for _, hash := range owned {
				if slices.Contains(tagged, hash) == exclude {
					t.Errorf("%s filter selected ineligible track %q", name, hash)
				}
			}

			j.m, _ = j.m.djStartFor(djSeedCurrent, time.Now())
			if !slices.Contains(j.m.DJSeed().TagIDs, id) {
				t.Fatalf("new seed lost the current track's stored tags: %+v", j.m.DJSeed())
			}
		})
	}
}
