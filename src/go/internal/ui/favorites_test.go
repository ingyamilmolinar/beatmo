package ui

import (
	"reflect"
	"testing"
)

func TestInMemoryFavoritesStoreSetGet(t *testing.T) {
	s := NewInMemoryFavoritesStore()
	if s.Get("kick") {
		t.Fatalf("empty store should report false for unknown key")
	}
	s.Set("kick", true)
	if !s.Get("kick") {
		t.Fatalf("after Set(true), Get should report true")
	}
	s.Set("kick", false)
	if s.Get("kick") {
		t.Fatalf("after Set(false), Get should report false")
	}
}

func TestInMemoryFavoritesStorePreloaded(t *testing.T) {
	s := NewInMemoryFavoritesStore("kick", "snare", "")
	if !s.Get("kick") || !s.Get("snare") {
		t.Fatalf("preloaded ids must be set")
	}
	if s.Get("") {
		t.Fatalf("empty key must always be false")
	}
}

func TestInMemoryFavoritesStoreKeysSorted(t *testing.T) {
	s := NewInMemoryFavoritesStore("zeta", "alpha", "mike")
	got := s.Keys()
	want := []string{"alpha", "mike", "zeta"}
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
	if s.Get("kick") {
		t.Fatalf("nil store Get should report false")
	}
	s.Set("kick", true) // must not panic
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

// TestPinSource_Tier covers the four cases (project, user-star, both, none).
// "Both" deliberately resolves to project tier — the architecture lets a
// project pin override a user's ★ when the menu has limited visual real
// estate, and project pins are the more specific signal.
func TestPinSource_Tier(t *testing.T) {
	src := PinSource{
		ProjectPins: map[string]struct{}{"kick-808": {}, "shared-id": {}},
		UserStars:   map[string]struct{}{"snare-tight": {}, "shared-id": {}},
	}
	cases := []struct {
		id   string
		want int
	}{
		{"kick-808", 0},     // project only
		{"shared-id", 0},    // both → project wins
		{"snare-tight", 1},  // user star only
		{"unrelated", 2},    // none
		{"", 2},             // empty id → tier 2 (won't match anything)
	}
	for _, tc := range cases {
		if got := src.Tier(tc.id); got != tc.want {
			t.Errorf("Tier(%q) = %d, want %d", tc.id, got, tc.want)
		}
	}
}

// TestSortByTierAlpha pins the empty-query menu order: project pins → ★ →
// rest, alphabetised within each tier (case-insensitive). Without this, two
// project pins would appear in input order, which is the catalog's
// category+RelPath order — not what the user expects when they pinned
// specific items.
func TestSortByTierAlpha(t *testing.T) {
	src := PinSource{
		ProjectPins: map[string]struct{}{"zeta-pin": {}, "alpha-pin": {}},
		UserStars:   map[string]struct{}{"yankee": {}, "bravo": {}},
	}
	labels := map[string]string{
		"zeta-pin":  "Zeta Pin",
		"alpha-pin": "Alpha Pin",
		"yankee":    "Yankee",
		"bravo":     "Bravo",
		"mike":      "Mike",
		"charlie":   "Charlie",
	}
	labelFor := func(id string) string { return labels[id] }
	ids := []string{"zeta-pin", "yankee", "mike", "charlie", "alpha-pin", "bravo"}
	src.SortByTierAlpha(ids, labelFor)
	// Pinned tiers (0, 1) are alpha-sorted; tier 2 keeps input order so the
	// catalog's curated ordering is preserved for unpinned items.
	want := []string{"alpha-pin", "zeta-pin", "bravo", "yankee", "mike", "charlie"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("SortByTierAlpha[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}

// TestSortByTierAlpha_TrivialInputs verifies the helper is a safe no-op on
// nil and single-element slices (a common sort.Slice edge case).
func TestSortByTierAlpha_TrivialInputs(t *testing.T) {
	src := PinSource{}
	src.SortByTierAlpha(nil, func(string) string { return "" })
	one := []string{"only"}
	src.SortByTierAlpha(one, func(string) string { return "x" })
	if len(one) != 1 || one[0] != "only" {
		t.Errorf("single-element slice mutated: %v", one)
	}
}

// TestStableTierBreaker_FuzzyWinsThenTier pins the contract that fuzzy
// scores are primary: a clearly higher fuzzy score beats a project pin. Only
// within equal-score groups does tier kick in. This is what makes the menu
// useful when a user types a query that strongly favors one item — pinning
// shouldn't drown out the obvious match.
func TestStableTierBreaker_FuzzyWinsThenTier(t *testing.T) {
	src := PinSource{
		ProjectPins: map[string]struct{}{"unrelated-pin": {}},
		UserStars:   map[string]struct{}{},
	}
	scores := map[string]int{
		"strong-match":  100,
		"unrelated-pin": 5,
		"weak-match":    5,
	}
	labels := map[string]string{
		"strong-match":  "Strong Match",
		"unrelated-pin": "Unrelated Pin",
		"weak-match":    "Weak Match",
	}
	scoreOf := func(id string) int { return scores[id] }
	labelFor := func(id string) string { return labels[id] }

	ids := []string{"unrelated-pin", "weak-match", "strong-match"}
	src.StableTierBreaker(ids, scoreOf, labelFor)
	// Strong match wins on score; within the tied score-5 pair the project
	// pin (tier 0) precedes the unranked weak-match (tier 2).
	want := []string{"strong-match", "unrelated-pin", "weak-match"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("StableTierBreaker[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}

// TestStableTierBreaker_AlphaWithinTier pins the third-key sort: within
// equal score AND equal tier, ids are alphabetised by label. Without this,
// a user typing a generic 3-letter query would see arbitrary in-catalog
// ordering for the matched group.
func TestStableTierBreaker_AlphaWithinTier(t *testing.T) {
	src := PinSource{}
	scores := map[string]int{"zeta": 10, "alpha": 10, "mike": 10}
	labels := map[string]string{"zeta": "Zeta", "alpha": "Alpha", "mike": "Mike"}
	ids := []string{"zeta", "mike", "alpha"}
	src.StableTierBreaker(ids,
		func(id string) int { return scores[id] },
		func(id string) string { return labels[id] })
	want := []string{"alpha", "mike", "zeta"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("StableTierBreaker[%d]: got %q want %q (full %v)", i, ids[i], want[i], ids)
		}
	}
}
