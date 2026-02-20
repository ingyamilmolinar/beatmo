package ui

import (
	"image"
	"math"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestEQCurveCoordinateRoundtrip(t *testing.T) {
	r := image.Rect(100, 50, 500, 250)
	// Test a few frequencies.
	for _, hz := range []float64{31, 125, 1000, 8000, 16000} {
		x := freqToX(hz, r)
		if x < r.Min.X || x > r.Max.X {
			t.Errorf("freqToX(%.0f) = %d, out of range [%d, %d]", hz, x, r.Min.X, r.Max.X)
		}
	}
	// Test dB roundtrip.
	for _, db := range []float64{-12, -6, 0, 6, 12} {
		y := gainDBToY(db, r)
		roundtrip := yToGainDB(y, r)
		if math.Abs(roundtrip-db) > 1.5 {
			t.Errorf("dB roundtrip: %.1f -> y=%d -> %.1f (expected ~%.1f)", db, y, roundtrip, db)
		}
	}
}

func TestEQCurveGeometricMean(t *testing.T) {
	// Verify ISO band center frequencies use geometric mean.
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()

	bands := dv.buildEQBands(dv.eqBandGainsDB, dv.eqBandMuted)
	expectedCenters := []float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}
	for i, b := range bands {
		expected := expectedCenters[i]
		// Geometric mean: sqrt(lo*hi)
		got := b.Freq
		// Allow 5% tolerance due to integer edge rounding.
		ratio := got / expected
		if ratio < 0.90 || ratio > 1.10 {
			t.Errorf("band %d: expected center ~%.0f Hz, got %.1f Hz", i, expected, got)
		}
	}
}

func TestEQCurveConstantQ(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()

	bands := dv.buildEQBands(dv.eqBandGainsDB, dv.eqBandMuted)
	for i, b := range bands {
		if math.Abs(b.Q-1.414) > 0.001 {
			t.Errorf("band %d: expected Q=1.414, got %.3f", i, b.Q)
		}
	}
}

func TestEQCurveDragUpdatesGain(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()
	dv.eqWaveformMode = false

	// Ensure EQ rect is set up.
	if dv.eqRect.Empty() {
		t.Skip("eqRect not initialized")
	}

	// Find handle position for band 5 (1 kHz).
	center := math.Sqrt(eqBandDefs[5].loHz * eqBandDefs[5].hiHz)
	hx := freqToX(center, dv.eqRect)
	hy := gainDBToY(0, dv.eqRect) // starts at 0 dB

	// Press on the handle.
	consumed := dv.handleEQCurveDrag(hx, hy, true)
	if !consumed {
		t.Fatal("expected curve drag to consume press on band 5 handle")
	}
	if dv.eqCurveDragBand != 5 {
		t.Fatalf("expected drag band 5, got %d", dv.eqCurveDragBand)
	}

	// Drag upward (toward +6 dB).
	targetY := gainDBToY(6, dv.eqRect)
	consumed = dv.handleEQCurveDrag(hx, targetY, true)
	if !consumed {
		t.Fatal("expected drag to consume")
	}

	// Check gain was updated.
	if dv.eqBandGainsDB[5] < 4.0 {
		t.Errorf("expected gain > 4 dB after drag, got %.1f", dv.eqBandGainsDB[5])
	}

	// Release.
	consumed = dv.handleEQCurveDrag(hx, targetY, false)
	if !consumed {
		t.Fatal("expected release to consume")
	}
	if dv.eqCurveDragBand != -1 {
		t.Errorf("expected drag band reset to -1, got %d", dv.eqCurveDragBand)
	}
}

func TestEQCurveXToFreqRoundtrip(t *testing.T) {
	r := image.Rect(100, 50, 500, 250)
	// Round-trip: freq -> X -> freq should be approximately equal.
	for _, hz := range []float64{20, 100, 500, 2000, 10000, 20000} {
		x := freqToX(hz, r)
		got := xToFreq(x, r)
		ratio := got / hz
		if ratio < 0.90 || ratio > 1.10 {
			t.Errorf("xToFreq roundtrip for %.0f Hz: got %.1f Hz (ratio %.2f)", hz, got, ratio)
		}
	}
	// Edge cases: at rect min/max.
	if xToFreq(r.Min.X, r) < 19 || xToFreq(r.Min.X, r) > 21 {
		t.Errorf("expected ~20 Hz at rect left, got %.1f", xToFreq(r.Min.X, r))
	}
	if xToFreq(r.Max.X, r) < 19000 || xToFreq(r.Max.X, r) > 21000 {
		t.Errorf("expected ~20000 Hz at rect right, got %.1f", xToFreq(r.Max.X, r))
	}
}

func TestEQCurveHPFHandleDrag(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()
	dv.eqWaveformMode = false

	if dv.eqRect.Empty() {
		t.Skip("eqRect not initialized")
	}

	// Enable HPF with initial cutoff.
	dv.hpfEnabled = true
	dv.hpfCutoffHz = 200
	dv.eqCurveDirty = true

	// Find the HPF handle position.
	hx := freqToX(200, dv.eqRect)
	hy := dv.curveYAtX(hx, dv.eqRect)

	// Press on the handle.
	consumed := dv.handleEQCurveDrag(hx, hy, true)
	if !consumed {
		t.Fatal("expected HPF handle to consume press")
	}
	if dv.eqCurveDragFilter != "hpf" {
		t.Fatalf("expected eqCurveDragFilter='hpf', got %q", dv.eqCurveDragFilter)
	}

	// Drag rightward (to higher frequency, e.g. 500 Hz).
	targetX := freqToX(500, dv.eqRect)
	consumed = dv.handleEQCurveDrag(targetX, hy, true)
	if !consumed {
		t.Fatal("expected drag to consume")
	}
	if dv.hpfCutoffHz < 400 || dv.hpfCutoffHz > 600 {
		t.Errorf("expected HPF cutoff ~500 Hz after drag, got %.0f", dv.hpfCutoffHz)
	}

	// Release.
	consumed = dv.handleEQCurveDrag(targetX, hy, false)
	if !consumed {
		t.Fatal("expected release to consume")
	}
	if dv.eqCurveDragFilter != "" {
		t.Errorf("expected drag filter reset to empty, got %q", dv.eqCurveDragFilter)
	}
}

func TestEQCurveLPFHandleDrag(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()
	dv.eqWaveformMode = false

	if dv.eqRect.Empty() {
		t.Skip("eqRect not initialized")
	}

	// Enable LPF with initial cutoff.
	dv.lpfEnabled = true
	dv.lpfCutoffHz = 10000
	dv.eqCurveDirty = true

	// Find the LPF handle position.
	lx := freqToX(10000, dv.eqRect)
	ly := dv.curveYAtX(lx, dv.eqRect)

	// Press on the handle.
	consumed := dv.handleEQCurveDrag(lx, ly, true)
	if !consumed {
		t.Fatal("expected LPF handle to consume press")
	}
	if dv.eqCurveDragFilter != "lpf" {
		t.Fatalf("expected eqCurveDragFilter='lpf', got %q", dv.eqCurveDragFilter)
	}

	// Drag leftward (to lower frequency, e.g. 5000 Hz).
	targetX := freqToX(5000, dv.eqRect)
	consumed = dv.handleEQCurveDrag(targetX, ly, true)
	if !consumed {
		t.Fatal("expected drag to consume")
	}
	if dv.lpfCutoffHz < 4000 || dv.lpfCutoffHz > 6000 {
		t.Errorf("expected LPF cutoff ~5000 Hz after drag, got %.0f", dv.lpfCutoffHz)
	}

	// Release.
	consumed = dv.handleEQCurveDrag(targetX, ly, false)
	if !consumed {
		t.Fatal("expected release to consume")
	}
	if dv.eqCurveDragFilter != "" {
		t.Errorf("expected drag filter reset to empty, got %q", dv.eqCurveDragFilter)
	}
}

func TestEQCurveFilterToggle(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()

	// Initially disabled.
	if dv.hpfEnabled || dv.lpfEnabled {
		t.Fatal("expected filters disabled by default")
	}

	// Enable HPF.
	dv.toggleHPF()
	if !dv.hpfEnabled {
		t.Error("expected HPF enabled after toggle")
	}

	// Curve should now include HPF band.
	dv.eqCurveDirty = true
	dv.eqCurveEnsure()
	// Verify the low-frequency end is attenuated (HPF at 20 Hz is a no-op,
	// so set a higher cutoff first).
	dv.hpfCutoffHz = 200
	dv.eqCurveDirty = true
	dv.eqCurveEnsure()
	if len(dv.eqCurveCache) > 0 && dv.eqCurveCache[0].GainDB > -1 {
		t.Errorf("expected low-freq attenuation with HPF at 200Hz, got %.2f dB", dv.eqCurveCache[0].GainDB)
	}

	// Disable HPF.
	dv.toggleHPF()
	if dv.hpfEnabled {
		t.Error("expected HPF disabled after second toggle")
	}

	// Enable LPF.
	dv.toggleLPF()
	if !dv.lpfEnabled {
		t.Error("expected LPF enabled after toggle")
	}
	dv.lpfCutoffHz = 5000
	dv.eqCurveDirty = true
	dv.eqCurveEnsure()
	last := dv.eqCurveCache[len(dv.eqCurveCache)-1]
	if last.GainDB > -1 {
		t.Errorf("expected high-freq attenuation with LPF at 5kHz, got %.2f dB", last.GainDB)
	}

	dv.toggleLPF()
	if dv.lpfEnabled {
		t.Error("expected LPF disabled after second toggle")
	}
}

func TestEQCurveMatchesSliders(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 320), nil, logger)
	dv.calcLayout()

	// Set a non-zero gain via slider.
	if len(dv.eqSliders) < 3 || dv.eqSliders[2] == nil {
		t.Skip("sliders not allocated")
	}
	dv.eqSliders[2].Value = 0.75
	gain := sliderToGainDB(0.75)
	dv.eqBandGainsDB[2] = gain

	// Verify curve cache reflects this gain.
	dv.eqCurveDirty = true
	dv.eqCurveEnsure()
	if len(dv.eqCurveCache) == 0 {
		t.Fatal("curve cache empty after ensure")
	}

	// Find the response at band 2 center frequency.
	center := math.Sqrt(eqBandDefs[2].loHz * eqBandDefs[2].hiHz)
	var closestDB float64
	closestDist := math.MaxFloat64
	for _, p := range dv.eqCurveCache {
		d := math.Abs(math.Log10(p.FreqHz) - math.Log10(center))
		if d < closestDist {
			closestDist = d
			closestDB = p.GainDB
		}
	}

	// The response at the center frequency should be near the set gain.
	if math.Abs(closestDB-gain) > 2 {
		t.Errorf("curve at %.0f Hz: expected ~%.1f dB, got %.1f dB", center, gain, closestDB)
	}
}
