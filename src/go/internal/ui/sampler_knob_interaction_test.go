//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSamplerKnobScalesAndBadges(t *testing.T) {
	for _, idx := range []int{samplerKnobStart, samplerKnobEnd, samplerKnobTranspose, samplerKnobDetune, samplerKnobGain} {
		sc := samplerKnobScale(idx)
		if sc.Max <= sc.Min {
			t.Fatalf("knob %d scale invalid: %+v", idx, sc)
		}
	}
	if u := samplerKnobScale(samplerKnobTranspose).Unit; u != "st" {
		t.Fatalf("transpose unit=%q want st", u)
	}
	if u := samplerKnobScale(samplerKnobGain).Unit; u != "dB" {
		t.Fatalf("gain unit=%q want dB", u)
	}

	g := newSamplerTabGame(t)
	layoutSamplerTab(t, g)
	s := &g.drum.sampler
	for i, k := range s.knobs {
		if k == nil {
			t.Fatalf("knob %d nil", i)
		}
		if !k.Endless {
			t.Fatalf("sampler knob %d should be Endless", i)
		}
		if k.StepMul <= 0 {
			t.Fatalf("sampler knob %d StepMul not initialised", i)
		}
		if i >= len(s.knobStepBadges) || s.knobStepBadges[i] == nil {
			t.Fatalf("sampler knob %d missing step badge", i)
		}
	}
}

// TestSamplerBadgeTapCyclesStepViaTreeDispatch verifies the FULL production
// input path for sampler badge taps:
//
//	mouse → SetInputForTest → g.Update() → HitIndex → samplerKnobHitAdapter.OnPress → cycleSamplerKnobStep
func TestSamplerBadgeTapCyclesStepViaTreeDispatch(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick") // make hasBuffer() true so knobs lay out
	layoutSamplerTab(t, g)
	g.Update() // flush hit-index

	img := newTrackedImage("test.samplerbadgetree", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate badge rects from a real draw

	idx := samplerKnobGain
	badge := g.drum.sampler.knobStepBadges[idx]
	if badge == nil || badge.Rect().Empty() {
		t.Fatalf("sampler badge rect not populated after draw (knob rect=%v)", g.drum.sampler.knobs[idx].Rect())
	}
	before := badge.Step()
	r := badge.Rect()
	bx, by := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	// Invalidate so the next Update's layoutPass re-publishes HitAreas,
	// picking up the badge rect that was just populated by Draw.
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := bx, by
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	pressed = true
	g.Update() // layout re-publishes badge HitArea; tree dispatches press to OnPress
	pressed = false
	g.Update()

	if badge.Step() == before {
		t.Fatalf("sampler badge step did not cycle via tree dispatch (still %v)", before)
	}
	if g.drum.sampler.knobs[idx].StepMul != badge.Step() {
		t.Fatalf("sampler knob StepMul (%v) not synced to badge step (%v) after tap",
			g.drum.sampler.knobs[idx].StepMul, badge.Step())
	}
}

// TestSamplerReadoutTapOpensEditorViaTreeDispatch verifies the FULL production
// input path for sampler readout taps:
//
//	mouse → SetInputForTest → g.Update() → HitIndex → samplerKnobHitAdapter.OnPress → openSamplerParamEditor
func TestSamplerReadoutTapOpensEditorViaTreeDispatch(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	g.Update()

	img := newTrackedImage("test.samplerreadouttree", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate readout rects from a real draw

	idx := samplerKnobGain
	rr := g.drum.samplerKnobReadoutRect(idx)
	if rr.Empty() {
		t.Fatalf("sampler readout rect not populated after draw")
	}

	// Find a tap point inside the readout rect but outside any badge rect.
	// The readout spans the full knob cell width; badges are narrow centered pills.
	tx, ty := rr.Min.X+2, rr.Min.Y+rr.Dy()/2
	badge := g.drum.sampler.knobStepBadges[idx]
	if badge != nil && !badge.Rect().Empty() && image.Pt(tx, ty).In(badge.Rect()) {
		// Left edge is inside the badge; try right edge.
		tx = rr.Max.X - 2
		if image.Pt(tx, ty).In(badge.Rect()) {
			t.Fatalf("both left and right edges of readout rect (%v) are inside badge rect (%v) — geometry overlap", rr, badge.Rect())
		}
	}

	// Invalidate so the next Update's layoutPass re-publishes HitAreas,
	// picking up the readout rect that was just populated by Draw.
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := tx, ty
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	pressed = true
	g.Update() // layout re-publishes readout HitArea; tree dispatches press to OnPress
	pressed = false
	g.Update()

	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("sampler readout tap via tree did not open the editor (tap=(%d,%d) readout=%v)", tx, ty, rr)
	}
	// Commit a gain value (~0 dB) and confirm the sampler state changed.
	g.drum.paramEditor.ti.SetText("0")
	g.drum.paramEditor.commit()
	if g.drum.sampler.gainDB < -0.5 || g.drum.sampler.gainDB > 0.5 {
		t.Fatalf("sampler gainDB after edit=%v want ~0", g.drum.sampler.gainDB)
	}
}

// TestSamplerNumericCommitDoesNotAudition opens the numeric editor for the
// sampler gain knob, commits "0" (0 dB), and asserts no audition fires. The
// gainDB state update confirms the commit is non-vacuous.
func TestSamplerNumericCommitDoesNotAudition(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)

	restore := SwapSamplerAuditionFnForTest(func(string) { t.Fatalf("sampler numeric commit must NOT audition") })
	defer SwapSamplerAuditionFnForTest(restore)

	idx := samplerKnobGain
	g.drum.openSamplerParamEditor(idx)
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("openSamplerParamEditor did not open the editor (idx=%d)", idx)
	}

	g.drum.paramEditor.ti.SetText("0")
	g.drum.paramEditor.commit()

	// Confirm the commit wrote gainDB (non-vacuous assertion).
	if g.drum.sampler.gainDB < -0.5 || g.drum.sampler.gainDB > 0.5 {
		t.Fatalf("sampler numeric commit did not propagate: gainDB=%v want ~0", g.drum.sampler.gainDB)
	}
}

// TestSamplerBadgeWheelChangesResolutionViaTreeDispatch verifies scrolling over
// the sampler step-resolution badge shifts the rung via the real tree dispatch,
// without changing the knob value.
func TestSamplerBadgeWheelChangesResolutionViaTreeDispatch(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	g.Update()
	img := newTrackedImage("test.samplerbadgewheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	idx := samplerKnobGain
	badge := g.drum.sampler.knobStepBadges[idx]
	if badge == nil || badge.Rect().Empty() {
		t.Fatalf("sampler badge rect not populated")
	}
	knob := g.drum.sampler.knobs[idx]
	valBefore := knob.Value
	stepBefore := badge.Step()

	r := badge.Rect()
	mouseX, mouseY := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	wy := 0.0
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	// Debounced/clicky: a single notch must not move the rung.
	wy = -1 // coarser
	g.Update()
	if badge.Step() != stepBefore {
		t.Fatalf("a single wheel notch must not change sampler resolution: %v -> %v", stepBefore, badge.Step())
	}
	for i := 1; i < knobBadgeWheelNotchesPerRung; i++ {
		g.Update()
	}
	wy = 0

	if badge.Step() <= stepBefore {
		t.Fatalf("scroll over sampler badge should coarsen: before=%v after=%v", stepBefore, badge.Step())
	}
	if knob.StepMul != badge.Step() {
		t.Fatalf("sampler knob StepMul not synced: %v vs %v", knob.StepMul, badge.Step())
	}
	if knob.Value != valBefore {
		t.Fatalf("wheel over sampler badge must NOT turn the knob: %v -> %v", valBefore, knob.Value)
	}
}

// TestSamplerKnobWheelAxes: a two-finger LEFT/RIGHT scroll over a sampler knob
// changes its value; an UP/DOWN scroll does NOT (sampler has no overflow rows,
// so up/down is a no-op rather than nudging the value).
func TestSamplerKnobWheelAxes(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	g.Update()
	img := newTrackedImage("test.samplerwheelaxes", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	g.drum.eqPanelZone.Invalidate()

	idx := samplerKnobGain
	k := g.drum.sampler.knobs[idx]
	r := k.Rect()
	capH := Profile().DensityValues().SynthKnobCaptionH
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + (r.Dy()-capH)/2

	before := k.Value
	// Debounced/clicky: one horizontal notch must NOT move the value.
	wheelOverPoint(t, g, cx, cy, 3, 0)
	if k.Value != before {
		t.Fatalf("a single horizontal wheel notch must not move the sampler value: %.4f -> %.4f", before, k.Value)
	}
	for i := 1; i < knobValueWheelNotchesPerStep; i++ {
		wheelOverPoint(t, g, cx, cy, 3, 0) // accumulate to one step
	}
	if k.Value == before {
		t.Fatalf("horizontal wheel did not change sampler value after %d notches (%.4f)", knobValueWheelNotchesPerStep, before)
	}
	mid := k.Value
	wheelOverPoint(t, g, cx, cy, 0, 3) // vertical → must NOT change value
	if k.Value != mid {
		t.Fatalf("vertical wheel over sampler knob changed value (%.4f -> %.4f)", mid, k.Value)
	}
}
