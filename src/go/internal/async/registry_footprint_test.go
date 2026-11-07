package async

import (
	"context"
	"testing"
)

// Every pool production registers over a normal session, with its worker count. The floor is a correctness
// constraint: below this, the callers without a fallback break. A new standing pool = a row here AND a bump of
// minDefaultBudget.
var standingFootprint = []struct {
	name    string
	workers int
	note    string
}{
	{"hooks.fanout", 2, "internal/hooks/bus.go newRegistryBus"},
	{"eventlogger.format", 1, "internal/eventlogger/logger.go"},
	{"eventstream.persist", 1, "internal/eventstream/sink.go (BEATMO_EVENT_LOG)"},
	{"audio.timers", 1, "internal/ui/game_new.go"},
	{"audio.prewarm", 4, "internal/ui/voice_prewarmer.go (RuntimeProf().PrewarmWorkers, desktop)"},
	{"synth.preview", 1, "internal/ui/synth_graph_pipeline.go"},
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

// The floor constant, not a live budget: on a big host NumCPU-4 would mask a too-small floor that js/wasm and any
// <= 10-core host still get.
func TestDefaultBudgetFloorFitsStandingFootprint(t *testing.T) {
	if want := footprintTotal(); minDefaultBudget < want {
		t.Errorf("minDefaultBudget = %d, want >= %d (standing footprint)", minDefaultBudget, want)
		for _, p := range standingFootprint {
			t.Logf("  %-22s %d  (%s)", p.name, p.workers, p.note)
		}
	}
}

// Every standing pool, registered in order against the default budget, must fit.
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
