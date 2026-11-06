package songmatch

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TunePassages runs the per-instrument matcher on each manifest tuning passage,
// returning a "seed" TuningHint per passage (sorted by instrument id). Never patches.
func TunePassages(refWav wave.Wave, manifest fingerprint.TimeManifest, budget, restarts int) ([]TuningHint, error) {
	passes := append([]fingerprint.TuningPassage(nil), manifest.TuningPassages...)
	sort.Slice(passes, func(i, j int) bool { return passes[i].Instrument < passes[j].Instrument })
	var hints []TuningHint
	for _, p := range passes {
		passage := refWav.Slice(p.StartSec*1000, p.EndSec*1000)
		res, err := synthmatch.Match(p.Instrument, passage, budget, restarts)
		if err != nil {
			return nil, err
		}
		hints = append(hints, TuningHint{
			Instrument: p.Instrument, Kind: "seed",
			Suggestion: "re-tune " + string(p.Instrument) + "'s document toward the solo passage (review git diff + ear)",
			SeedDeltas: res.Params,
		})
	}
	return hints, nil
}
