//go:build test

package ui

import "testing"

// TestSoakHeapBounded_PlayingSynthTabParamChurn reproduces the production
// WASM OOM stack:
//
//	EQPanelZone.Draw → drawSynthTab → drawSynthSectionCard → Knob.Draw → drawArc
//
// The historical 7-scenario soak fleet draws via drawDrumPane → RowRackZone
// and therefore never enters the synth panel. The user's reported crash
// happened after `row 1 solo on` + `play started` with the Synth tab visible
// and presumably a knob being dragged; this test keeps the tab active for the
// entire play window AND cycles audio.SetInstrumentParam every 5 frames
// (~12 Hz) so the hashRecipeParams keying in NewVoiceWithParams rotates as
// fast as a real slider drag.
//
// Bounds match the moderate scenario — synth-tab drawing + param churn should
// not raise steady-state allocation more than the 256 B/frame head→tail bound
// the rest of the suite enforces. If this test fails, the failure printout
// names whichever per-component counter (voice cache, params manager, analyzer
// bridge reads) crossed its bound, so the next plan can target the exact
// retained structure.
func TestSoakHeapBounded_PlayingSynthTabParamChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:            "playing-synth-tab-param-churn",
		playing:         true,
		synthTabOpen:    true,
		paramChurnEvery: 5,
		editEvery:       600, // mirror the user's "row 1 solo on" cadence
	}, 8400, 300, 4, 1.5, 1.3)
}

// TestSoakHeapBounded_PlayingSynthTabDrawHeavy redraws every frame with the
// Synth tab open. The default soak draws at 1/30; the production crash
// landed inside the Draw path, so we need at least one scenario that pumps
// the synth-tab Draw at full rate to surface a Draw-localized leak (knob
// vertex/index allocation, drawArc path retention, or the EQPanelZone Synth
// tab's analyzer-snapshot ingest) the cadenced scenarios under-sample.
func TestSoakHeapBounded_PlayingSynthTabDrawHeavy(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:         "playing-synth-tab-draw-heavy",
		playing:      true,
		synthTabOpen: true,
		drawEvery:    1,
	}, 2400, 100, 4, 1.5, 1.3)
}
