package ui

import (
	"errors"
	"sync"
	"testing"
)

// recordingStore captures Save/Load calls for adapter testing without
// touching disk. It implements userprefs.Store via duck typing — we
// only need the four methods the adapter actually uses.
type recordingStore struct {
	mu          sync.Mutex
	loadResult  map[string]bool
	loadErr     error
	saved       []map[string]bool
	loadCalled  int
	saveCalled  int
	flushCalled int
}

func (s *recordingStore) LoadFavorites() (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadCalled++
	out := make(map[string]bool, len(s.loadResult))
	for k, v := range s.loadResult {
		out[k] = v
	}
	return out, s.loadErr
}

func (s *recordingStore) SaveFavorites(favs map[string]bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalled++
	snap := make(map[string]bool, len(favs))
	for k, v := range favs {
		snap[k] = v
	}
	s.saved = append(s.saved, snap)
	return nil
}

func (s *recordingStore) WaitFlushed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushCalled++
	return nil
}

func (s *recordingStore) Close() error { return nil }

func TestPersistedFavoritesLoadsOnConstruction(t *testing.T) {
	rec := &recordingStore{loadResult: map[string]bool{"kick": true, "snare": true}}
	s := NewPersistedFavoritesStore(rec)
	if rec.loadCalled != 1 {
		t.Fatalf("LoadFavorites called %d times, want 1", rec.loadCalled)
	}
	if !s.Get("kick") || !s.Get("snare") {
		t.Fatalf("expected loaded ids to be visible to Get; saved=%v", rec.saved)
	}
	if s.Get("unknown") {
		t.Fatalf("Get for unknown id must be false")
	}
}

func TestPersistedFavoritesSetSchedulesSave(t *testing.T) {
	rec := &recordingStore{loadResult: map[string]bool{}}
	s := NewPersistedFavoritesStore(rec)
	s.Set("kick", true)
	if rec.saveCalled != 1 {
		t.Fatalf("SaveFavorites called %d times after one Set, want 1", rec.saveCalled)
	}
	last := rec.saved[len(rec.saved)-1]
	if !last["kick"] {
		t.Fatalf("last save did not include kick: %v", last)
	}
	s.Set("kick", false)
	if rec.saveCalled != 2 {
		t.Fatalf("SaveFavorites called %d times after Set false, want 2", rec.saveCalled)
	}
	last = rec.saved[len(rec.saved)-1]
	if last["kick"] {
		t.Fatalf("after Set(kick,false), kick should not be in saved snapshot: %v", last)
	}
}

func TestPersistedFavoritesKeysSorted(t *testing.T) {
	rec := &recordingStore{loadResult: map[string]bool{"zeta": true, "alpha": true, "mike": true}}
	s := NewPersistedFavoritesStore(rec)
	keys := s.Keys()
	want := []string{"alpha", "mike", "zeta"}
	if len(keys) != len(want) {
		t.Fatalf("Keys len=%d want %d", len(keys), len(want))
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("Keys[%d]=%q want %q (full=%v)", i, keys[i], k, keys)
		}
	}
}

func TestPersistedFavoritesNilBackend(t *testing.T) {
	// Adapter must work even when backend is nil — useful for embedded
	// scenarios where persistence is not wired (e.g., -demo mode that
	// shouldn't write to user config dirs).
	s := NewPersistedFavoritesStore(nil)
	if s == nil {
		t.Fatalf("constructor returned nil for nil backend")
	}
	s.Set("kick", true)
	if !s.Get("kick") {
		t.Fatalf("nil-backend store should still mirror in-memory state")
	}
}

func TestPersistedFavoritesSurvivesLoadError(t *testing.T) {
	rec := &recordingStore{loadErr: errors.New("disk on fire")}
	s := NewPersistedFavoritesStore(rec)
	// Construction must not panic on load error; mirror starts empty.
	if got := s.Keys(); len(got) != 0 {
		t.Fatalf("expected empty mirror on load error, got %v", got)
	}
	// Subsequent operations still work.
	s.Set("a", true)
	if !s.Get("a") {
		t.Fatalf("Set after load-error should still update mirror")
	}
}

func TestPersistedFavoritesEmptyKeyIgnored(t *testing.T) {
	rec := &recordingStore{loadResult: map[string]bool{}}
	s := NewPersistedFavoritesStore(rec)
	s.Set("", true)
	if rec.saveCalled != 0 {
		t.Fatalf("Set with empty key should be a no-op; saves=%d", rec.saveCalled)
	}
	if s.Get("") {
		t.Fatalf("Get with empty key must be false")
	}
}
