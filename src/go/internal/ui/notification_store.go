package ui

import "time"

// notifHistoryCap bounds the in-memory + persisted notification ring.
const notifHistoryCap = 100

// Tests flip it to exercise the no-persistence path.
var UseNotificationsHistory = true

// nowUnixMilli is overridable so tests can pin timestamps deterministically.
var nowUnixMilli = func() int64 { return time.Now().UnixMilli() }

// Seeded (persisted) entries fill the history but are not session entries, so the live band starts idle.
type notificationStore struct {
	cap        int
	entries    []notification // oldest..newest
	sessionGen bool           // true once a real Push happened this session
	pushSeq    int64          // monotonic push counter (never resets; survives ring eviction)
	// nil in unit tests.
	persist func([]notification)
}

func newNotificationStore(capacity int) *notificationStore {
	if capacity < 1 {
		capacity = 1
	}
	return &notificationStore{cap: capacity}
}

func (s *notificationStore) Push(n notification) {
	s.entries = append(s.entries, n)
	if len(s.entries) > s.cap {
		s.entries = s.entries[len(s.entries)-s.cap:]
	}
	s.sessionGen = true
	s.pushSeq++
	if s.persist != nil {
		s.persist(s.snapshot())
	}
}

// Monotonic and eviction-proof: a robust change signal for memoizing consumers.
func (s *notificationStore) PushSeq() int64 { return s.pushSeq }

// Writes an EMPTY snapshot so a cleared history stays cleared across refreshes, and advances pushSeq so the badge
// invalidates. sessionGen stays: clearing is not a notification.
func (s *notificationStore) Clear() {
	s.entries = s.entries[:0]
	s.pushSeq++
	if s.persist != nil {
		s.persist(s.snapshot())
	}
}

// History returns the ring newest-first (copy).
func (s *notificationStore) History() []notification {
	out := make([]notification, len(s.entries))
	for i, n := range s.entries {
		out[len(s.entries)-1-i] = n
	}
	return out
}

// Latest returns the newest entry or nil when the ring is empty.
func (s *notificationStore) Latest() *notification {
	if len(s.entries) == 0 {
		return nil
	}
	n := s.entries[len(s.entries)-1]
	return &n
}

// Len reports the number of entries currently held.
func (s *notificationStore) Len() int { return len(s.entries) }

// The live band renders only after a real Push.
func (s *notificationStore) HasSessionEntry() bool { return s.sessionGen }

// Not a session entry.
func (s *notificationStore) seed(entries []notification) {
	if len(entries) > s.cap {
		entries = entries[len(entries)-s.cap:]
	}
	s.entries = append(s.entries[:0], entries...)
}

func (s *notificationStore) snapshot() []notification {
	out := make([]notification, len(s.entries))
	copy(out, s.entries)
	return out
}
