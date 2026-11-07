package audio

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// For `go test -race`: concurrent setters, getters, resets and document reads across goroutines. The manager mutex is
// the only shared state on this path under -tags test.
func TestSetInstrumentParamConcurrentHammer(t *testing.T) {
	const goroutines = 8
	const opsPerGoroutine = 4000
	const numInstruments = 6

	ids := []instrument.ID{roster.Snare, roster.Kick, roster.Hihat, roster.Clap, roster.Tom, roster.Cowbell}
	params := []string{"pitch", "decay", "tone", "drive", "body", "brightness"}

	// So earlier tests' state does not bias allocation under -race.
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
					_, _ = InstrumentDoc(id)
				}
			}
		}(g)
	}
	wg.Wait()

	// At most one entry per id, each safely readable (no torn writes).
	for _, id := range ids {
		got := GetInstrumentParams(id)
		for _, v := range got {
			// Touch every value to give -race a chance to observe a torn read.
			_ = v
		}
	}
}
