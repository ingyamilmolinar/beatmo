//go:build test

package ui

import (
	"image"
	"math"
	"testing"
)

// eqHitByTag returns the first hit area with the given tag, or nil.
func eqHitByTag(z *EQPanelZone, tag string) *HitArea {
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == tag {
			return &z.hitAreas[i]
		}
	}
	return nil
}

// TestEQNoStepperButtons — the +/- stepper was removed; drag-to-slide is the
// mobile edit gesture. No stepper hit areas may exist on any layout.
func TestEQNoStepperButtons(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		forceSmallScreenForTest = mobile
		UpdateProfile()
		d := DensityComfortable
		if mobile {
			d = DensitySpacious
		}
		restore := SetDensityForTest(d)
		z, _ := newTestEQPanelZone(nil)
		z.tabState.SetActiveTab(TabEQ)
		z.Layout(image.Rect(0, 200, 400, 500))
		if eqHitByTag(z, "eq-step-plus") != nil || eqHitByTag(z, "eq-step-minus") != nil {
			t.Errorf("mobile=%v: stepper buttons must not exist", mobile)
		}
		restore()
	}
	forceSmallScreenForTest = false
	UpdateProfile()
}

// TestEQMobileDragSlidesBand — the core requirement: on mobile the user drags
// a band node up/down to change its gain (column grab + drag), no stepper.
func TestEQMobileDragSlidesBand(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	in := noInputForTest()
	defer in()

	z, log := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	z.Layout(image.Rect(0, 200, 400, 500))
	z.bandGainsDB[4] = 0
	h := eqCurveHandlerFor(t, z)

	pr := z.eqPlotRect()
	hx, _ := z.eqBandHandlePos(4)
	// Press anywhere in band 4's column, then drag up to a boost.
	if r := h.OnPress(hx, gainDBToY(0, pr)); r != InputCaptured {
		t.Fatalf("press in band column did not capture (%v)", r)
	}
	h.OnDrag(hx, gainDBToY(8, pr))
	if z.bandGainsDB[4] < 6 {
		t.Errorf("drag up did not boost band: gain=%.1f want >6", z.bandGainsDB[4])
	}
	h.OnDrag(hx, gainDBToY(-8, pr))
	if z.bandGainsDB[4] > -6 {
		t.Errorf("drag down did not cut band: gain=%.1f want <-6", z.bandGainsDB[4])
	}
	if len(log.gainChanges) == 0 {
		t.Error("drag must fire OnGainChange")
	}
}

// TestEQBandLabelsStackedNotOverlapped — each band renders its frequency above
// its level (stacked), the two never overlap, and neighbouring bands' label
// cells don't overlap either. Both desktop and mobile.
func TestEQBandLabelsStackedNotOverlapped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		dens   Density
		r      image.Rectangle
	}{
		{"desktop", false, DensityComfortable, image.Rect(0, 540, 1280, 720)},
		{"mobile", true, DensitySpacious, image.Rect(0, 574, 390, 844)},
	} {
		forceSmallScreenForTest = tc.mobile
		UpdateProfile()
		restore := SetDensityForTest(tc.dens)
		z, _ := newTestEQPanelZone(nil)
		z.tabState.SetActiveTab(TabEQ)
		z.Layout(tc.r)

		var prevLevel, prevFreq image.Rectangle
		for i := range eqBandDefs {
			freq, level := z.eqBandLabelRects(i)
			if freq.Empty() || level.Empty() {
				t.Fatalf("%s band%d: empty label rect", tc.name, i)
			}
			// Stacked: frequency strictly above level, no vertical overlap.
			if freq.Max.Y > level.Min.Y {
				t.Errorf("%s band%d: freq %v overlaps level %v (not stacked)", tc.name, i, freq, level)
			}
			if i > 0 {
				if freq.Overlaps(prevFreq) {
					t.Errorf("%s band%d freq %v overlaps band%d freq %v", tc.name, i, freq, i-1, prevFreq)
				}
				if level.Overlaps(prevLevel) {
					t.Errorf("%s band%d level %v overlaps band%d level %v", tc.name, i, level, i-1, prevLevel)
				}
			}
			prevFreq, prevLevel = freq, level
		}
		restore()
	}
	forceSmallScreenForTest = false
	UpdateProfile()
}

// TestEQFilterButtonsLargerOnMobile — HP/LP toggle buttons must be visibly
// larger on mobile than desktop (so they're easy to hit), in addition to the
// touch-min hit expansion.
func TestEQFilterButtonsLargerOnMobile(t *testing.T) {
	measure := func(mobile bool) (w, h int) {
		forceSmallScreenForTest = mobile
		UpdateProfile()
		d := DensityComfortable
		if mobile {
			d = DensitySpacious
		}
		restore := SetDensityForTest(d)
		defer restore()
		z, _ := newTestEQPanelZone(nil)
		z.tabState.SetActiveTab(TabEQ)
		z.Layout(image.Rect(0, 200, 400, 500))
		r := z.hpfBtn.Rect()
		return r.Dx(), r.Dy()
	}
	dw, dh := measure(false)
	mw, mh := measure(true)
	forceSmallScreenForTest = false
	UpdateProfile()
	if mh <= dh || mw < dw {
		t.Errorf("HP button should be larger on mobile: desktop %dx%d, mobile %dx%d", dw, dh, mw, mh)
	}
}

// TestEQChannelPillTouchMinMobile — the Master/channel dropdown pill must
// present a touch-min hit target on mobile (its hit area is touch-expanded;
// the visible pill grows with the taller mobile sticky bar).
func TestEQChannelPillTouchMinMobile(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	z.Layout(image.Rect(0, 200, 400, 500))

	mt := Profile().MinTarget
	// Visible pill must be taller than the desktop 18 px (bigger on mobile).
	if pill := z.stickyBar.ChannelBtn().Rect(); pill.Dy() <= 18 {
		t.Errorf("Master pill visible height %d not enlarged on mobile", pill.Dy())
	}
	// Hit target must meet the touch-min.
	ha := eqHitByTag(z, "eq-channel-btn")
	if ha == nil {
		t.Fatal("channel pill hit area missing")
	}
	if !ha.Touch || ha.ClipRect.Dx() < mt || ha.ClipRect.Dy() < mt {
		t.Errorf("channel pill touch target %v (touch=%v) below MinTarget %d", ha.ClipRect, ha.Touch, mt)
	}
}

// eqCurveHandlerFor returns the registered "eq-curve-area" hit handler.
func eqCurveHandlerFor(t *testing.T, z *EQPanelZone) HitHandler {
	t.Helper()
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			return a.Handler
		}
	}
	t.Fatal("eq-curve-area hit area not found")
	return nil
}

// TestEQMobileColumnGrabSelectsNearestBand — on mobile, a press anywhere in
// the curve plot (not precisely on a handle circle) must select the nearest
// band by frequency column and begin a drag. This is the core "EQ is hard to
// use on mobile" fix: a finger can't reliably hit a 26-px handle among 10
// packed bands, so the whole column is the grab target.
func TestEQMobileColumnGrabSelectsNearestBand(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	in := noInputForTest()
	defer in()

	z, _ := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	r := image.Rect(0, 200, 400, 500)
	z.Layout(r)

	// Push every band's handle to the top of the plot so a press near the
	// bottom of the plot is far from every handle (precise hit fails).
	for i := range z.bandGainsDB {
		z.bandGainsDB[i] = 12
	}

	handler := eqCurveHandlerFor(t, z)

	band := 3
	def := eqBandDefs[band]
	cx := freqToX(math.Sqrt(def.loHz*def.hiHz), r)

	// Press low in the plot (just above the mute/dB chrome), at band 3's
	// column X — nowhere near any handle.
	muteTop := z.eqMuteBtns[0].Rect().Min.Y
	py := muteTop - 2
	z.curveDragBand = -1
	if res := handler.OnPress(cx, py); res != InputCaptured {
		t.Fatalf("mobile column press did not capture (res=%v)", res)
	}
	if z.curveDragBand != band {
		t.Errorf("column grab selected band %d, want nearest band %d", z.curveDragBand, band)
	}
}

// TestEQDesktopNoColumnGrab — desktop keeps precise-handle grab (mouse is
// precise; column grab would be ambiguous over overlapping bands). A press
// off the handle must miss.
func TestEQDesktopNoColumnGrab(t *testing.T) {
	forceSmallScreenForTest = false
	UpdateProfile()
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	in := noInputForTest()
	defer in()

	z, _ := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)
	for i := range z.bandGainsDB {
		z.bandGainsDB[i] = 12
	}

	handler := eqCurveHandlerFor(t, z)
	band := 3
	def := eqBandDefs[band]
	cx := freqToX(math.Sqrt(def.loHz*def.hiHz), r)
	muteTop := z.eqMuteBtns[0].Rect().Min.Y
	py := muteTop - 2
	z.curveDragBand = -1
	if res := handler.OnPress(cx, py); res == InputCaptured {
		t.Error("desktop press off-handle should miss (precise grab only)")
	}
}

// TestEQMobileColumnGrabExcludesBottomChrome — the column grab must not fire
// over the mute / dB-input chrome at the bottom; that region belongs to those
// controls. Guards TestMobileHitRadiusNew's bottom-edge "miss".
func TestEQMobileColumnGrabExcludesBottomChrome(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	in := noInputForTest()
	defer in()

	z, _ := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	r := image.Rect(0, 200, 400, 500)
	z.Layout(r)

	handler := eqCurveHandlerFor(t, z)
	def := eqBandDefs[3]
	cx := freqToX(math.Sqrt(def.loHz*def.hiHz), r)
	z.curveDragBand = -1
	if res := handler.OnPress(cx, r.Max.Y-1); res == InputCaptured {
		t.Error("press in the bottom mute/dB chrome should not start a curve drag")
	}
}

// TestEQHandlesClearBottomLabelsAllLayouts — the gain plot region must not
// overlap the bottom freq-label / dB strip, so a band handle (even at an
// extreme ±12 dB gain) never covers the frequency-range label or the level
// number. Guards the "numbers not visible — handle on top of the labels" bug
// on both desktop and mobile.
func TestEQHandlesClearBottomLabelsAllLayouts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		dens   Density
		r      image.Rectangle
	}{
		{"desktop", false, DensityComfortable, image.Rect(0, 540, 1280, 720)},
		{"mobile", true, DensitySpacious, image.Rect(0, 574, 390, 844)},
	} {
		forceSmallScreenForTest = tc.mobile
		UpdateProfile()
		restore := SetDensityForTest(tc.dens)
		z, _ := newTestEQPanelZone(nil)
		z.tabState.SetActiveTab(TabEQ)
		z.Layout(tc.r)

		plot := z.eqPlotRect()
		labelTop := z.eqLabelStripTop()
		if plot.Max.Y > labelTop {
			t.Errorf("%s: plot bottom %d overlaps label strip starting at %d", tc.name, plot.Max.Y, labelTop)
		}
		// Drive every band to the extreme gains and assert the handle the
		// renderer/hit-test actually uses (eqBandHandlePos) stays inside the
		// plot, clear of the bottom label strip.
		for _, db := range []float64{eqCurveMaxDB, eqCurveMinDB} {
			for i := range z.bandGainsDB {
				z.bandGainsDB[i] = db
			}
			for i := range eqBandDefs {
				_, hy := z.eqBandHandlePos(i)
				if hy < plot.Min.Y || hy > plot.Max.Y {
					t.Errorf("%s band%d @%.0fdB: handle Y=%d escapes plot %v", tc.name, i, db, hy, plot)
				}
				if hy >= labelTop {
					t.Errorf("%s band%d @%.0fdB: handle Y=%d reaches label strip (top %d)", tc.name, i, db, hy, labelTop)
				}
			}
		}
		restore()
	}
	forceSmallScreenForTest = false
	UpdateProfile()
}

// TestEQFilterButtonsTouchMinMobile — the HP / LP toggle buttons must present
// a touch-min hit target on mobile (the visible 28×18 chrome is below 44 px;
// the hit area is expanded via Touch + ClipRect without growing the visual).
func TestEQFilterButtonsTouchMinMobile(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	in := noInputForTest()
	defer in()

	z, _ := newTestEQPanelZone(nil)
	z.tabState.SetActiveTab(TabEQ)
	z.Layout(image.Rect(0, 200, 400, 500))

	minTarget := Profile().MinTarget
	found := map[string]bool{}
	for _, a := range z.HitAreas() {
		if a.Tag != "eq-hpf-btn" && a.Tag != "eq-lpf-btn" {
			continue
		}
		found[a.Tag] = true
		if !a.Touch {
			t.Errorf("%s must use Touch expansion on mobile", a.Tag)
		}
		if a.ClipRect.Dx() < minTarget || a.ClipRect.Dy() < minTarget {
			t.Errorf("%s touch clip %v below MinTarget %d", a.Tag, a.ClipRect, minTarget)
		}
	}
	if !found["eq-hpf-btn"] || !found["eq-lpf-btn"] {
		t.Errorf("HP/LP hit areas missing (got %v)", found)
	}
}
