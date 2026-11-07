package ui

import (
	"sort"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Project pins (tier 0) outrank the user's ★ store (tier 1), which outranks the catalog (tier 2). Built fresh
// each frame: the maps are short. A new scope slots into this tier API, not a separate sort.
type PinSource struct {
	ProjectPins map[instrument.ID]struct{}
	UserStars   map[instrument.ID]struct{}
}

// Lower is more pinned; empty or unknown ids are tier 2.
func (p PinSource) Tier(id instrument.ID) int {
	if id == "" {
		return 2
	}
	if _, ok := p.ProjectPins[id]; ok {
		return 0
	}
	if _, ok := p.UserStars[id]; ok {
		return 1
	}
	return 2
}

// Pinned tiers sort to the top alphabetically; tier 2 keeps the catalog's curated order. With no pins it is a
// no-op, so the catalog order is never clobbered.
func (p PinSource) SortByTierAlpha(ids []instrument.ID, labelFor func(instrument.ID) string) {
	if len(ids) < 2 {
		return
	}
	if len(p.ProjectPins) == 0 && len(p.UserStars) == 0 {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool {
		ti := p.Tier(ids[i])
		tj := p.Tier(ids[j])
		if ti != tj {
			return ti < tj
		}
		if ti == 2 {
			// SliceStable keeps input order when less is false both ways.
			return false
		}
		return strings.ToLower(labelFor(ids[i])) < strings.ToLower(labelFor(ids[j]))
	})
}

// Pinned and ★ ids sort ABOVE unpinned ones whatever their score: favorites lead every search.
func (p PinSource) SortByTierScore(ids []instrument.ID, scoreOf func(instrument.ID) int, labelFor func(instrument.ID) string) {
	if len(ids) < 2 {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool {
		ti := p.Tier(ids[i])
		tj := p.Tier(ids[j])
		if ti != tj {
			return ti < tj
		}
		si := scoreOf(ids[i])
		sj := scoreOf(ids[j])
		if si != sj {
			return si > sj
		}
		return strings.ToLower(labelFor(ids[i])) < strings.ToLower(labelFor(ids[j]))
	})
}

// For ids already sorted by score: pinned items win equal-score ties without overriding a clearly higher score.
func (p PinSource) StableTierBreaker(ids []instrument.ID, scoreOf func(instrument.ID) int, labelFor func(instrument.ID) string) {
	if len(ids) < 2 {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool {
		si := scoreOf(ids[i])
		sj := scoreOf(ids[j])
		if si != sj {
			return si > sj
		}
		ti := p.Tier(ids[i])
		tj := p.Tier(ids[j])
		if ti != tj {
			return ti < tj
		}
		return strings.ToLower(labelFor(ids[i])) < strings.ToLower(labelFor(ids[j]))
	})
}

// Wired once at startup; safe from any goroutine. Get reads the in-memory mirror and never blocks on storage.
type FavoritesStore interface {
	Get(key instrument.ID) bool
	// The mirror reflects the call at once; storage may lag a few ms.
	Set(key instrument.ID, fav bool)
	// Alphabetical, deterministic.
	Keys() []instrument.ID
}

// The default before a store is registered, so callers never see nil.
type InMemoryFavoritesStore struct {
	mu  sync.RWMutex
	set map[instrument.ID]struct{}
}

func NewInMemoryFavoritesStore(ids ...instrument.ID) *InMemoryFavoritesStore {
	s := &InMemoryFavoritesStore{set: make(map[instrument.ID]struct{}, len(ids))}
	for _, id := range ids {
		if id == "" {
			continue
		}
		s.set[id] = struct{}{}
	}
	return s
}

func (s *InMemoryFavoritesStore) Get(key instrument.ID) bool {
	if s == nil || key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.set[key]
	return ok
}

func (s *InMemoryFavoritesStore) Set(key instrument.ID, fav bool) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if fav {
		s.set[key] = struct{}{}
		return
	}
	delete(s.set, key)
}

func (s *InMemoryFavoritesStore) Keys() []instrument.ID {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]instrument.ID, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

// An insertion sort for short lists keeps the file free of "sort".
func sortStrings[S ~string](s []S) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// One SetFavoritesStore at startup is the expected pattern; the lock is cheap.
var (
	favoritesGuard sync.RWMutex
	favorites      FavoritesStore = NewInMemoryFavoritesStore()
)

func SetFavoritesStore(s FavoritesStore) {
	favoritesGuard.Lock()
	defer favoritesGuard.Unlock()
	if s == nil {
		favorites = NewInMemoryFavoritesStore()
		return
	}
	favorites = s
}

// Never nil: an empty in-memory store when none is registered.
func Favorites() FavoritesStore {
	favoritesGuard.RLock()
	defer favoritesGuard.RUnlock()
	return favorites
}
