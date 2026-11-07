package eventlogger

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// LogRecord is one retained, searchable log line: the event's timestamp, its
// category (Tag, the bracket prefix), the rendered message, and whether it is
// an error. Session-only; never persisted.
type LogRecord struct {
	Seq   int64
	At    time.Time
	Kind  hooks.Kind
	Tag   string
	Scope hooks.ActionScope
	Msg   string
	IsErr bool
}

// LogHistory retains recent LogRecords for the UI logs panel.
type LogHistory interface {
	// Record appends a record (bounded; oldest evicted).
	Record(LogRecord)
	// Snapshot returns retained records NEWEST-FIRST (copy — caller may mutate).
	Snapshot() []LogRecord
	// Cheap (no copy): the UI's change signal, so it never re-Snapshots the ring every frame.
	LatestSeq() int64
	// Clear drops all retained records (the "Clear history" action). The sink
	// remains usable and accepts new records afterwards.
	Clear()
}

// ringHistory is a bounded oldest..newest ring behind a mutex.
type ringHistory struct {
	mu      sync.RWMutex
	cap     int
	entries []LogRecord // oldest..newest
}

// NewRingHistory returns a bounded LogHistory. capacity < 1 is clamped to 1.
func NewRingHistory(capacity int) LogHistory {
	if capacity < 1 {
		capacity = 1
	}
	return &ringHistory{cap: capacity}
}

func (r *ringHistory) Record(rec LogRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, rec)
	if len(r.entries) > r.cap {
		r.entries = r.entries[len(r.entries)-r.cap:]
	}
}

func (r *ringHistory) Snapshot() []LogRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LogRecord, len(r.entries))
	for i, e := range r.entries {
		out[len(r.entries)-1-i] = e // newest-first
	}
	return out
}

func (r *ringHistory) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = r.entries[:0]
}

func (r *ringHistory) LatestSeq() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.entries) == 0 {
		return 0
	}
	return r.entries[len(r.entries)-1].Seq
}

// noopHistory is the default: retains nothing. Keeps History() non-nil.
type noopHistory struct{}

func (noopHistory) Record(LogRecord)      {}
func (noopHistory) Snapshot() []LogRecord { return nil }
func (noopHistory) LatestSeq() int64      { return 0 }
func (noopHistory) Clear()                {}

var (
	histGuard sync.RWMutex
	histSink  LogHistory = noopHistory{}
)

// SetLogHistory registers the global log-history sink. Production wires this
// once at startup with a NewRingHistory(cap); a nil sink resets to the no-op.
func SetLogHistory(h LogHistory) {
	histGuard.Lock()
	defer histGuard.Unlock()
	if h == nil {
		histSink = noopHistory{}
		return
	}
	histSink = h
}

// History returns the registered sink (never nil).
func History() LogHistory {
	histGuard.RLock()
	defer histGuard.RUnlock()
	return histSink
}
