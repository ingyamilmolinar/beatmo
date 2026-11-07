package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// Get/Keys read a mirror warmed at construction; Set updates it synchronously and saves on the backend's
// pool, so persistence I/O never reaches the UI goroutine.
type persistedFavorites struct {
	backend userprefs.Store

	mu  sync.RWMutex
	set map[instrument.ID]struct{}
}

// The caller keeps the Store and must Close() it at shutdown to drain pending writes.
func NewPersistedFavoritesStore(backend userprefs.Store) FavoritesStore {
	s := &persistedFavorites{
		backend: backend,
		set:     map[instrument.ID]struct{}{},
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

func (s *persistedFavorites) Get(key instrument.ID) bool {
	if key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.set[key]
	return ok
}

func (s *persistedFavorites) Set(key instrument.ID, fav bool) {
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

func (s *persistedFavorites) Keys() []instrument.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]instrument.ID, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

// Caller holds s.mu. A fresh map so the backend can mutate it without touching the mirror.
func (s *persistedFavorites) snapshotLocked() map[instrument.ID]bool {
	out := make(map[instrument.ID]bool, len(s.set))
	for k := range s.set {
		out[k] = true
	}
	return out
}
