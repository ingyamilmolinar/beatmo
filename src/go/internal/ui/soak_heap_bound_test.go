//go:build test

package ui

import "testing"

// The negative control: an idle-frame allocator regression shows here, independent of playback.
func TestSoakHeapBounded_Idle(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "idle"}, 8400, 300, 4, 1.5, 1.3)
}

// Parity recording and timeline history are the structures that grow with playback.
func TestSoakHeapBounded_PlayingModerate(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "playing-moderate", playing: true}, 8400, 300, 4, 1.5, 1.3)
}

// Solo and mute toggles keep the parity generation and the recording path active, so a fix that prunes only idle
// decisions is still caught.
func TestSoakHeapBounded_PlayingWithEdits(t *testing.T) {
	runSoakHeapBound(t, soakScenario{name: "playing-with-edits", playing: true, editEvery: 600}, 8400, 300, 4, 1.5, 1.3)
}

// The moderate scenarios draw at 1/30 to stay fast, under-sampling a leak local to the per-Draw allocator.
func TestSoakHeapBounded_DrawHeavy(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:      "draw-heavy",
		playing:   true,
		drawEvery: 1,
	}, 2400, 100, 4, 1.5, 1.3)
}

// An insert effect added and removed mid-play: a leak in effect chain → channel rebuild → platform notify shows as a
// slope failure while the moderate scenario stays flat.
func TestSoakHeapBounded_PlayingWithFXChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:         "playing-with-fx-churn",
		playing:      true,
		fxChurnEvery: 200,
	}, 8400, 300, 4, 1.5, 1.3)
}

// A slider drag on a live FX param calls the bridge at frame rate, so a per-call retain compounds quickly; ~12 Hz
// gives enough samples.
func TestSoakHeapBounded_PlayingWithFXParamChurn(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:              "playing-with-fx-param-churn",
		playing:           true,
		fxParamChurnEvery: 5,
	}, 8400, 300, 4, 1.5, 1.3)
}

// Every edit flips predDirty and bumps parityGen, the rebuild and prune paths pure playback skips.
func TestSoakHeapBounded_PlayingWithGraphEdits(t *testing.T) {
	runSoakHeapBound(t, soakScenario{
		name:           "playing-with-graph-edits",
		playing:        true,
		graphEditEvery: 600,
	}, 8400, 300, 4, 1.5, 1.3)
}

// All production-shaped load at once: a leak only this catches lives in the interaction between mutation paths.
// The long mode drives abs far past windowCap into the cold archive.
func TestSoakHeapBounded_PlayingProductionLike(t *testing.T) {
	frames := 50_000
	editEvery := 3000
	fxChurnEvery := 1000
	fxParamChurnEvery := 25
	graphEditEvery := 3000
	if testing.Short() {
		frames = 8400
		editEvery = 600
		fxChurnEvery = 200
		fxParamChurnEvery = 5
		graphEditEvery = 600
	}
	runSoakHeapBound(t, soakScenario{
		name:              "playing-production-like",
		playing:           true,
		editEvery:         editEvery,
		fxChurnEvery:      fxChurnEvery,
		fxParamChurnEvery: fxParamChurnEvery,
		graphEditEvery:    graphEditEvery,
	}, frames, 1500, 4, 1.5, 1.3)
}
