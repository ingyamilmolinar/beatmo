package clone

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// LoudnessNorm is the crest rule `make measure-loudness` sets render.amplitude
// from: RMS / peak over the loudest 300 ms window (scale-invariant). ok is
// false for an empty or silent signal. cmd/measure-loudness calls this, so a
// clone's amplitude and the roster's are the same measurement.
func LoudnessNorm(samples []float64, rate int) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	seg := fingerprint.AutoSegment(wave.Wave{Samples: samples, SampleRate: rate}, 0.3)
	obs := wave.NewPeakRMSObserver().Observe(seg)
	if obs.Peak <= 1e-9 || obs.RMS <= 1e-12 {
		return 0, false
	}
	return obs.RMS / obs.Peak, true
}
