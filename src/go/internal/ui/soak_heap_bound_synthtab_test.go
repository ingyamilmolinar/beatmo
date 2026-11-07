//go:build test

package ui

import "testing"

// The Synth tab stays visible for the whole play window while a param changes every 5 frames, as fast as a real
// slider drag; the other scenarios never enter the synth panel. A failure names the component counter that
// crossed its bound.
func TestSoakHeapBounded_PlayingSynthTabParamChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:            "playing-synth-tab-param-churn",
		playing:         true,
		synthTabOpen:    true,
		paramChurnEvery: 5,
		editEvery:       600, // mirror the user's "row 1 solo on" cadence
	}, 8400, 300, 4, 1.5, 1.3)
}

// Full-rate Synth-tab Draw: the cadenced scenarios under-sample a Draw-localised leak.
func TestSoakHeapBounded_PlayingSynthTabDrawHeavy(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:         "playing-synth-tab-draw-heavy",
		playing:      true,
		synthTabOpen: true,
		drawEvery:    1,
	}, 2400, 100, 4, 1.5, 1.3)
}
