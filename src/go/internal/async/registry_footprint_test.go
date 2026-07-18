package async

import (
	"context"
	"testing"
)

// standingFootprint mirrors the "Standing pools" table in CLAUDE.md: every
// pool that production code registers against DefaultRegistry over a normal
// session, with the worker count it asks for at creation.
//
// The registry's default budget floor MUST cover this whole set. A floor that
// is too small does NOT save any resources — every caller with a fallback just
// allocates the same workers in a *private* pool outside the budget — but it
// does break the callers that have no fallback (ui.dialog silently fails the
// WAV upload; recording.lifecycle's MustGet panics the whole app). So the floor
// is a correctness constraint, not a tuning knob.
//
// Adding a standing pool means adding a row here AND bumping minDefaultBudget.
var standingFootprint = []struct {
	name    string
	workers int
	note    string
}{
	{"hooks.fanout", 2, "internal/hooks/bus.go newRegistryBus"},
	{"eventlogger.format", 1, "internal/eventlogger/logger.go"},
	{"eventstream.persist", 1, "internal/eventstream/sink.go (BEATMO_EVENT_LOG)"},
	{"audio.timers", 1, "internal/ui/game_new.go"},
	{"audio.prewarm", 2, "internal/ui/voice_prewarmer.go"},
	{"synth.preview", 1, "internal/ui/synth_mirror.go"},
	{"recording.lifecycle", 1, "internal/audio/recording_lifecycle_pool.go (MustGet — panics!)"},
	{"ui.dialog", 1, "internal/ui/drumview_ctor.go via async.Go (no fallback!)"},
	{"userprefs.persist", 1, "internal/userprefs/store_notjs.go (desktop only)"},
}

func footprintTotal() int {
	total := 0
	for _, p := range standingFootprint {
		total += p.workers
	}
	return total
}

// TestDefaultBudgetFloorFitsStandingFootprint pins the floor to the real
// standing footprint. It asserts the *floor constant* rather than a live
// NewRegistry budget on purpose: the default is max(minDefaultBudget,
// NumCPU-4), so on a big host NumCPU-4 would mask a too-small floor while
// js/wasm (NumCPU==1) and any host with <=10 cores still get the floor.
func TestDefaultBudgetFloorFitsStandingFootprint(t *testing.T) {
	if want := footprintTotal(); minDefaultBudget < want {
		t.Errorf("minDefaultBudget = %d, want >= %d (standing footprint)", minDefaultBudget, want)
		for _, p := range standingFootprint {
			t.Logf("  %-22s %d  (%s)", p.name, p.workers, p.note)
		}
	}
}

// TestDefaultRegistryFitsEveryStandingPool is the end-to-end version: build a
// registry with the default budget and actually register every standing pool
// in order. Every one must fit — no ErrBudgetExhausted, which is what breaks
// the WAV upload and panics recording on js/wasm and on any <=10-core host.
func TestDefaultRegistryFitsEveryStandingPool(t *testing.T) {
	r := NewRegistry(context.Background(), 0) // 0 → default (floor or NumCPU-4)
	t.Cleanup(func() {
		for _, p := range standingFootprint {
			r.Release(p.name)
		}
	})
	for _, p := range standingFootprint {
		if _, err := r.Get(p.name, Options{MaxConcurrent: p.workers, QueueSize: 8}); err != nil {
			bs := r.BudgetStats()
			t.Fatalf("Get(%q, workers=%d) = %v; budget=%d used=%d pools=%d — %s cannot be created",
				p.name, p.workers, err, bs.Budget, bs.Used, bs.Pools, p.name)
		}
	}
}
