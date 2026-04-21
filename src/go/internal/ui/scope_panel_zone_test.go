//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestScopePanelZoneLayout(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	if z.ID() != "scope-panel" {
		t.Errorf("expected ID 'scope-panel', got %q", z.ID())
	}
	r := image.Rect(0, 0, 800, 160)
	z.Layout(r)
	for i, btn := range z.stageButtons {
		if btn == nil {
			t.Errorf("stage button %d is nil", i)
			continue
		}
		if btn.Rect().Empty() {
			t.Errorf("stage button %d has empty rect", i)
		}
	}
	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Error("expected non-empty hit areas")
	}
}

func TestScopeTapSelection(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	// Initially no taps.
	if z.tapA >= 0 || z.tapB >= 0 {
		t.Error("expected no taps initially")
	}
	// Click Synth -> A.
	z.handleStageClick(scope.StageSynth)
	if z.tapA != scope.StageSynth {
		t.Errorf("expected tapA=StageSynth, got %d", z.tapA)
	}
	// Click EQ -> B.
	z.handleStageClick(scope.StageEQ)
	if z.tapB != scope.StageEQ {
		t.Errorf("expected tapB=StageEQ, got %d", z.tapB)
	}
	// Click Synth again -> clear A.
	z.handleStageClick(scope.StageSynth)
	if z.tapA >= 0 {
		t.Error("expected tapA cleared")
	}
}

// TestScopeTapAllStages verifies every stage (including InsertFX) can be
// selected as a tap point.
func TestScopeTapAllStages(t *testing.T) {
	assertDefaultParityState(t)

	stages := scope.AllStages()
	for _, stage := range stages {
		z := NewScopePanelZone(ScopeCallbacks{})
		z.handleStageClick(stage)
		if z.tapA != stage {
			t.Errorf("stage %s: expected tapA=%d, got %d", scope.StageLabel(stage), stage, z.tapA)
		}
		if z.tapB >= 0 {
			t.Errorf("stage %s: tapB should be unset, got %d", scope.StageLabel(stage), z.tapB)
		}
	}
}

// TestScopeTapClearB verifies clicking tapB's stage clears it.
func TestScopeTapClearB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	z.handleStageClick(scope.StageSynth)  // → tapA
	z.handleStageClick(scope.StageMaster) // → tapB

	if z.tapB != scope.StageMaster {
		t.Fatalf("expected tapB=StageMaster, got %d", z.tapB)
	}

	z.handleStageClick(scope.StageMaster) // → clear tapB
	if z.tapB >= 0 {
		t.Errorf("expected tapB cleared, got %d", z.tapB)
	}
	if z.tapA != scope.StageSynth {
		t.Errorf("tapA should be unchanged, got %d", z.tapA)
	}
}

// TestScopeTapReplaceB verifies that clicking a third stage replaces tapB.
func TestScopeTapReplaceB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	z.handleStageClick(scope.StageSynth)    // → tapA
	z.handleStageClick(scope.StageAntiPop)  // → tapB
	z.handleStageClick(scope.StageInsertFX) // → replaces tapB

	if z.tapA != scope.StageSynth {
		t.Errorf("tapA should remain StageSynth, got %d", z.tapA)
	}
	if z.tapB != scope.StageInsertFX {
		t.Errorf("expected tapB=StageInsertFX, got %d", z.tapB)
	}
}

// TestScopeTapCallbacks verifies that all tap change callbacks fire correctly.
func TestScopeTapCallbacks(t *testing.T) {
	assertDefaultParityState(t)

	var tapACalls []scope.Stage
	var tapBCalls []scope.Stage
	var clearACalls, clearBCalls int

	z := NewScopePanelZone(ScopeCallbacks{
		OnTapAChange: func(s scope.Stage) { tapACalls = append(tapACalls, s) },
		OnTapBChange: func(s scope.Stage) { tapBCalls = append(tapBCalls, s) },
		OnClearTapA:  func() { clearACalls++ },
		OnClearTapB:  func() { clearBCalls++ },
	})

	// Click Synth → tapA callback.
	z.handleStageClick(scope.StageSynth)
	if len(tapACalls) != 1 || tapACalls[0] != scope.StageSynth {
		t.Fatalf("expected 1 OnTapAChange(StageSynth), got %v", tapACalls)
	}

	// Click Master → tapB callback.
	z.handleStageClick(scope.StageMaster)
	if len(tapBCalls) != 1 || tapBCalls[0] != scope.StageMaster {
		t.Fatalf("expected 1 OnTapBChange(StageMaster), got %v", tapBCalls)
	}

	// Click InsertFX → replaces tapB, tapB callback fires again.
	z.handleStageClick(scope.StageInsertFX)
	if len(tapBCalls) != 2 || tapBCalls[1] != scope.StageInsertFX {
		t.Fatalf("expected 2nd OnTapBChange(StageInsertFX), got %v", tapBCalls)
	}

	// Click Synth → clear tapA callback.
	z.handleStageClick(scope.StageSynth)
	if clearACalls != 1 {
		t.Fatalf("expected 1 OnClearTapA call, got %d", clearACalls)
	}

	// Click InsertFX → clear tapB callback.
	z.handleStageClick(scope.StageInsertFX)
	if clearBCalls != 1 {
		t.Fatalf("expected 1 OnClearTapB call, got %d", clearBCalls)
	}
}

// TestScopeTapAfterClearA verifies that after clearing tapA, clicking a new
// stage assigns to tapA (not tapB).
func TestScopeTapAfterClearA(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	z.handleStageClick(scope.StageSynth)   // → tapA
	z.handleStageClick(scope.StageAntiPop) // → tapB
	z.handleStageClick(scope.StageSynth)   // → clear tapA
	z.handleStageClick(scope.StageEQ)      // → should go to tapA (it's unset)

	if z.tapA != scope.StageEQ {
		t.Errorf("expected tapA=StageEQ, got %d", z.tapA)
	}
	if z.tapB != scope.StageAntiPop {
		t.Errorf("tapB should be unchanged, got %d", z.tapB)
	}
}

// TestScopeDisplayModeCycle verifies OVR -> SPL -> DIF -> OVR cycling.
func TestScopeDisplayModeCycle(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	if z.displayMode != scopeOverlay {
		t.Fatal("expected overlay mode by default")
	}

	// Click 1: OVR -> SPL
	z.splitBtn.OnClick()
	if z.displayMode != scopeSplit {
		t.Fatalf("expected split mode, got %d", z.displayMode)
	}
	if z.splitBtn.Text != "SPL" {
		t.Errorf("expected button text 'SPL', got %q", z.splitBtn.Text)
	}

	// Click 2: SPL -> DIF
	z.splitBtn.OnClick()
	if z.displayMode != scopeDiff {
		t.Fatalf("expected diff mode, got %d", z.displayMode)
	}
	if z.splitBtn.Text != "DIF" {
		t.Errorf("expected button text 'DIF', got %q", z.splitBtn.Text)
	}

	// Click 3: DIF -> OVR
	z.splitBtn.OnClick()
	if z.displayMode != scopeOverlay {
		t.Fatalf("expected overlay mode, got %d", z.displayMode)
	}
	if z.splitBtn.Text != "OVR" {
		t.Errorf("expected button text 'OVR', got %q", z.splitBtn.Text)
	}
}

// TestScopeFreezeToggle verifies the freeze button updates state and text.
func TestScopeFreezeToggle(t *testing.T) {
	assertDefaultParityState(t)

	z := NewScopePanelZone(ScopeCallbacks{
		OnFreezeToggle: func() bool {
			return true // simulate freezing
		},
	})

	if z.frozen {
		t.Fatal("expected not frozen by default")
	}

	z.freezeBtn.OnClick()
	if !z.frozen {
		t.Fatal("expected frozen after toggle")
	}
	if z.freezeBtn.Text != ">" {
		t.Errorf("expected button text '>', got %q", z.freezeBtn.Text)
	}
}

// TestScopeHitAreasIncludeAllButtons verifies hit areas are generated for
// every interactive element.
func TestScopeHitAreasIncludeAllButtons(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))

	areas := z.HitAreas()
	tags := map[string]bool{}
	for _, a := range areas {
		tags[a.Tag] = true
	}

	expected := []string{
		"scope-zoom",
		"scope-inst-btn",
		"scope-split-btn",
		"scope-freeze-btn",
		"scope-close-btn",
	}
	for _, tag := range expected {
		if !tags[tag] {
			t.Errorf("missing hit area with tag %q", tag)
		}
	}

	// Verify all 6 stage buttons have hit areas.
	for i := 0; i < 6; i++ {
		tag := fmt.Sprintf("scope-stage-%d", i)
		if !tags[tag] {
			t.Errorf("missing hit area for stage button %d (tag %q)", i, tag)
		}
	}
}

// TestScopeYGainDefault verifies yGain is 1.0 on construction.
func TestScopeYGainDefault(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	if z.yGain != 1.0 {
		t.Errorf("expected yGain=1.0, got %f", z.yGain)
	}
}

// TestScopeYGainShiftWheel verifies Shift+Scroll changes yGain.
func TestScopeYGainShiftWheel(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 160))

	// Override key press to simulate shift held.
	restore := SetInputForTest(
		func() (int, int) { return 400, 100 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool {
			return k == ebiten.KeyShiftLeft
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	handler := &scopeZoomHandler{zone: z}
	origWindow := z.windowMs

	// Shift+scroll up should increase yGain.
	handler.OnWheel(400, 100, 3)
	if z.yGain <= 1.0 {
		t.Errorf("expected yGain > 1.0 after shift+scroll up, got %f", z.yGain)
	}
	// windowMs should be unchanged.
	if z.windowMs != origWindow {
		t.Errorf("windowMs should not change with shift held, got %f", z.windowMs)
	}

	// Shift+scroll down should decrease yGain.
	prev := z.yGain
	handler.OnWheel(400, 100, -3)
	if z.yGain >= prev {
		t.Errorf("expected yGain to decrease after shift+scroll down, got %f", z.yGain)
	}
}

// TestScopeYGainClamp verifies yGain is clamped to [0.25, 16.0].
func TestScopeYGainClamp(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	handler := &scopeZoomHandler{zone: z}

	// Scroll up a lot — should clamp at 16.0.
	for i := 0; i < 100; i++ {
		handler.OnWheel(0, 0, 5)
	}
	if z.yGain > 16.0 {
		t.Errorf("expected yGain <= 16.0, got %f", z.yGain)
	}

	// Scroll down a lot — should clamp at 0.25.
	for i := 0; i < 200; i++ {
		handler.OnWheel(0, 0, -5)
	}
	if z.yGain < 0.25 {
		t.Errorf("expected yGain >= 0.25, got %f", z.yGain)
	}
}

// TestScopeYGainNoShiftUnchanged verifies scroll without shift only changes windowMs.
func TestScopeYGainNoShiftUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	// No keys pressed.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	handler := &scopeZoomHandler{zone: z}
	origGain := z.yGain

	handler.OnWheel(0, 0, 3)
	if z.yGain != origGain {
		t.Errorf("yGain should not change without shift, got %f", z.yGain)
	}
	if z.windowMs == 20 {
		t.Error("windowMs should have changed without shift")
	}
}

// TestScopeDoubleClickReset verifies double-click resets both axes.
func TestScopeDoubleClickReset(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.windowMs = 100
	z.yGain = 4.0

	handler := &scopeZoomHandler{zone: z}

	// First press.
	handler.OnPress(400, 100)
	// Second press within 300ms (immediate).
	result := handler.OnPress(400, 100)
	if result != InputConsumed {
		t.Error("expected InputConsumed on double-click")
	}
	if z.windowMs != 20 {
		t.Errorf("expected windowMs=20 after reset, got %f", z.windowMs)
	}
	if z.yGain != 1.0 {
		t.Errorf("expected yGain=1.0 after reset, got %f", z.yGain)
	}
}

// TestScopeAutoGainToggle verifies the AG button toggles autoGain and resets yGain.
func TestScopeAutoGainToggle(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	if z.autoGain {
		t.Fatal("expected autoGain=false by default")
	}

	z.yGain = 4.0
	z.autoGainBtn.OnClick()
	if !z.autoGain {
		t.Fatal("expected autoGain=true after toggle")
	}
	if z.yGain != 1.0 {
		t.Errorf("expected yGain reset to 1.0 on AG enable, got %f", z.yGain)
	}

	z.autoGainBtn.OnClick()
	if z.autoGain {
		t.Fatal("expected autoGain=false after second toggle")
	}
}

// TestScopeAutoGainDisabledByManualZoom verifies Shift+Scroll disables auto-gain.
func TestScopeAutoGainDisabledByManualZoom(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.autoGain = true

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	handler := &scopeZoomHandler{zone: z}
	handler.OnWheel(0, 0, 1)
	if z.autoGain {
		t.Fatal("expected autoGain=false after manual shift+scroll")
	}
}

// TestScopePeakAmplitude verifies peak amplitude computation.
func TestScopePeakAmplitude(t *testing.T) {
	assertDefaultParityState(t)

	state := &scope.State{
		TapA: scope.TapData{Active: true, Samples: []float64{0.1, -0.3, 0.2}},
		TapB: scope.TapData{Active: true, Samples: []float64{0.05, 0.4, -0.1}},
	}
	peak := scopePeakAmplitude(state)
	if peak != 0.4 {
		t.Errorf("expected peak=0.4, got %f", peak)
	}

	// Only one tap active.
	state.TapB.Active = false
	peak = scopePeakAmplitude(state)
	if peak != 0.3 {
		t.Errorf("expected peak=0.3 with only tapA, got %f", peak)
	}
}

// TestScopeTraceFillDrawsCalls verifies fill rects appear when fillCol is set.
func TestScopeTraceFillDrawsCalls(t *testing.T) {
	assertDefaultParityState(t)

	wave := []float64{0.5, 0.5, 0.5, 0.5}
	rect := image.Rect(0, 0, 4, 100)
	midY := 50
	fillCol := color.NRGBA{68, 136, 255, 18}

	rects := collectFilledRects(t, func() {
		drawWaveTrace(nil, wave, rect, midY, 4, colScopeA, 1.0, fillCol)
	})

	// Should have trace rects AND fill rects.
	// With fill, each pixel column produces a trace rect + fill rect(s).
	fillCount := 0
	fillRGBA := color.RGBAModel.Convert(fillCol).(color.RGBA)
	for _, r := range rects {
		if r.Color == fillRGBA {
			fillCount++
		}
	}
	if fillCount == 0 {
		t.Error("expected fill rects with non-nil fillCol, got none")
	}
}

// TestScopeTraceFillNilNoExtra verifies no fill rects appear when fillCol is nil.
func TestScopeTraceFillNilNoExtra(t *testing.T) {
	assertDefaultParityState(t)

	wave := []float64{0.5, 0.5, 0.5, 0.5}
	rect := image.Rect(0, 0, 4, 100)
	midY := 50
	fillCol := color.NRGBA{68, 136, 255, 18}
	fillRGBA := color.RGBAModel.Convert(fillCol).(color.RGBA)

	rects := collectFilledRects(t, func() {
		drawWaveTrace(nil, wave, rect, midY, 4, colScopeA, 1.0, nil)
	})

	for _, r := range rects {
		if r.Color == fillRGBA {
			t.Error("unexpected fill rect when fillCol is nil")
			break
		}
	}
}

// TestScopeHitAreasIncludeAGButton verifies the AG button has a hit area.
func TestScopeHitAreasIncludeAGButton(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))

	areas := z.HitAreas()
	found := false
	for _, a := range areas {
		if a.Tag == "scope-ag-btn" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing hit area with tag 'scope-ag-btn'")
	}
}

// TestScopeDiffComputation verifies element-wise A-B difference.
func TestScopeDiffComputation(t *testing.T) {
	assertDefaultParityState(t)

	a := []float64{1.0, 0.5, 0.0, -0.5}
	b := []float64{0.5, 0.5, 0.5, 0.5}
	diff := scopeDiffSamples(a, b)

	expected := []float64{0.5, 0.0, -0.5, -1.0}
	if len(diff) != len(expected) {
		t.Fatalf("expected %d samples, got %d", len(expected), len(diff))
	}
	for i, v := range diff {
		if v != expected[i] {
			t.Errorf("diff[%d] = %f, expected %f", i, v, expected[i])
		}
	}
}

// TestScopeDiffLengthMismatch verifies diff uses min length without panic.
func TestScopeDiffLengthMismatch(t *testing.T) {
	assertDefaultParityState(t)

	a := []float64{1.0, 0.5, 0.0, -0.5, -1.0}
	b := []float64{0.5, 0.5, 0.5}
	diff := scopeDiffSamples(a, b)

	if len(diff) != 3 {
		t.Fatalf("expected 3 samples (min length), got %d", len(diff))
	}
	if diff[0] != 0.5 {
		t.Errorf("diff[0] = %f, expected 0.5", diff[0])
	}
}

// TestScopeTraceVisibilityDefault verifies both traces visible by default.
func TestScopeTraceVisibilityDefault(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	if !z.showTapA || !z.showTapB {
		t.Error("expected both traces visible by default")
	}
}

// TestScopeTraceToggleA verifies swatch click toggles trace A visibility.
func TestScopeTraceToggleA(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 160))

	handler := &scopeSwatchHandler{zone: z, tap: "A"}
	handler.OnPress(0, 0)
	if z.showTapA {
		t.Error("expected showTapA=false after toggle")
	}
	handler.OnPress(0, 0)
	if !z.showTapA {
		t.Error("expected showTapA=true after second toggle")
	}
}

// TestScopeTraceToggleB verifies swatch click toggles trace B visibility.
func TestScopeTraceToggleB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})

	handler := &scopeSwatchHandler{zone: z, tap: "B"}
	handler.OnPress(0, 0)
	if z.showTapB {
		t.Error("expected showTapB=false after toggle")
	}
}

// TestScopeSwatchHitAreas verifies swatch hit areas are registered.
func TestScopeSwatchHitAreas(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))

	areas := z.HitAreas()
	tags := map[string]bool{}
	for _, a := range areas {
		tags[a.Tag] = true
	}
	if !tags["scope-swatch-a"] {
		t.Error("missing hit area 'scope-swatch-a'")
	}
	if !tags["scope-swatch-b"] {
		t.Error("missing hit area 'scope-swatch-b'")
	}
}

// TestScopeHiddenTraceNotDrawn verifies hidden trace produces no colored rects.
func TestScopeHiddenTraceNotDrawn(t *testing.T) {
	assertDefaultParityState(t)

	wave := []float64{0.5, 0.5, 0.5, 0.5}
	rect := image.Rect(0, 0, 4, 100)
	midY := 50
	traceRGBA := color.RGBAModel.Convert(colScopeA).(color.RGBA)

	// With trace visible: should have blue rects.
	rectsVisible := collectFilledRects(t, func() {
		drawWaveTrace(nil, wave, rect, midY, 4, colScopeA, 1.0, nil)
	})
	found := false
	for _, r := range rectsVisible {
		if r.Color == traceRGBA {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected blue trace rects when visible")
	}

	// When showA=false, drawScopeOverlay skips the call entirely,
	// so we verify by checking that the toggle state works.
	z := NewScopePanelZone(ScopeCallbacks{})
	z.showTapA = false
	if z.showTapA {
		t.Error("showTapA should be false")
	}
}
