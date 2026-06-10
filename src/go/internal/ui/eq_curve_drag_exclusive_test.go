//go:build test

package ui

import (
	"image"
	"math"
	"strings"
	"testing"
)

// --- Phase 6: EQ Curve Drag Exclusive Tests ---

func TestCurveDragExclusiveDesktop(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)
	pr := z.eqPlotRect() // handles map into the plot region, not the full panel

	// Find a band handle center (band 5 = 1 kHz).
	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, pr)
	hy := gainDBToY(0, pr) // starts at 0 dB

	// Find the curve area handler.
	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("eq-curve-area hit area not found")
	}

	// Press on the handle.
	result := handler.OnPress(hx, hy)
	if result != InputCaptured {
		t.Fatal("expected press on handle to capture input")
	}

	// Drag upward (boost).
	targetY := gainDBToY(6.0, pr)
	handler.OnDrag(hx, targetY)

	// Verify gain was updated.
	gain := z.bandGainsDB[band]
	if gain < 5.0 || gain > 7.0 {
		t.Errorf("expected gain ~6 dB, got %.1f", gain)
	}

	// Verify OnGainChange was called.
	if len(log.gainChanges) == 0 {
		t.Fatal("expected OnGainChange callback")
	}

	// Release.
	handler.OnRelease(hx, targetY)

	// Verify OnApplyEQ was called.
	if log.applyCount == 0 {
		t.Fatal("expected OnApplyEQ on release")
	}
}

func TestCurveDragExclusiveMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 200, 400, 500)
	z.Layout(r)
	pr := z.eqPlotRect()

	// Same flow as desktop.
	band := 3
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, pr)
	hy := gainDBToY(0, pr)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("eq-curve-area hit area not found")
	}

	result := handler.OnPress(hx, hy)
	if result != InputCaptured {
		t.Fatal("expected press on handle to capture input on mobile")
	}

	targetY := gainDBToY(-4.0, pr)
	handler.OnDrag(hx, targetY)

	gain := z.bandGainsDB[band]
	if gain > -3.0 || gain < -5.0 {
		t.Errorf("expected gain ~-4 dB, got %.1f", gain)
	}

	handler.OnRelease(hx, targetY)
	if log.applyCount == 0 {
		t.Fatal("expected OnApplyEQ on release")
	}
}

func TestNoSliderGroupHitArea(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	for _, a := range z.HitAreas() {
		if a.Tag == "eq-slider-group" {
			t.Fatal("eq-slider-group hit area should not exist")
		}
	}
}

func TestNoBandButtonHitArea(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 200, 400, 500))

	for _, a := range z.HitAreas() {
		if strings.HasPrefix(a.Tag, "eq-band-btn-") {
			t.Fatalf("eq-band-btn hit area should not exist, found %q", a.Tag)
		}
	}
}

func TestHandleSizeIncreased(t *testing.T) {
	if eqHandleRadius != 10 {
		t.Errorf("expected eqHandleRadius == 10, got %d", eqHandleRadius)
	}

	desktop := desktopProfile()
	if desktop.EQHandleRadius != 16 {
		t.Errorf("expected desktop EQHandleRadius == 16, got %d", desktop.EQHandleRadius)
	}

	mobile := mobileProfile()
	if mobile.EQHandleRadius != 26 {
		t.Errorf("expected mobile EQHandleRadius == 26, got %d", mobile.EQHandleRadius)
	}
}

func TestDBLabelShownDuringDrag(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(0, r)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	handler.OnPress(hx, hy)

	// After press, label should be set.
	if z.curveDragDBText == "" {
		t.Fatal("expected dB label to be set on press")
	}
	if !strings.Contains(z.curveDragDBText, "dB") {
		t.Errorf("expected label to contain 'dB', got %q", z.curveDragDBText)
	}

	// Drag to update.
	targetY := gainDBToY(6.0, r)
	handler.OnDrag(hx, targetY)
	if !strings.Contains(z.curveDragDBText, "+") {
		t.Errorf("expected positive dB label for boost, got %q", z.curveDragDBText)
	}

	handler.OnRelease(hx, targetY)
}

func TestDBLabelClearedOnRelease(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	band := 3
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(0, r)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	handler.OnPress(hx, hy)
	handler.OnDrag(hx, gainDBToY(3.0, r))
	handler.OnRelease(hx, gainDBToY(3.0, r))

	if z.curveDragDBText != "" {
		t.Errorf("expected label cleared on release, got %q", z.curveDragDBText)
	}
}

func TestDBLabelPositionAboveHandle(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(0, r) // mid-panel

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	handler.OnPress(hx, hy)

	// Label should be above handle (smaller Y).
	if z.curveDragLabelY >= hy {
		t.Errorf("expected label Y (%d) above handle Y (%d)", z.curveDragLabelY, hy)
	}

	handler.OnRelease(hx, hy)
}

func TestDBLabelFlipsBelowNearTop(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)
	pr := z.eqPlotRect()

	// Set band to max gain so handle is near the top.
	band := 5
	z.bandGainsDB[band] = 12.0

	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, pr)
	hy := z.eqHandleY(12.0) // near top (clamped within plot)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	handler.OnPress(hx, hy)

	// Label should be below handle (larger Y) when near top.
	if z.curveDragLabelY < hy {
		t.Errorf("expected label Y (%d) below handle Y (%d) near top", z.curveDragLabelY, hy)
	}

	handler.OnRelease(hx, hy)
}

func TestDBLabelAccuracyExtremes(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)
	pr := z.eqPlotRect()

	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, pr)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	// Drag to top = +12 dB.
	handler.OnPress(hx, gainDBToY(0, pr))
	handler.OnDrag(hx, gainDBToY(12.0, pr))
	if z.curveDragDBText != "+12.0 dB" {
		t.Errorf("expected '+12.0 dB', got %q", z.curveDragDBText)
	}

	handler.OnRelease(hx, gainDBToY(12.0, pr))

	// Drag to bottom = -12 dB.
	handler.OnPress(hx, z.eqHandleY(12.0)) // handle is now at +12 (clamped)
	handler.OnDrag(hx, gainDBToY(-12.0, pr))
	if z.curveDragDBText != "-12.0 dB" {
		t.Errorf("expected '-12.0 dB', got %q", z.curveDragDBText)
	}

	handler.OnRelease(hx, gainDBToY(-12.0, pr))
}

func TestFilterDragShowsHz(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	z.callbacks.HPFEnabled = func() bool { return true }
	z.callbacks.HPFCutoffHz = func() float64 { return 100 }
	z.callbacks.OnHPFCutoffChange = func(hz float64) {}

	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	hx := freqToX(100, r)
	hy := z.curveYAtX(hx)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	result := handler.OnPress(hx, hy)
	if result != InputCaptured {
		t.Fatal("expected HPF handle press to capture")
	}

	// Drag to a new frequency.
	newX := freqToX(200, r)
	handler.OnDrag(newX, hy)

	if !strings.Contains(z.curveDragDBText, "Hz") {
		t.Errorf("expected Hz label for filter drag, got %q", z.curveDragDBText)
	}

	handler.OnRelease(newX, hy)
}

func TestMuteButtonsStillWork(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	// Find a mute button hit area.
	var muteHandler HitHandler
	for _, a := range z.HitAreas() {
		if strings.HasPrefix(a.Tag, "eq-mute-") {
			muteHandler = a.Handler
			break
		}
	}
	if muteHandler == nil {
		t.Fatal("no mute button hit area found")
	}

	muteHandler.OnPress(0, 0) // coords don't matter for buttonHitAdapter

	if len(log.muteToggles) == 0 {
		t.Fatal("expected mute toggle callback")
	}
}

func TestCurveAreaZIndex(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	var curveZ, muteZ int
	curveFound, muteFound := false, false
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			curveZ = a.ZIndex
			curveFound = true
		}
		if strings.HasPrefix(a.Tag, "eq-mute-") && !muteFound {
			muteZ = a.ZIndex
			muteFound = true
		}
	}
	if !curveFound {
		t.Fatal("eq-curve-area not found")
	}
	if !muteFound {
		t.Fatal("eq-mute button not found")
	}

	if curveZ != 130 {
		t.Errorf("expected curve z-index 130, got %d", curveZ)
	}
	if muteZ != 131 {
		t.Errorf("expected mute z-index 131, got %d", muteZ)
	}
}

func TestMobileHitRadiusNew(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 200, 400, 500)
	z.Layout(r)

	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(0, r)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	// Press at 24px from center: should capture (mobile radius = 26).
	result := handler.OnPress(hx+24, hy)
	if result != InputCaptured {
		t.Error("expected 24px press to capture on mobile (radius=26)")
	}
	handler.OnRelease(hx+24, hy)
	z.curveDragBand = -1

	// Press well outside all handles (bottom of rect): should miss.
	result = handler.OnPress(hx, r.Max.Y-1)
	if result == InputCaptured {
		t.Error("expected press at bottom edge to miss all handles")
	}
}

func TestDesktopHitRadiusNew(t *testing.T) {
	forceSmallScreenForTest = false
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	r := image.Rect(0, 400, 600, 580)
	z.Layout(r)

	band := 5
	def := eqBandDefs[band]
	center := math.Sqrt(def.loHz * def.hiHz)
	hx := freqToX(center, r)
	hy := gainDBToY(0, r)

	var handler HitHandler
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			handler = a.Handler
			break
		}
	}
	if handler == nil {
		t.Fatal("curve area not found")
	}

	// Press at 14px from center: should capture (desktop radius = 16).
	result := handler.OnPress(hx+14, hy)
	if result != InputCaptured {
		t.Error("expected 14px press to capture on desktop (radius=16)")
	}
	handler.OnRelease(hx+14, hy)
	z.curveDragBand = -1

	// Press at 18px from center: should miss.
	result = handler.OnPress(hx+18, hy)
	if result == InputCaptured {
		t.Error("expected 18px press to miss on desktop (radius=16)")
	}
}
