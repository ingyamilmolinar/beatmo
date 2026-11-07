package ui

import (
	"reflect"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInMemoryFavoritesStoreSetGet(t *testing.T) {
	s := NewInMemoryFavoritesStore()
	if s.Get(roster.Kick) {
		t.Fatalf("empty store should report false for unknown key")
	}
	s.Set(roster.Kick, true)
	if !s.Get(roster.Kick) {
		t.Fatalf("after Set(true), Get should report true")
	}
	s.Set(roster.Kick, false)
	if s.Get(roster.Kick) {
		t.Fatalf("after Set(false), Get should report false")
	}
}

func TestInMemoryFavoritesStorePreloaded(t *testing.T) {
	s := NewInMemoryFavoritesStore(roster.Kick, roster.Snare, "")
	if !s.Get(roster.Kick) || !s.Get(roster.Snare) {
		t.Fatalf("preloaded ids must be set")
	}
	if s.Get("") {
		t.Fatalf("empty key must always be false")
	}
}

func TestInMemoryFavoritesStoreKeysSorted(t *testing.T) {
	s := NewInMemoryFavoritesStore("zeta", "alpha", "mike")
	got := s.Keys()
	want := []instrument.ID{"alpha", "mike", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys not sorted: got %v want %v", got, want)
	}
}

func TestInMemoryFavoritesStoreEmptyKeyIgnored(t *testing.T) {
	s := NewInMemoryFavoritesStore()
	s.Set("", true)
	if len(s.Keys()) != 0 {
		t.Fatalf("setting empty key should be a no-op, got %v", s.Keys())
	}
}

func TestInMemoryFavoritesStoreNilSafe(t *testing.T) {
	var s *InMemoryFavoritesStore // nil
	// Should not panic, should report sane defaults.
	if s.Get(roster.Kick) {
		t.Fatalf("nil store Get should report false")
	}
	s.Set(roster.Kick, true) // must not panic
	if keys := s.Keys(); keys != nil && len(keys) != 0 {
		t.Fatalf("nil store Keys should be nil/empty, got %v", keys)
	}
}

func TestSetFavoritesStoreReplacesGlobal(t *testing.T) {
	original := Favorites()
	t.Cleanup(func() { SetFavoritesStore(original) })

	mine := NewInMemoryFavoritesStore("custom-id")
	SetFavoritesStore(mine)
	if got := Favorites(); got != mine {
		t.Fatalf("Favorites() did not return registered store")
	}
	if !Favorites().Get("custom-id") {
		t.Fatalf("Favorites() did not see the preloaded id")
	}
}

func TestSetFavoritesStoreNilFallback(t *testing.T) {
	original := Favorites()
	t.Cleanup(func() { SetFavoritesStore(original) })

	SetFavoritesStore(nil)
	got := Favorites()
	if got == nil {
		t.Fatalf("Favorites() must never return nil")
	}
	// Sanity: the nil-fallback is a fresh empty store.
	if got.Get("anything") {
		t.Fatalf("nil-fallback store should be empty")
	}
}

func TestFavoritesGlobalDefaultIsUsable(t *testing.T) {
	// Without ever calling SetFavoritesStore, Favorites() must work.
	got := Favorites()
	if got == nil {
		t.Fatalf("global default Favorites() returned nil")
	}
	got.Set("temp-test-id", true)
	if !got.Get("temp-test-id") {
		t.Fatalf("default favorites store did not honor Set")
	}
	t.Cleanup(func() { got.Set("temp-test-id", false) })
}

// Both resolves to the project tier, the more specific signal.
func TestPinSource_Tier(t *testing.T) {
	src := PinSource{
		ProjectPins: map[instrument.ID]struct{}{roster.Kick808: {}, "shared-id": {}},
		UserStars:   map[instrument.ID]struct{}{roster.SnareFat: {}, "shared-id": {}},
	}
	cases := []struct {
		id   instrument.ID
		want int
	}{
		{roster.Kick808, 0},  // project only
		{"shared-id", 0},     // both → project wins
		{roster.SnareFat, 1}, // user star only
		{"unrelated", 2},     // none
		{"", 2},              // empty id → tier 2 (won't match anything)
	}
	for _, tc := range cases {
		if got := src.Tier(tc.id); got != tc.want {
			t.Errorf("Tier(%q) = %d, want %d", tc.id, got, tc.want)
		}
	}
}

// Project pins, then ★, then the rest, alphabetised within each pinned tier (case-insensitive).
func TestSortByTierAlpha(t *testing.T) {
	src := PinSource{
		ProjectPins: map[instrument.ID]struct{}{"zeta-pin": {}, "alpha-pin": {}},
		UserStars:   map[instrument.ID]struct{}{"yankee": {}, "bravo": {}},
	}
	labels := map[instrument.ID]string{
		"zeta-pin":  "Zeta Pin",
		"alpha-pin": "Alpha Pin",
		"yankee":    "Yankee",
		"bravo":     "Bravo",
		"mike":      "Mike",
		"charlie":   "Charlie",
	}
	labelFor := func(id instrument.ID) string { return labels[id] }
	ids := []instrument.ID{"zeta-pin", "yankee", "mike", "charlie", "alpha-pin", "bravo"}
	src.SortByTierAlpha(ids, labelFor)
	// Unpinned items keep the catalogue's curated order.
	want := []instrument.ID{"alpha-pin", "zeta-pin", "bravo", "yankee", "mike", "charlie"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("SortByTierAlpha[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}

func TestSortByTierAlpha_TrivialInputs(t *testing.T) {
	src := PinSource{}
	src.SortByTierAlpha(nil, func(instrument.ID) string { return "" })
	one := []instrument.ID{"only"}
	src.SortByTierAlpha(one, func(instrument.ID) string { return "x" })
	if len(one) != 1 || one[0] != "only" {
		t.Errorf("single-element slice mutated: %v", one)
	}
}

// Score first: a clearly better match beats a project pin, so pins never drown the obvious result.
func TestStableTierBreaker_FuzzyWinsThenTier(t *testing.T) {
	src := PinSource{
		ProjectPins: map[instrument.ID]struct{}{"unrelated-pin": {}},
		UserStars:   map[instrument.ID]struct{}{},
	}
	scores := map[instrument.ID]int{
		"strong-match":  100,
		"unrelated-pin": 5,
		"weak-match":    5,
	}
	labels := map[instrument.ID]string{
		"strong-match":  "Strong Match",
		"unrelated-pin": "Unrelated Pin",
		"weak-match":    "Weak Match",
	}
	scoreOf := func(id instrument.ID) int { return scores[id] }
	labelFor := func(id instrument.ID) string { return labels[id] }

	ids := []instrument.ID{"unrelated-pin", "weak-match", "strong-match"}
	src.StableTierBreaker(ids, scoreOf, labelFor)
	// Within the tied pair the pin precedes the weak match.
	want := []instrument.ID{"strong-match", "unrelated-pin", "weak-match"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("StableTierBreaker[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}

// Equal score and tier sort by label, not arbitrary catalogue order.
func TestStableTierBreaker_AlphaWithinTier(t *testing.T) {
	src := PinSource{}
	scores := map[instrument.ID]int{"zeta": 10, "alpha": 10, "mike": 10}
	labels := map[instrument.ID]string{"zeta": "Zeta", "alpha": "Alpha", "mike": "Mike"}
	ids := []instrument.ID{"zeta", "mike", "alpha"}
	src.StableTierBreaker(ids,
		func(id instrument.ID) int { return scores[id] },
		func(id instrument.ID) string { return labels[id] })
	want := []instrument.ID{"alpha", "mike", "zeta"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("StableTierBreaker[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}
