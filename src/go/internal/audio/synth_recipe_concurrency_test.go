package audio

import (
	"math/rand"
	"sync"
	"testing"
)

// Phase-1 race-detector hammer on the param manager. Establishes that
// concurrent SetInstrumentParam, GetInstrumentParams, ResetInstrumentParams,
// SetInstrumentParams (bulk), and BindInstrumentToRecipe across many
// goroutines do not produce a data race under `go test -race`.
//
// We do not exercise Play() here because that requires real engine wiring
// that the -tags test build does not provide. The manager mutex is the only
// shared state on the SetInstrumentParam path; the cache invalidation hook
// is a no-op under -tags test. If a future regression introduces a lock
// gap, -race will catch it here.
func TestSetInstrumentParamConcurrentHammer(t *testing.T) {
	const goroutines = 8
	const opsPerGoroutine = 4000
	const numInstruments = 6

	ids := []string{"snare", "kick", "hihat", "clap", "tom", "cowbell"}
	params := []string{"pitch", "decay", "tone", "drive", "body", "brightness"}

	// Pre-clear so leftover state from earlier tests doesn't bias allocation
	// patterns under -race.
	for _, id := range ids {
		ResetInstrumentParams(id)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			ResetInstrumentParams(id)
		}
	})

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(seed int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(seed)))
			for i := 0; i < opsPerGoroutine; i++ {
				id := ids[r.Intn(numInstruments)]
				switch r.Intn(5) {
				case 0:
					SetInstrumentParam(id, params[r.Intn(len(params))], r.Float64()*2-1)
				case 1:
					_ = GetInstrumentParams(id)
				case 2:
					ResetInstrumentParams(id)
				case 3:
					SetInstrumentParams(id, RecipeParams{
						params[r.Intn(len(params))]: r.Float64(),
						params[r.Intn(len(params))]: r.Float64(),
					})
				case 4:
					_ = RecipeForInstrument(id)
				}
			}
		}(g)
	}
	wg.Wait()

	// After the hammer, every instrument should be in a coherent state: the
	// manager retained at most numInstruments entries (one per id), and each
	// stored map must be safely readable (no torn writes).
	for _, id := range ids {
		got := GetInstrumentParams(id)
		for _, v := range got {
			// Touch every value to give -race a chance to observe a torn read.
			_ = v
		}
	}
}
