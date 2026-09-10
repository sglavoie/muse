package db

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/radio"
)

// QueueProvenance records who put a queue row in the play queue and, for a DJ
// selection, why the planner picked it.
//
// Ownership is explicit and stored per row. It is never derived from
// source_playlist, which holds a user-visible display label: a user may name a
// playlist "Radio", and rows written before migration 024 carry no ownership
// information at all. Both cases read back as manual entries, so a DJ refresh
// leaves them alone.
type QueueProvenance struct {
	// DJOwned reports that the DJ chose this row and may replace it when the
	// user applies new settings. False for every entry the user created:
	// explicit enqueues, pasted copies, and playlist-derived copies.
	DJOwned bool
	// Reasons is the planner's explanation for the selection, empty for manual
	// entries. It is display data only; nothing decides ownership from it.
	Reasons radio.Reasons
}

// ManualProvenance is the provenance of an entry the user created. It is the
// zero value, so a caller that forgets to set provenance gets the protected
// classification rather than an unowned row the DJ may delete.
func ManualProvenance() QueueProvenance { return QueueProvenance{} }

// DJProvenance is the provenance of a DJ-selected entry carrying the reasons it
// was chosen.
func DJProvenance(reasons radio.Reasons) QueueProvenance {
	return QueueProvenance{DJOwned: true, Reasons: reasons}
}

// djOwnedFlag renders DJOwned as the integer the queue column stores.
func (p QueueProvenance) djOwnedFlag() int {
	if p.DJOwned {
		return 1
	}
	return 0
}

// storedReason is the on-disk shape of one radio.Reason. The field names are
// part of the stored format; renaming one silently orphans every saved reason.
type storedReason struct {
	Code   string `json:"code"`
	Value  string `json:"value,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// encodeQueueReasons renders reasons as the JSON array stored in
// queue.dj_reasons. An empty list encodes as the empty string rather than "[]"
// so manual rows and DJ rows without reasons share one representation.
func encodeQueueReasons(reasons radio.Reasons) (string, error) {
	if len(reasons) == 0 {
		return "", nil
	}
	stored := make([]storedReason, 0, len(reasons))
	for _, reason := range reasons {
		stored = append(stored, storedReason{
			Code:   string(reason.Code),
			Value:  reason.Value,
			Detail: reason.Detail,
		})
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("db: encode queue reasons: %w", err)
	}
	return string(encoded), nil
}

// decodeQueueReasons parses a stored dj_reasons value. An empty or blank column
// yields no reasons and no error.
func decodeQueueReasons(raw string) (radio.Reasons, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var stored []storedReason
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("db: decode queue reasons: %w", err)
	}
	if len(stored) == 0 {
		return nil, nil
	}
	reasons := make(radio.Reasons, 0, len(stored))
	for _, s := range stored {
		reasons = append(reasons, radio.Reason{
			Code:   radio.ReasonCode(s.Code),
			Value:  s.Value,
			Detail: s.Detail,
		})
	}
	return reasons, nil
}
