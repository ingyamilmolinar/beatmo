package userprefs

import (
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// TestNotificationHistoryRoundTrip — SaveNotifications then LoadNotifications
// from a fresh store pointed at the same file must return the same records.
func TestNotificationHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewBackingStore(Options{
		Path:     filepath.Join(dir, "prefs.json"),
		PoolName: "userprefs.test-notif-roundtrip",
	})
	t.Cleanup(func() {
		_ = store.Close()
		_ = async.DefaultRegistry().Release("userprefs.test-notif-roundtrip")
	})
	ns, ok := store.(NotificationHistoryStore)
	if !ok {
		t.Fatalf("backing store does not implement NotificationHistoryStore")
	}
	want := []NotificationRecord{
		{Text: "Imported project", IsErr: false, UnixMs: 1000},
		{Text: "Error loading JSON", IsErr: true, UnixMs: 2000},
	}
	if err := ns.SaveNotifications(want); err != nil {
		t.Fatalf("SaveNotifications: %v", err)
	}
	if err := store.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	store2 := NewBackingStore(Options{
		Path:     filepath.Join(dir, "prefs.json"),
		PoolName: "userprefs.test-notif-roundtrip-2",
	})
	t.Cleanup(func() {
		_ = store2.Close()
		_ = async.DefaultRegistry().Release("userprefs.test-notif-roundtrip-2")
	})
	got, err := store2.(NotificationHistoryStore).LoadNotifications()
	if err != nil {
		t.Fatalf("LoadNotifications: %v", err)
	}
	if len(got) != 2 || got[0].Text != "Imported project" || got[1].IsErr != true || got[1].UnixMs != 2000 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

// TestNotificationHistoryCap — SaveNotifications keeps only the newest 100.
func TestNotificationHistoryCap(t *testing.T) {
	dir := t.TempDir()
	store := NewBackingStore(Options{
		Path:     filepath.Join(dir, "prefs.json"),
		PoolName: "userprefs.test-notif-cap",
	})
	t.Cleanup(func() {
		_ = store.Close()
		_ = async.DefaultRegistry().Release("userprefs.test-notif-cap")
	})
	ns := store.(NotificationHistoryStore)
	var recs []NotificationRecord
	for i := 0; i < 150; i++ {
		recs = append(recs, NotificationRecord{Text: "n", UnixMs: int64(i)})
	}
	if err := ns.SaveNotifications(recs); err != nil {
		t.Fatalf("SaveNotifications: %v", err)
	}
	got, err := ns.LoadNotifications()
	if err != nil {
		t.Fatalf("LoadNotifications: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("cap not enforced: got %d want 100", len(got))
	}
	// newest retained: the last record (UnixMs 149) must survive; oldest (0) dropped.
	if got[len(got)-1].UnixMs != 149 {
		t.Fatalf("newest dropped: last=%d want 149", got[len(got)-1].UnixMs)
	}
	if got[0].UnixMs != 50 {
		t.Fatalf("expected oldest-kept UnixMs 50, got %d", got[0].UnixMs)
	}
}

// TestNotificationHistoryEmptyDefault — a store with no saved notifications
// returns an empty slice, not an error.
func TestNotificationHistoryEmptyDefault(t *testing.T) {
	dir := t.TempDir()
	store := NewBackingStore(Options{
		Path:     filepath.Join(dir, "prefs.json"),
		PoolName: "userprefs.test-notif-empty",
	})
	t.Cleanup(func() {
		_ = store.Close()
		_ = async.DefaultRegistry().Release("userprefs.test-notif-empty")
	})
	got, err := store.(NotificationHistoryStore).LoadNotifications()
	if err != nil {
		t.Fatalf("LoadNotifications: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty history, got %+v", got)
	}
}
