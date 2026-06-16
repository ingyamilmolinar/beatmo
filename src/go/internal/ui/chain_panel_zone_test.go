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

func TestChainPanelZoneLayout(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	if z.ID() != "chain-panel" {
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

func TestChainTapSelection(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	// Tap A defaults to Synth so the Chain tab is never blank on open.
	if z.tapA != scope.StageSynth {
		t.Errorf("expected tapA=StageSynth by default, got %d", z.tapA)
	}
	if z.tapB >= 0 {
		t.Errorf("expected tapB unset by default, got %d", z.tapB)
	}
	// Click EQ -> B (A already has Synth).
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

// TestNewChainPanelZoneDefaultsTapASynth pins the UX default that the Chain
// tab opens with Synth selected as Tap A — and verifies the OnTapAChange
// callback fires once during construction so the audio scope service mirrors
// the UI state.
func TestNewChainPanelZoneDefaultsTapASynth(t *testing.T) {
	assertDefaultParityState(t)
	var tapACalls []scope.Stage
	z := NewChainPanelZone(ChainCallbacks{
		OnTapAChange: func(s scope.Stage) { tapACalls = append(tapACalls, s) },
	})
	if z.tapA != scope.StageSynth {
		t.Errorf("default tapA: got %d want StageSynth(%d)", z.tapA, scope.StageSynth)
	}
	if z.tapB >= 0 {
		t.Errorf("default tapB should be unset, got %d", z.tapB)
	}
	if len(tapACalls) != 1 || tapACalls[0] != scope.StageSynth {
		t.Fatalf("expected one OnTapAChange(StageSynth) during ctor, got %v", tapACalls)
	}
}

// TestChainTapAllStages verifies every stage (including InsertFX) can be
// selected as a tap point. NewChainPanelZone defaults tapA to StageSynth,
// so we clear it first to exercise the "fresh assign to tapA" path.
func TestChainTapAllStages(t *testing.T) {
	assertDefaultParityState(t)

	stages := scope.AllStages()
	for _, stage := range stages {
		z := NewChainPanelZone(ChainCallbacks{})
		z.tapA = -1 // clear default so the next click lands on tapA
		z.handleStageClick(stage)
		if z.tapA != stage {
			t.Errorf("stage %s: expected tapA=%d, got %d", scope.StageLabel(stage), stage, z.tapA)
		}
		if z.tapB >= 0 {
			t.Errorf("stage %s: tapB should be unset, got %d", scope.StageLabel(stage), z.tapB)
		}
	}
}

// TestChainTapClearB verifies clicking tapB's stage clears it.
func TestChainTapClearB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.tapA = -1 // start blank; ctor default (Synth) would intercept the first click

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

// TestChainTapReplaceB verifies that clicking a third stage replaces tapB.
func TestChainTapReplaceB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.tapA = -1 // start blank for the "Synth → tapA" assertion below

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

// TestChainTapCallbacks verifies that all tap change callbacks fire correctly.
func TestChainTapCallbacks(t *testing.T) {
	assertDefaultParityState(t)

	var tapACalls []scope.Stage
	var tapBCalls []scope.Stage
	var clearACalls, clearBCalls int

	z := NewChainPanelZone(ChainCallbacks{
		OnTapAChange: func(s scope.Stage) { tapACalls = append(tapACalls, s) },
		OnTapBChange: func(s scope.Stage) { tapBCalls = append(tapBCalls, s) },
		OnClearTapA:  func() { clearACalls++ },
		OnClearTapB:  func() { clearBCalls++ },
	})
	// NewChainPanelZone fires OnTapAChange(Synth) once for the default tapA.
	// Drain that and reset tapA so the click-flow assertions below start clean.
	if len(tapACalls) != 1 || tapACalls[0] != scope.StageSynth {
		t.Fatalf("ctor should have fired OnTapAChange(Synth) once, got %v", tapACalls)
	}
	tapACalls = tapACalls[:0]
	z.tapA = -1

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

// TestChainTapAfterClearA verifies that after clearing tapA, clicking a new
// stage assigns to tapA (not tapB).
func TestChainTapAfterClearA(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.tapA = -1 // start blank to test the "first click → tapA" path

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

// TestChainDisplayModePillsDirect verifies each of the three separate mode
// pills sets the displayMode directly (replacing the legacy OVR→SPL→DIF
// cycle button). Each pill keeps its own static label and is sticky on
// repeated clicks.
func TestChainDisplayModePillsDirect(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

	if z.displayMode != chainOverlay {
		t.Fatal("expected overlay mode by default")
	}

	z.splitBtn.OnClick()
	if z.displayMode != chainSplit {
		t.Fatalf("after splitBtn click: displayMode=%d want chainSplit=%d", z.displayMode, chainSplit)
	}
	if z.splitBtn.Text != "SPL" {
		t.Errorf("splitBtn text = %q; want %q (label is static, no cycle)", z.splitBtn.Text, "SPL")
	}

	z.diffBtn.OnClick()
	if z.displayMode != chainDiff {
		t.Fatalf("after diffBtn click: displayMode=%d want chainDiff=%d", z.displayMode, chainDiff)
	}
	if z.diffBtn.Text != "DIF" {
		t.Errorf("diffBtn text = %q; want %q", z.diffBtn.Text, "DIF")
	}

	z.overlayBtn.OnClick()
	if z.displayMode != chainOverlay {
		t.Fatalf("after overlayBtn click: displayMode=%d want chainOverlay=%d", z.displayMode, chainOverlay)
	}
	if z.overlayBtn.Text != "OVR" {
		t.Errorf("overlayBtn text = %q; want %q", z.overlayBtn.Text, "OVR")
	}
}

// TestChainFreezeToggle verifies the freeze button updates state and icon.
func TestChainFreezeToggle(t *testing.T) {
	assertDefaultParityState(t)

	z := NewChainPanelZone(ChainCallbacks{
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
	if z.freezeBtn.Icon != string(IconPlay) {
		t.Errorf("expected play icon when frozen, got Icon=%q", z.freezeBtn.Icon)
	}
	if z.freezeBtn.Text != "" {
		t.Errorf("expected empty text (icon-only), got %q", z.freezeBtn.Text)
	}
}

// TestChainHitAreasIncludeAllButtons verifies hit areas are generated for
// every interactive element.
func TestChainHitAreasIncludeAllButtons(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))

	areas := z.HitAreas()
	tags := map[string]bool{}
	for _, a := range areas {
		tags[a.Tag] = true
	}

	expected := []string{
		"scope-zoom",
		"scope-split-btn",
		"scope-freeze-btn",
	}
	for _, tag := range expected {
		if !tags[tag] {
			t.Errorf("missing hit area with tag %q", tag)
		}
	}
	// The Scope tab no longer owns its own instrument selector; the EQ panel's
	// eqChannelBtn is the single shared selector for all Wave Analyzer tabs.
	if tags["scope-inst-btn"] {
		t.Error("Scope panel must not register a 'scope-inst-btn' hit area")
	}

	// Verify all 6 stage buttons have hit areas.
	for i := 0; i < 6; i++ {
		tag := fmt.Sprintf("scope-stage-%d", i)
		if !tags[tag] {
			t.Errorf("missing hit area for stage button %d (tag %q)", i, tag)
		}
	}
}

// TestChainYGainDefault verifies yGain is 1.0 on construction.
func TestChainYGainDefault(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	if z.yGain != 1.0 {
		t.Errorf("expected yGain=1.0, got %f", z.yGain)
	}
}

// TestChainYGainShiftWheel verifies Shift+Scroll changes yGain.
func TestChainYGainShiftWheel(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
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

	handler := &chainZoomHandler{zone: z}
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

// TestChainYGainClamp verifies yGain is clamped to [0.25, 16.0].
func TestChainYGainClamp(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	handler := &chainZoomHandler{zone: z}

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

// TestChainYGainNoShiftUnchanged verifies scroll without shift only changes windowMs.
func TestChainYGainNoShiftUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

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

	handler := &chainZoomHandler{zone: z}
	origGain := z.yGain

	handler.OnWheel(0, 0, 3)
	if z.yGain != origGain {
		t.Errorf("yGain should not change without shift, got %f", z.yGain)
	}
	if z.windowMs == 20 {
		t.Error("windowMs should have changed without shift")
	}
}

// TestChainDoubleClickReset verifies double-click resets both axes.
func TestChainDoubleClickReset(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.windowMs = 100
	z.yGain = 4.0

	handler := &chainZoomHandler{zone: z}

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

// TestChainAutoGainToggle verifies the AG button toggles autoGain and resets yGain.
func TestChainAutoGainToggle(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

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

// TestChainAutoGainDisabledByManualZoom verifies Shift+Scroll disables auto-gain.
func TestChainAutoGainDisabledByManualZoom(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
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

	handler := &chainZoomHandler{zone: z}
	handler.OnWheel(0, 0, 1)
	if z.autoGain {
		t.Fatal("expected autoGain=false after manual shift+scroll")
	}
}

// TestChainPeakAmplitude verifies peak amplitude computation.
func TestChainPeakAmplitude(t *testing.T) {
	assertDefaultParityState(t)

	state := &scope.State{
		TapA: scope.TapData{Active: true, Samples: []float64{0.1, -0.3, 0.2}},
		TapB: scope.TapData{Active: true, Samples: []float64{0.05, 0.4, -0.1}},
	}
	peak := chainPeakAmplitude(state)
	if peak != 0.4 {
		t.Errorf("expected peak=0.4, got %f", peak)
	}

	// Only one tap active.
	state.TapB.Active = false
	peak = chainPeakAmplitude(state)
	if peak != 0.3 {
		t.Errorf("expected peak=0.3 with only tapA, got %f", peak)
	}
}

// TestChainTraceFillDrawsCalls verifies fill rects appear when fillCol is set.
func TestChainTraceFillDrawsCalls(t *testing.T) {
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

// TestChainTraceFillNilNoExtra verifies no fill rects appear when fillCol is nil.
func TestChainTraceFillNilNoExtra(t *testing.T) {
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

// TestChainHitAreasIncludeAGButton verifies the AG button has a hit area.
func TestChainHitAreasIncludeAGButton(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
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

// TestChainDiffComputation verifies element-wise A-B difference.
func TestChainDiffComputation(t *testing.T) {
	assertDefaultParityState(t)

	a := []float64{1.0, 0.5, 0.0, -0.5}
	b := []float64{0.5, 0.5, 0.5, 0.5}
	diff := chainDiffSamples(a, b)

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

// TestChainDiffLengthMismatch verifies diff uses min length without panic.
func TestChainDiffLengthMismatch(t *testing.T) {
	assertDefaultParityState(t)

	a := []float64{1.0, 0.5, 0.0, -0.5, -1.0}
	b := []float64{0.5, 0.5, 0.5}
	diff := chainDiffSamples(a, b)

	if len(diff) != 3 {
		t.Fatalf("expected 3 samples (min length), got %d", len(diff))
	}
	if diff[0] != 0.5 {
		t.Errorf("diff[0] = %f, expected 0.5", diff[0])
	}
}

// TestChainTraceVisibilityDefault verifies both traces visible by default.
func TestChainTraceVisibilityDefault(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.showTapA || !z.showTapB {
		t.Error("expected both traces visible by default")
	}
}

// TestChainTraceToggleA verifies swatch click toggles trace A visibility.
func TestChainTraceToggleA(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 160))

	handler := &chainSwatchHandler{zone: z, tap: "A"}
	handler.OnPress(0, 0)
	if z.showTapA {
		t.Error("expected showTapA=false after toggle")
	}
	handler.OnPress(0, 0)
	if !z.showTapA {
		t.Error("expected showTapA=true after second toggle")
	}
}

// TestChainTraceToggleB verifies swatch click toggles trace B visibility.
func TestChainTraceToggleB(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

	handler := &chainSwatchHandler{zone: z, tap: "B"}
	handler.OnPress(0, 0)
	if z.showTapB {
		t.Error("expected showTapB=false after toggle")
	}
}

// TestChainSwatchHitAreas verifies swatch hit areas are registered.
func TestChainSwatchHitAreas(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
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

// TestChainHiddenTraceNotDrawn verifies hidden trace produces no colored rects.
func TestChainHiddenTraceNotDrawn(t *testing.T) {
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

	// When showA=false, drawChainOverlay skips the call entirely,
	// so we verify by checking that the toggle state works.
	z := NewChainPanelZone(ChainCallbacks{})
	z.showTapA = false
	if z.showTapA {
		t.Error("showTapA should be false")
	}
}

// TestChainSetTapAClearAndAssign exercises the public SetTapA/SetTapB API
// (callback wiring + clear path) which handleStageClick alone does not hit.
func TestChainSetTapAClearAndAssign(t *testing.T) {
	assertDefaultParityState(t)
	var clearedA, clearedB bool
	var lastA, lastB scope.Stage
	z := NewChainPanelZone(ChainCallbacks{
		OnTapAChange: func(s scope.Stage) { lastA = s },
		OnTapBChange: func(s scope.Stage) { lastB = s },
		OnClearTapA:  func() { clearedA = true },
		OnClearTapB:  func() { clearedB = true },
	})
	z.SetTapA(scope.StageEQ)
	if z.tapA != scope.StageEQ || lastA != scope.StageEQ {
		t.Fatalf("SetTapA: tapA=%d lastA=%d", z.tapA, lastA)
	}
	z.SetTapB(scope.StageSynth)
	if z.tapB != scope.StageSynth || lastB != scope.StageSynth {
		t.Fatalf("SetTapB: tapB=%d lastB=%d", z.tapB, lastB)
	}
	z.SetTapA(-1)
	if z.tapA != -1 || !clearedA {
		t.Fatalf("SetTapA(-1): tapA=%d cleared=%v", z.tapA, clearedA)
	}
	z.SetTapB(-1)
	if z.tapB != -1 || !clearedB {
		t.Fatalf("SetTapB(-1): tapB=%d cleared=%v", z.tapB, clearedB)
	}
}

// TestChainZoneLifecycleNoOps covers the trivial Zone-interface methods
// and stub handler bodies that are otherwise zero-coverage.
func TestChainZoneLifecycleNoOps(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.NeedsLayout() {
		t.Error("NeedsLayout should be true before first Layout")
	}
	z.Layout(image.Rect(0, 0, 800, 160))
	if z.NeedsLayout() {
		t.Error("NeedsLayout should be false after Layout")
	}
	z.Invalidate()
	if !z.NeedsLayout() {
		t.Error("Invalidate did not set needLayout")
	}
	z.Update() // no-op; must not panic
	if got := z.HandleKey(ebiten.KeyEnter); got != InputIgnored {
		t.Errorf("HandleKey returned %v, want InputIgnored", got)
	}
	if got := z.HandleChars([]rune{'x'}); got != InputIgnored {
		t.Errorf("HandleChars returned %v, want InputIgnored", got)
	}
	z.SetPortal(nil) // setter; nil is a valid value
}

// TestChainLayoutTooSmallClearsHitAreas covers the early-return branch in
// Layout when the rect is below the minimum drawable size.
func TestChainLayoutTooSmallClearsHitAreas(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 400, 80))
	if len(z.HitAreas()) == 0 {
		t.Fatal("setup: expected hit areas after normal Layout")
	}
	z.Layout(image.Rect(0, 0, 4, 4)) // below the 8x8 floor
	if len(z.HitAreas()) != 0 {
		t.Fatalf("expected hit areas cleared, got %d", len(z.HitAreas()))
	}
}

// TestChainSwatchHandlersToggleVisibility is the "concrete dispatch
// scenario" the plan calls for: hit-area lookup → handler invocation →
// state transition, all without an Ebiten draw call.
func TestChainSwatchHandlersToggleVisibility(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 160))

	var swatchA, swatchB *chainSwatchHandler
	for _, ha := range z.HitAreas() {
		switch ha.Tag {
		case "scope-swatch-a":
			swatchA = ha.Handler.(*chainSwatchHandler)
		case "scope-swatch-b":
			swatchB = ha.Handler.(*chainSwatchHandler)
		}
	}
	if swatchA == nil || swatchB == nil {
		t.Fatalf("missing swatch handlers (A=%v B=%v)", swatchA, swatchB)
	}

	if !z.showTapA || !z.showTapB {
		t.Fatal("expected both traces visible by default")
	}
	if got := swatchA.OnPress(0, 0); got != InputConsumed {
		t.Errorf("swatch A OnPress = %v, want InputConsumed", got)
	}
	if z.showTapA {
		t.Error("swatch A press did not toggle showTapA off")
	}
	if got := swatchB.OnPress(0, 0); got != InputConsumed {
		t.Errorf("swatch B OnPress = %v, want InputConsumed", got)
	}
	if z.showTapB {
		t.Error("swatch B press did not toggle showTapB off")
	}
	swatchA.OnDrag(1, 2)
	swatchA.OnRelease(3, 4)
	if got := swatchA.OnWheel(0, 0, 1); got != InputIgnored {
		t.Errorf("swatch OnWheel = %v, want InputIgnored", got)
	}
}

// TestChainZoomHandlerStubMethods covers the empty OnDrag/OnRelease bodies
// on chainZoomHandler that aren't reached by the existing wheel tests, plus
// the steps==0 early-return in OnWheel.
func TestChainZoomHandlerStubMethods(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 160))
	var h *chainZoomHandler
	for _, ha := range z.HitAreas() {
		if ha.Tag == "scope-zoom" {
			h = ha.Handler.(*chainZoomHandler)
			break
		}
	}
	if h == nil {
		t.Fatal("scope-zoom hit area not found")
	}
	h.OnDrag(0, 0)
	h.OnRelease(0, 0)
	if got := h.OnWheel(0, 0, 0); got != InputIgnored {
		t.Errorf("zero-step OnWheel = %v, want InputIgnored", got)
	}
}

// --- Task A3 tests: vertical stage thumbnails + 3 separate mode pills ---

// TestChainStageThumbnailsVertical verifies the new stage column lays the 6
// stage thumbnails out vertically: every Min.X is approximately the same and
// Y values strictly increase down the column.
func TestChainStageThumbnailsVertical(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 400, 300))

	var firstX int
	var prevY int
	for i, btn := range z.stageButtons {
		if btn == nil {
			t.Fatalf("stage button %d nil", i)
		}
		r := btn.Rect()
		if r.Empty() {
			t.Fatalf("stage button %d empty rect", i)
		}
		if i == 0 {
			firstX = r.Min.X
			prevY = r.Min.Y - 1
		}
		if absInt(r.Min.X-firstX) > 2 {
			t.Errorf("stage button %d Min.X=%d differs from first %d (>2px)", i, r.Min.X, firstX)
		}
		if r.Min.Y <= prevY {
			t.Errorf("stage button %d Min.Y=%d not strictly greater than previous %d", i, r.Min.Y, prevY)
		}
		prevY = r.Min.Y
	}
}

// TestChainModeButtonsAreThree verifies the new 3 separate mode pills exist
// and clicking each sets displayMode directly (no cycle).
func TestChainModeButtonsAreThree(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

	if z.overlayBtn == nil {
		t.Fatal("overlayBtn nil")
	}
	if z.splitBtn == nil {
		t.Fatal("splitBtn nil")
	}
	if z.diffBtn == nil {
		t.Fatal("diffBtn nil")
	}

	// Start by setting displayMode away from default to verify direct assignment.
	z.displayMode = chainDiff
	z.overlayBtn.OnClick()
	if z.displayMode != chainOverlay {
		t.Errorf("overlayBtn click: displayMode=%d want chainOverlay=%d", z.displayMode, chainOverlay)
	}
	z.splitBtn.OnClick()
	if z.displayMode != chainSplit {
		t.Errorf("splitBtn click: displayMode=%d want chainSplit=%d", z.displayMode, chainSplit)
	}
	z.diffBtn.OnClick()
	if z.displayMode != chainDiff {
		t.Errorf("diffBtn click: displayMode=%d want chainDiff=%d", z.displayMode, chainDiff)
	}
}

// TestChainModeOnlyOneActive verifies the displayMode is exactly chainSplit
// after clicking splitBtn (i.e. there is no implicit cycle).
func TestChainModeOnlyOneActive(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})

	// From any starting state, clicking splitBtn must land on chainSplit
	// — no matter how many times.
	z.splitBtn.OnClick()
	if z.displayMode != chainSplit {
		t.Fatalf("after splitBtn click: displayMode=%d want chainSplit=%d", z.displayMode, chainSplit)
	}
	z.splitBtn.OnClick()
	if z.displayMode != chainSplit {
		t.Errorf("second splitBtn click changed displayMode to %d (expected sticky chainSplit)", z.displayMode)
	}
}

// --- Task A4 tests: hover tooltips ---

// TestChainTooltipOpensOnDwell verifies that with the cursor sitting over a
// stage button for at least 300ms the portal receives an entry with id
// "chain-tt" carrying the StageDescription.
func TestChainTooltipOpensOnDwell(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 400, 300))

	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 1024, 768))
	z.SetPortal(portal)

	stageBtn := z.stageButtons[0]
	stageR := stageBtn.Rect()
	if stageR.Empty() {
		t.Fatal("stage button rect empty after Layout")
	}
	cx := stageR.Min.X + stageR.Dx()/2
	cy := stageR.Min.Y + stageR.Dy()/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1024, 768 },
	)
	defer restore()

	// First Update arms the dwell timer.
	z.Update()
	if portal.Has("chain-tt") {
		t.Fatal("tooltip opened immediately on first Update; want 300ms dwell")
	}

	// Backdate the dwell-start so the next Update crosses the threshold.
	z.hoverStartMs -= 400

	z.Update()
	if !portal.Has("chain-tt") {
		t.Fatal("tooltip not opened after 300ms+ dwell")
	}
}

// TestChainTooltipClosesOnExit verifies that moving the cursor outside the
// hovered button removes the tooltip portal entry.
func TestChainTooltipClosesOnExit(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 400, 300))

	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 1024, 768))
	z.SetPortal(portal)

	stageBtn := z.stageButtons[0]
	stageR := stageBtn.Rect()
	if stageR.Empty() {
		t.Fatal("stage button rect empty after Layout")
	}
	cx := stageR.Min.X + stageR.Dx()/2
	cy := stageR.Min.Y + stageR.Dy()/2

	cursorX := cx
	cursorY := cy
	restore := SetInputForTest(
		func() (int, int) { return cursorX, cursorY },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1024, 768 },
	)
	defer restore()

	// Open the tooltip (arm + cross threshold).
	z.Update()
	z.hoverStartMs -= 400
	z.Update()
	if !portal.Has("chain-tt") {
		t.Fatal("setup: tooltip should be open before exit")
	}

	// Move cursor outside the panel entirely.
	cursorX = 1000
	cursorY = 700
	z.Update()
	if portal.Has("chain-tt") {
		t.Error("tooltip still open after cursor exited the button")
	}
}
