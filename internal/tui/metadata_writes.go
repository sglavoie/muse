package tui

import "sync"

// metadataWriteQueue orders the asynchronous full-row metadata writes. Every
// writer changes memory on the UI goroutine and persists afterwards, so the
// order the user made the changes is the order they were published here — but
// the goroutines that carry them to the database can finish in any order. The
// genre merge makes that visible: it plans a full row at confirmation time and
// persists it later, so without an ordering rule it can land after an ordinary
// edit of the same track and revert fields the user just changed.
//
// The rule is that the newest ticket for a hash wins. An older writer that has
// not reached the database yet stands down instead of writing, and one that
// already has is overwritten by the newer write queued behind it.
//
// A nil queue is a Model that never built one: every write runs unordered,
// which is what the many Model{} literals in tests want.
type metadataWriteQueue struct {
	// writing is held across a database call so a stale writer cannot read the
	// current ticket, lose the race to a newer writer, and then overwrite it.
	writing sync.Mutex

	mu      sync.Mutex
	next    uint64
	current map[string]uint64 // hash → newest published ticket
	waiting map[string]int    // hash → tickets not yet claimed
}

func newMetadataWriteQueue() *metadataWriteQueue {
	return &metadataWriteQueue{current: map[string]uint64{}, waiting: map[string]int{}}
}

// publish claims the next write slot for hash. Call it on the UI goroutine
// right after memory changes and before dispatching the write, and pair every
// ticket with exactly one claim.
func (q *metadataWriteQueue) publish(hash string) uint64 {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.next++
	q.current[hash] = q.next
	q.waiting[hash]++
	return q.next
}

// claim runs write when ticket is still the newest intent for hash, and reports
// whether it ran. A false means a later change superseded this one, so the
// caller must not treat its snapshot as the truth anywhere else either.
func (q *metadataWriteQueue) claim(hash string, ticket uint64, write func() error) (bool, error) {
	if q == nil {
		return true, write()
	}
	q.writing.Lock()
	defer q.writing.Unlock()

	q.mu.Lock()
	current := q.current[hash]
	// Dropped only once every ticket for the hash has been claimed, so a writer
	// that ran first cannot erase the evidence a straggler needs to stand down.
	q.waiting[hash]--
	if q.waiting[hash] <= 0 {
		delete(q.waiting, hash)
		delete(q.current, hash)
	}
	q.mu.Unlock()

	if current != ticket {
		return false, nil
	}
	return true, write()
}
