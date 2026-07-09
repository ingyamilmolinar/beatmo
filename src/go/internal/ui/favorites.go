package ui

import (
	"sort"
	"strings"
	"sync"
)

// PinSource is the per-render input describing which instrument ids are
// pinned, and at which tier. Project pins (tier 0, persisted with the
// project JSON) outrank the user's ★ store (tier 1, persisted in
// userprefs); both outrank the remaining catalog items (tier 2). Construct
// a fresh PinSource each frame — the maps are short and the cost is
// negligible compared to the menu's existing layout pass.
//
// Future scopes (Scope == "user", "remote", "project") should slot into
// this same tier API rather than introducing a separate sort path. The
// menu's render-time order is the only consumer; nothing else cares which
// tier an instrument lives in.
type PinSource struct {
	ProjectPins map[string]struct{}
	UserStars   map[string]struct{}
}

// Tier returns 0 for project pins, 1 for ★-favorited, 2 otherwise. Lower
// values mean more-pinned. Empty / unknown ids fall through to tier 2.
func (p PinSource) Tier(id string) int {
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

// SortByTierAlpha stably reorders ids so pinned items (tier 0/1) bubble
// to the top, alpha-sorted within each pinned tier. Tier-2 (unpinned)
// items keep their input order — typically the catalog's curated
// category-then-name ordering, which is what the user is used to seeing.
//
// When no items are pinned (both ProjectPins and UserStars empty), the
// helper is a no-op: there's nothing to bubble, and re-sorting tier-2 by
// label would clobber the catalog's intentional ordering. Production
// menus only see a non-empty pin set after the user ★s their first item;
// before then the menu is identical to pre-PR behavior.
//
// labelFor maps id → display label; pass a closure over the menu's
// displayLabelByID map. It is never called for tier-2 ids when pin
// sources are empty (the early exit fires first).
func (p PinSource) SortByTierAlpha(ids []string, labelFor func(string) string) {
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
			// sort.SliceStable preserves input order when "less" returns
			// false for both directions of an equal pair.
			return false
		}
		return strings.ToLower(labelFor(ids[i])) < strings.ToLower(labelFor(ids[j]))
	})
}

// SortByTierScore stably reorders ids by (tier, -score, label): pinned and
// ★-favorited ids always sort ABOVE unpinned ones, regardless of fuzzy
// score, and score only orders items within the same tier. This is the
// "favorites always on top of every search" ordering — use it when a search
// query is active and matching favorites must lead the results.
func (p PinSource) SortByTierScore(ids []string, scoreOf func(string) int, labelFor func(string) string) {
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

// StableTierBreaker reorders ids that are already sorted by descending
// score, breaking ties by tier and then label. Use after a fuzzy-score
// sort when you want pinned items to win equal-score ties without
// overriding clear higher-score wins.
//
// Implementation: a single sort.SliceStable with a (-score, tier, label)
// comparator. scoreOf must return a comparable int; ties on score promote
// tier; ties on tier promote alphabetical label.
func (p PinSource) StableTierBreaker(ids []string, scoreOf func(string) int, labelFor func(string) string) {
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

// FavoritesStore is the UI-side abstraction over starred items. The
// concrete persistence backend lives in internal/userprefs/, and is
// wired in once at process startup (cmd/beatmo.go for desktop,
// js_bootstrap_wasm.go for browser) via SetFavoritesStore. Tests
// substitute InMemoryFavoritesStore.
//
// Methods are safe to call from any goroutine. Set is fire-and-forget
// from the caller's perspective; persistence happens off-thread on
// desktop and synchronously on WASM. Get reads the in-memory mirror
// and never blocks on disk / localStorage.
type FavoritesStore interface {
	// Get reports whether key is currently favorited. Unknown keys
	// return false.
	Get(key string) bool
	// Set marks key as favorited (true) or removes the favorite (false).
	// Persistence is best-effort — the in-memory mirror always reflects
	// the call; disk/localStorage may lag by a few ms.
	Set(key string, fav bool)
	// Keys returns the set of favorited keys in deterministic
	// (alphabetical) order. Useful for rendering "favorites" group
	// rows and for assertions in tests.
	Keys() []string
}

// InMemoryFavoritesStore is the test fallback and the default when no
// store has been registered yet (so callers never see a nil
// FavoritesStore). Production code wires a userprefs-backed adapter
// that delegates Get/Set/Keys to an in-memory mirror plus async
// persistence; this struct is the in-memory mirror in isolation.
type InMemoryFavoritesStore struct {
	mu  sync.RWMutex
	set map[string]struct{}
}

// NewInMemoryFavoritesStore returns an empty store. Optional ids
// preload the favorites — handy when constructing a store for a test
// that needs a non-empty initial state.
func NewInMemoryFavoritesStore(ids ...string) *InMemoryFavoritesStore {
	s := &InMemoryFavoritesStore{set: make(map[string]struct{}, len(ids))}
	for _, id := range ids {
		if id == "" {
			continue
		}
		s.set[id] = struct{}{}
	}
	return s
}

func (s *InMemoryFavoritesStore) Get(key string) bool {
	if s == nil || key == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.set[key]
	return ok
}

func (s *InMemoryFavoritesStore) Set(key string, fav bool) {
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

func (s *InMemoryFavoritesStore) Keys() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.set))
	for k := range s.set {
		out = append(out, k)
	}
	sortStrings(out)
	return out
}

// sortStrings is a tiny helper to avoid importing "sort" just for one
// call site — favorites lists are short so a basic insertion sort is
// fine and keeps the file dependency-light.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// favoritesGuard protects the global FavoritesStore so SetFavoritesStore
// and Favorites can be called from any goroutine. The expected pattern
// is one call to SetFavoritesStore at process startup; the lock is
// cheap so the simple design is fine.
var (
	favoritesGuard sync.RWMutex
	favorites      FavoritesStore = NewInMemoryFavoritesStore()
)

// SetFavoritesStore registers the global FavoritesStore. Production
// wiring calls this once at startup with a userprefs-backed adapter;
// tests call it within t.Cleanup-restored scopes.
func SetFavoritesStore(s FavoritesStore) {
	favoritesGuard.Lock()
	defer favoritesGuard.Unlock()
	if s == nil {
		favorites = NewInMemoryFavoritesStore()
		return
	}
	favorites = s
}

// Favorites returns the currently registered FavoritesStore. Never
// returns nil; if no store has been registered, an empty in-memory
// store is returned so callers can use Get/Set/Keys without nil checks.
func Favorites() FavoritesStore {
	favoritesGuard.RLock()
	defer favoritesGuard.RUnlock()
	return favorites
}
