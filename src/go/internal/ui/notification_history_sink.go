package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// NotificationHistorySink is the UI-side persistence surface for the
// notification history. The live DrumView ring writes through Save after
// every push and seeds itself from Load at construction. Mirrors the
// FavoritesStore global-sink pattern so persistence I/O stays off the UI
// goroutine (the userprefs backend owns its own async pool).
type NotificationHistorySink interface {
	// Load returns persisted notifications oldest..newest (never panics;
	// returns nil when none exist).
	Load() []notification
	// Save replaces the persisted history with the given oldest..newest
	// snapshot (the backend bounds + persists asynchronously).
	Save([]notification)
}

// noopNotifHistory is the default sink: no persistence. Keeps
// NotificationHistory() non-nil so callers never need a nil check.
type noopNotifHistory struct{}

func (noopNotifHistory) Load() []notification { return nil }
func (noopNotifHistory) Save([]notification)  {}

var (
	notifHistoryGuard sync.RWMutex
	notifHistorySink  NotificationHistorySink = noopNotifHistory{}
)

// SetNotificationHistoryStore registers the global notification-history
// sink. Production wiring calls this once at startup with a
// userprefs-backed adapter; tests call it within t.Cleanup-restored scopes.
// A nil sink resets to the no-op default.
func SetNotificationHistoryStore(s NotificationHistorySink) {
	notifHistoryGuard.Lock()
	defer notifHistoryGuard.Unlock()
	if s == nil {
		notifHistorySink = noopNotifHistory{}
		return
	}
	notifHistorySink = s
}

// NotificationHistory returns the registered sink (never nil).
func NotificationHistory() NotificationHistorySink {
	notifHistoryGuard.RLock()
	defer notifHistoryGuard.RUnlock()
	return notifHistorySink
}

// persistedNotifHistory adapts a userprefs.NotificationHistoryStore to the
// UI-side NotificationHistorySink, converting between the UI notification
// struct and the persisted NotificationRecord shape.
type persistedNotifHistory struct {
	backend userprefs.NotificationHistoryStore
}

// NewPersistedNotificationHistory wraps a userprefs.NotificationHistoryStore.
func NewPersistedNotificationHistory(backend userprefs.NotificationHistoryStore) NotificationHistorySink {
	return &persistedNotifHistory{backend: backend}
}

func (p *persistedNotifHistory) Load() []notification {
	if p.backend == nil {
		return nil
	}
	recs, _ := p.backend.LoadNotifications()
	return recordsToNotifs(recs)
}

func (p *persistedNotifHistory) Save(snap []notification) {
	if p.backend == nil {
		return
	}
	_ = p.backend.SaveNotifications(notifsToRecords(snap))
}

func recordsToNotifs(recs []userprefs.NotificationRecord) []notification {
	if len(recs) == 0 {
		return nil
	}
	out := make([]notification, len(recs))
	for i, r := range recs {
		out[i] = notification{text: r.Text, isErr: r.IsErr, unixMs: r.UnixMs}
	}
	return out
}

func notifsToRecords(ns []notification) []userprefs.NotificationRecord {
	if len(ns) == 0 {
		return nil
	}
	out := make([]userprefs.NotificationRecord, len(ns))
	for i, n := range ns {
		out[i] = userprefs.NotificationRecord{Text: n.text, IsErr: n.isErr, UnixMs: n.unixMs}
	}
	return out
}
