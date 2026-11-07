package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// The ring writes through Save after every push and seeds from Load; the backend's own pool keeps I/O off the
// UI goroutine.
type NotificationHistorySink interface {
	// Oldest first; nil when none, never a panic.
	Load() []notification
	// The backend bounds and persists asynchronously.
	Save([]notification)
}

// Keeps NotificationHistory() non-nil.
type noopNotifHistory struct{}

func (noopNotifHistory) Load() []notification { return nil }
func (noopNotifHistory) Save([]notification)  {}

var (
	notifHistoryGuard sync.RWMutex
	notifHistorySink  NotificationHistorySink = noopNotifHistory{}
)

// nil resets to the no-op default.
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
		out[i] = notification{key: i18n.Key(r.Key), args: r.Args, text: r.Text, isErr: r.IsErr, unixMs: r.UnixMs}
	}
	return out
}

func notifsToRecords(ns []notification) []userprefs.NotificationRecord {
	if len(ns) == 0 {
		return nil
	}
	out := make([]userprefs.NotificationRecord, len(ns))
	for i, n := range ns {
		out[i] = userprefs.NotificationRecord{Key: string(n.key), Args: n.args, Text: n.text, IsErr: n.isErr, UnixMs: n.unixMs}
	}
	return out
}
