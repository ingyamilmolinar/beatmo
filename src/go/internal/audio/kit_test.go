package audio

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// stubKitBinder is the minimal KitRowBinder for unit testing ApplyKit
// without pulling in DrumView. Captures every SetRowInstrument call for
// assertion.
type stubKitBinder struct {
	mu    sync.Mutex
	insts []string
	roles []string
	swaps map[int]string // row → newInst
}

func newStubKitBinder(pairs ...string) *stubKitBinder {
	// pairs are flat: inst1, role1, inst2, role2, …
	s := &stubKitBinder{swaps: map[int]string{}}
	for i := 0; i < len(pairs); i += 2 {
		s.insts = append(s.insts, pairs[i])
		role := ""
		if i+1 < len(pairs) {
			role = pairs[i+1]
		}
		s.roles = append(s.roles, role)
	}
	return s
}

func (s *stubKitBinder) RowCount() int                { return len(s.insts) }
func (s *stubKitBinder) RowInstrument(idx int) string { return s.insts[idx] }
func (s *stubKitBinder) RowRole(idx int) string       { return s.roles[idx] }
func (s *stubKitBinder) SetRowInstrument(idx int, newID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insts[idx] = newID
	s.swaps[idx] = newID
}

func captureKitEvents(t *testing.T) func() []hooks.Event {
	t.Helper()
	var mu sync.Mutex
	var seen []hooks.Event
	unsub := hooks.Subscribe(hooks.EventKitApplied, func(e hooks.Event) {
		mu.Lock()
		seen = append(seen, e)
		mu.Unlock()
	})
	t.Cleanup(unsub)
	return func() []hooks.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]hooks.Event, len(seen))
		copy(out, seen)
		return out
	}
}

func waitForKitApplied(t *testing.T, events func() []hooks.Event) hooks.Event {
	t.Helper()
	for i := 0; i < 100; i++ {
		es := events()
		if len(es) > 0 {
			return es[len(es)-1]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("EventKitApplied not delivered within budget")
	return hooks.Event{}
}

// TestApplyKit_RebindsByExplicitRole — a row carrying an explicit Role
// gets rebound to the kit's member for that role; rows without a
// matching role are untouched.
func TestApplyKit_RebindsByExplicitRole(t *testing.T) {
	events := captureKitEvents(t)
	binder := newStubKitBinder(
		"snare", "snare",
		"kick", "kick",
		"clap", "", // empty role → heuristic falls back to "clap"
	)
	kit := Kit{
		ID:          "kit.b",
		DisplayName: "Kit B",
		Members: map[string]string{
			"snare": "snare-2",
			"kick":  "kick-deep",
			"hat":   "hihat-pedal", // no row carries the "hat" role
		},
	}
	got := ApplyKit(kit, binder)
	if got != 2 {
		t.Errorf("rebound count = %d, want 2 (snare+kick rebind; clap row resolves to role 'clap' which is not in the kit)", got)
	}
	if binder.insts[0] != "snare-2" {
		t.Errorf("row 0 inst = %q want snare-2", binder.insts[0])
	}
	if binder.insts[1] != "kick-deep" {
		t.Errorf("row 1 inst = %q want kick-deep", binder.insts[1])
	}
	if binder.insts[2] != "clap" {
		t.Errorf("row 2 inst = %q want clap (heuristic role 'clap'; not in kit Members, untouched)", binder.insts[2])
	}

	e := waitForKitApplied(t, events)
	p, ok := e.Payload.(hooks.KitPayload)
	if !ok || p.KitID != "kit.b" {
		t.Errorf("payload = %+v want KitID=kit.b", e.Payload)
	}
}

// TestApplyKit_HeuristicWhenRoleEmpty — rows without an explicit Role
// resolve via roleHeuristic. Covers the shipped name patterns
// (open-hihat / hihat / kick / snare / tom / clap / cowbell / ride /
// crash / shaker / bass).
func TestApplyKit_HeuristicWhenRoleEmpty(t *testing.T) {
	binder := newStubKitBinder(
		"snare-2", "",
		"kick-deep", "",
		"open-hihat", "",
		"tom-low", "",
	)
	kit := Kit{
		ID: "kit.electronic",
		Members: map[string]string{
			"snare": "rimshot",
			"kick":  "kick-tight",
			"hat":   "shaker",
			"tom":   "tom-high",
		},
	}
	rebound := ApplyKit(kit, binder)
	if rebound != 4 {
		t.Fatalf("rebound = %d want 4", rebound)
	}
	want := []string{"rimshot", "kick-tight", "shaker", "tom-high"}
	for i, w := range want {
		if binder.insts[i] != w {
			t.Errorf("row %d = %q want %q", i, binder.insts[i], w)
		}
	}
}

// TestApplyKit_NoOpWhenMemberMatchesCurrent — a kit member that maps to
// the row's existing instrument doesn't count as a rebind (avoids a
// spurious onRowInstrumentChanged + hook for a no-op swap).
func TestApplyKit_NoOpWhenMemberMatchesCurrent(t *testing.T) {
	binder := newStubKitBinder("snare", "snare", "kick", "kick")
	kit := Kit{
		ID:      "kit.same",
		Members: map[string]string{"snare": "snare", "kick": "kick-deep"},
	}
	if got := ApplyKit(kit, binder); got != 1 {
		t.Errorf("rebound = %d want 1 (only kick changes; snare is no-op)", got)
	}
	if _, swapped := binder.swaps[0]; swapped {
		t.Errorf("snare row was rebound to the same id (spurious swap)")
	}
}

// TestApplyKit_EmptyMembersAndNilBinder — defensive: ApplyKit with an
// empty kit or nil binder is a no-op (no panic, no hook).
func TestApplyKit_EmptyMembersAndNilBinder(t *testing.T) {
	if got := ApplyKit(Kit{}, newStubKitBinder("snare", "snare")); got != 0 {
		t.Errorf("empty Members rebound %d rows", got)
	}
	if got := ApplyKit(Kit{ID: "x", Members: map[string]string{"snare": "snare-2"}}, nil); got != 0 {
		t.Errorf("nil binder rebound %d rows", got)
	}
}

// TestKitRegistry_RoundTripsAndOrders — RegisterKit / KitForID /
// KitsForExport / UnregisterKit. Order is registration order; replacing
// an existing id preserves position.
func TestKitRegistry_RoundTripsAndOrders(t *testing.T) {
	t.Cleanup(ResetKitsForTest)
	ResetKitsForTest()

	a := Kit{ID: "a", DisplayName: "Kit A", Members: map[string]string{"snare": "snare"}}
	b := Kit{ID: "b", DisplayName: "Kit B", Members: map[string]string{"kick": "kick-deep"}}
	RegisterKit(a)
	RegisterKit(b)

	got, ok := KitForID("a")
	if !ok || got.DisplayName != "Kit A" {
		t.Errorf("KitForID(a) = %+v ok=%v", got, ok)
	}

	all := KitsForExport()
	if len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Errorf("KitsForExport order: %+v want [a b]", all)
	}

	// Replace a → preserves order.
	RegisterKit(Kit{ID: "a", DisplayName: "Kit A v2", Members: map[string]string{"snare": "rimshot"}})
	all = KitsForExport()
	if all[0].DisplayName != "Kit A v2" || all[0].Members["snare"] != "rimshot" {
		t.Errorf("re-register did not update: %+v", all[0])
	}

	UnregisterKit("a")
	all = KitsForExport()
	if len(all) != 1 || all[0].ID != "b" {
		t.Errorf("after Unregister: %+v", all)
	}
}

// TestKit_InsertChainIsInert documents and pins the Phase-6 deferral
// of kit-bus audio routing. ApplyKit must ignore Kit.InsertChain — the
// field exists to preserve user configuration across save/load cycles
// (and to keep the JSON wire format stable for when kit-bus lands)
// but the audio graph is not extended yet. A regression that wired
// InsertChain into the mixer without the explicit kit-bus implementation
// would break this test.
//
// What we can verify cheaply in a unit test:
//  1. ApplyKit returns normally with a non-empty InsertChain (no panic
//     from a half-written routing patch).
//  2. The InsertChain round-trips through KitsForExport unchanged so
//     pre-emptively authored kits survive the rollout.
//  3. ApplyKit's rebind behaviour is unaffected by the presence of an
//     InsertChain — the rows are still rebound by role.
//
// When kit-bus ships, this test must be updated (or replaced by a kit
// signal-path test) — failing to update it should make the regression
// visible.
func TestKit_InsertChainIsInert(t *testing.T) {
	t.Cleanup(ResetKitsForTest)
	ResetKitsForTest()

	kit := Kit{
		ID:          "kit.with-fx",
		DisplayName: "Kit With FX",
		Members:     map[string]string{"snare": "snare-2", "kick": "kick-deep"},
		InsertChain: []EffectSlot{
			{Type: EffectCompressor, Enabled: true, Params: map[string]float64{"threshold_db": -10}},
			{Type: EffectLimiter, Enabled: true, Params: map[string]float64{"ceiling_db": -1}},
		},
	}
	RegisterKit(kit)

	// (1) ApplyKit returns normally + rebinds rows as if InsertChain
	// were absent.
	binder := newStubKitBinder("snare", "snare", "kick", "kick")
	rebound := ApplyKit(kit, binder)
	if rebound != 2 {
		t.Errorf("rebound = %d, want 2 (InsertChain must not affect row rebind logic)", rebound)
	}
	if binder.insts[0] != "snare-2" || binder.insts[1] != "kick-deep" {
		t.Errorf("rows not rebound: %+v", binder.insts)
	}

	// (2) InsertChain round-trips through the registry unchanged.
	got, ok := KitForID("kit.with-fx")
	if !ok {
		t.Fatalf("KitForID lost the kit")
	}
	if len(got.InsertChain) != 2 || got.InsertChain[0].Type != EffectCompressor || got.InsertChain[1].Type != EffectLimiter {
		t.Errorf("InsertChain not preserved through registry: %+v", got.InsertChain)
	}
	if got.InsertChain[0].Params["threshold_db"] != -10 {
		t.Errorf("InsertChain param lost: %+v", got.InsertChain[0].Params)
	}

	// (3) KitsForExport also returns the chain intact (project save path).
	all := KitsForExport()
	if len(all) != 1 || len(all[0].InsertChain) != 2 {
		t.Errorf("KitsForExport dropped InsertChain: %+v", all)
	}
}

// TestRoleHeuristic_Patterns — every shipped instrument family resolves
// to its intended role; unknown ids return "".
func TestRoleHeuristic_Patterns(t *testing.T) {
	cases := []struct{ id, role string }{
		{"kick", "kick"}, {"kick-deep", "kick"}, {"kick-1", "kick"},
		{"snare", "snare"}, {"snare-2", "snare"}, {"rimshot", "snare"}, {"sidestick", "snare"},
		{"hihat", "hat"}, {"open-hihat", "hat"}, {"hihat-pedal", "hat"},
		{"tom", "tom"}, {"tom-high", "tom"}, {"tom-low", "tom"},
		{"clap", "clap"}, {"clap-tight", "clap"},
		{"cowbell", "cowbell"},
		{"ride", "ride"}, {"crash", "crash"}, {"shaker", "shaker"},
		{"bass-guitar", "bass"}, {"sub-bass", "bass"}, {"fm-bass", "bass"},
		{"unknown-thing", ""}, {"", ""},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := RoleForInstrument(c.id); got != c.role {
				t.Errorf("RoleForInstrument(%q) = %q want %q", c.id, got, c.role)
			}
		})
	}
}
