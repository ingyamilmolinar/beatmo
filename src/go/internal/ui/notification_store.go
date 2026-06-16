package ui

import "time"

// notifHistoryCap bounds the in-memory + persisted notification ring.
const notifHistoryCap = 100

// UseNotificationsHistory gates persistence of the notification history to
// userprefs. Tests flip it false to exercise the no-persistence path.
var UseNotificationsHistory = true

// nowUnixMilli is overridable so tests can pin timestamps deterministically.
var nowUnixMilli = func() int64 { return time.Now().UnixMilli() }

// notificationStore is the single source of truth for user notifications: a
// bounded oldest..newest ring plus accessors. The in-band area shows Latest()
// (only once a session entry exists); the history popup shows History()
// (newest-first). Seeded entries (from persistence) populate the ring for the
// popup but do NOT count as a session entry, so the live band starts idle.
type notificationStore struct {
	cap        int
	entries    []notification // oldest..newest
	sessionGen bool           // true once a real Push happened this session
	// persist, when set, is invoked with a fresh oldest..newest snapshot after
	// every Push so DrumView can write through to userprefs. nil in unit tests.
	persist func([]notification)
}

func newNotificationStore(capacity int) *notificationStore {
	if capacity < 1 {
		capacity = 1
	}
	return &notificationStore{cap: capacity}
}

// Push appends a live notification, trims to cap, marks the session as having
// produced an entry, and writes through the persist sink when present.
func (s *notificationStore) Push(n notification) {
	s.entries = append(s.entries, n)
	if len(s.entries) > s.cap {
		s.entries = s.entries[len(s.entries)-s.cap:]
	}
	s.sessionGen = true
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

// HasSessionEntry reports whether a real Push happened this session (vs only
// seeded history). The live band renders only when this is true.
func (s *notificationStore) HasSessionEntry() bool { return s.sessionGen }

// seed replaces the ring with persisted entries (oldest..newest), trimming to
// cap. It does NOT mark a session entry.
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
