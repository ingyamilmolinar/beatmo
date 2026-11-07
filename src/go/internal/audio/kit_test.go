package audio

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type stubKitBinder struct {
	mu    sync.Mutex
	insts []instrument.ID
	roles []instrument.Role
	swaps map[int]instrument.ID // row → newInst
}

// Role "" = derive it from the instrument.
type stubRow struct {
	inst instrument.ID
	role instrument.Role
}

func newStubKitBinder(rows ...stubRow) *stubKitBinder {
	s := &stubKitBinder{swaps: map[int]instrument.ID{}}
	for _, r := range rows {
		s.insts = append(s.insts, r.inst)
		s.roles = append(s.roles, r.role)
	}
	return s
}

func (s *stubKitBinder) RowCount() int                       { return len(s.insts) }
func (s *stubKitBinder) RowInstrument(idx int) instrument.ID { return s.insts[idx] }
func (s *stubKitBinder) RowRole(idx int) instrument.Role     { return s.roles[idx] }
func (s *stubKitBinder) SetRowInstrument(idx int, id instrument.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insts[idx] = id
	s.swaps[idx] = id
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

func TestApplyKit_RebindsByExplicitRole(t *testing.T) {
	events := captureKitEvents(t)
	binder := newStubKitBinder(
		stubRow{roster.Snare, instrument.RoleSnare},
		stubRow{roster.Kick, instrument.RoleKick},
		stubRow{roster.Clap, instrument.RoleNone}, // empty role → heuristic falls back to "clap"
	)
	kit := Kit{
		ID:          "kit.b",
		DisplayName: "Kit B",
		Members: map[instrument.Role]instrument.ID{
			instrument.RoleSnare: roster.SnareLofi,
			instrument.RoleKick:  roster.KickDeep,
			instrument.RoleHat:   roster.HihatPedal, // no row carries the "hat" role
		},
	}
	got := ApplyKit(kit, binder)
	if got != 2 {
		t.Errorf("rebound count = %d, want 2 (snare+kick rebind; clap row resolves to role 'clap' which is not in the kit)", got)
	}
	if binder.insts[0] != roster.SnareLofi {
		t.Errorf("row 0 inst = %q want %q", binder.insts[0], roster.SnareLofi)
	}
	if binder.insts[1] != roster.KickDeep {
		t.Errorf("row 1 inst = %q want %q", binder.insts[1], roster.KickDeep)
	}
	if binder.insts[2] != roster.Clap {
		t.Errorf("row 2 inst = %q want %q (role 'clap' is not in kit Members, untouched)", binder.insts[2], roster.Clap)
	}

	e := waitForKitApplied(t, events)
	p, ok := e.Payload.(hooks.KitPayload)
	if !ok || p.KitID != "kit.b" {
		t.Errorf("payload = %+v want KitID=kit.b", e.Payload)
	}
}

func TestApplyKit_HeuristicWhenRoleEmpty(t *testing.T) {
	binder := newStubKitBinder(
		stubRow{roster.SnareLofi, instrument.RoleNone},
		stubRow{roster.KickDeep, instrument.RoleNone},
		stubRow{"user.open-hihat.a1b2c3", instrument.RoleNone},
		stubRow{"user.tom-low.a1b2c3", instrument.RoleNone},
	)
	kit := Kit{
		ID: "kit.electronic",
		Members: map[instrument.Role]instrument.ID{
			instrument.RoleSnare: roster.Rimshot,
			instrument.RoleKick:  roster.KickTight,
			instrument.RoleHat:   roster.Shaker,
			instrument.RoleTom:   roster.TomHighDrive,
		},
	}
	rebound := ApplyKit(kit, binder)
	if rebound != 4 {
		t.Fatalf("rebound = %d want 4", rebound)
	}
	want := []instrument.ID{roster.Rimshot, roster.KickTight, roster.Shaker, roster.TomHighDrive}
	for i, w := range want {
		if binder.insts[i] != w {
			t.Errorf("row %d = %q want %q", i, binder.insts[i], w)
		}
	}
}

// A no-op swap must not fire onRowInstrumentChanged or the hook.
func TestApplyKit_NoOpWhenMemberMatchesCurrent(t *testing.T) {
	binder := newStubKitBinder(stubRow{roster.Snare, instrument.RoleSnare}, stubRow{roster.Kick, instrument.RoleKick})
	kit := Kit{
		ID:      "kit.same",
		Members: map[instrument.Role]instrument.ID{instrument.RoleSnare: roster.Snare, instrument.RoleKick: roster.KickDeep},
	}
	if got := ApplyKit(kit, binder); got != 1 {
		t.Errorf("rebound = %d want 1 (only kick changes; snare is no-op)", got)
	}
	if _, swapped := binder.swaps[0]; swapped {
		t.Errorf("snare row was rebound to the same id (spurious swap)")
	}
}

func TestApplyKit_EmptyMembersAndNilBinder(t *testing.T) {
	if got := ApplyKit(Kit{}, newStubKitBinder(stubRow{roster.Snare, instrument.RoleSnare})); got != 0 {
		t.Errorf("empty Members rebound %d rows", got)
	}
	if got := ApplyKit(Kit{ID: "x", Members: map[instrument.Role]instrument.ID{instrument.RoleSnare: roster.SnareLofi}}, nil); got != 0 {
		t.Errorf("nil binder rebound %d rows", got)
	}
}

// Replacing an existing id keeps its position.
func TestKitRegistry_RoundTripsAndOrders(t *testing.T) {
	t.Cleanup(ResetKitsForTest)
	ResetKitsForTest()

	a := Kit{ID: "a", DisplayName: "Kit A", Members: map[instrument.Role]instrument.ID{instrument.RoleSnare: roster.Snare}}
	b := Kit{ID: "b", DisplayName: "Kit B", Members: map[instrument.Role]instrument.ID{instrument.RoleKick: roster.KickDeep}}
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
	RegisterKit(Kit{ID: "a", DisplayName: "Kit A v2", Members: map[instrument.Role]instrument.ID{instrument.RoleSnare: roster.Rimshot}})
	all = KitsForExport()
	if all[0].DisplayName != "Kit A v2" || all[0].Members[instrument.RoleSnare] != roster.Rimshot {
		t.Errorf("re-register did not update: %+v", all[0])
	}

	UnregisterKit("a")
	all = KitsForExport()
	if len(all) != 1 || all[0].ID != "b" {
		t.Errorf("after Unregister: %+v", all)
	}
}

// A shipped instrument reads builtinProperties; these are the rules for names the user gave.
func TestRoleFromName_Patterns(t *testing.T) {
	cases := []struct {
		name string
		role instrument.Role
	}{
		{"My Kick", instrument.RoleKick}, {"snare_take2", instrument.RoleSnare}, {"rimshot loop", instrument.RoleSnare},
		{"sidestick", instrument.RoleSnare}, {"open-hihat", instrument.RoleHat}, {"Hi-Hat 3", instrument.RoleHat},
		{"floor tom", instrument.RoleTom}, {"clap stack", instrument.RoleClap}, {"cowbell", instrument.RoleCowbell},
		{"ride bell", instrument.RoleRide}, {"crash 18", instrument.RoleCrash}, {"shaker", instrument.RoleShaker},
		{"bass line", instrument.RoleBass}, {"pad", instrument.RoleNone}, {"", instrument.RoleNone},
	}
	for _, c := range cases {
		if got := roleFromName(c.name); got != c.role {
			t.Errorf("roleFromName(%q) = %q want %q", c.name, got, c.role)
		}
	}
}
