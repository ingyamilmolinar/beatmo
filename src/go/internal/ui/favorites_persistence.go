package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// persistedFavorites adapts a userprefs.Store to the UI-side
// FavoritesStore interface. The constructor warms an in-memory mirror
// from the backend so Get/Keys never block on disk; Set updates the
// mirror synchronously and schedules an async save via the backend's
// own pool (lazy-acquired on first save). Persistence I/O does not
// reach the UI goroutine.
type persistedFavorites struct {
	backend userprefs.Store

	mu  sync.RWMutex
	set map[string]struct{}
}

// NewPersistedFavoritesStore wraps a userprefs.Store. Loading happens
// once on construction; subsequent Get reads the in-memory mirror and
// is allocation-free. The caller retains ownership of the Store and
// must Close() it at shutdown to drain any pending writes.
func NewPersistedFavoritesStore(backend userprefs.Store) FavoritesStore {
	s := &persistedFavorites{
		backend: backend,
		set:     map[string]struct{}{},
	}
	if backend != nil {
		if loaded, _ := backend.LoadFavorites(); loaded != nil {
			for k, v := range loaded {
				if v {
					s.set[k] = struct{}{}
				}
			}
		}
	}
	return s
}

func (s *persistedFavorites) Get(key string) bool {
	if key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.set[key]
	return ok
}

func (s *persistedFavorites) Set(key string, fav bool) {
	if key == "" {
		return
	}
	s.mu.Lock()
	if fav {
		s.set[key] = struct{}{}
	} else {
		delete(s.set, key)
	}
	snap := s.snapshotLocked()
	s.mu.Unlock()
	if s.backend != nil {
		_ = s.backend.SaveFavorites(snap)
	}
}

func (s *persistedFavorites) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

// snapshotLocked builds a fresh map[string]bool from the internal set;
// caller holds s.mu. Returned to the backend so it can mutate the map
// without affecting the mirror.
func (s *persistedFavorites) snapshotLocked() map[string]bool {
	out := make(map[string]bool, len(s.set))
	for k := range s.set {
		out[k] = true
	}
	return out
}
