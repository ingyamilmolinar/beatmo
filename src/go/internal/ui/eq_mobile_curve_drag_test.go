//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// --- Mobile curve handle drag tests ---

// eqFilterState holds mutable HPF/LPF state for testing.
type eqFilterState struct {
	hpfEnabled bool
	hpfCutoff  float64
	lpfEnabled bool
	lpfCutoff  float64
}

// newTestEQPanelZoneWithFilters creates an EQPanelZone with HPF/LPF callbacks wired.
func newTestEQPanelZoneWithFilters(rows []*DrumRow) (*EQPanelZone, *eqTestCallbackLog, *eqFilterState) {
	log := &eqTestCallbackLog{}
	fs := &eqFilterState{hpfCutoff: 200, lpfCutoff: 10000}
	cb := EQCallbacks{
		OnGainChange: func(band int, db float64) {
			log.gainChanges = append(log.gainChanges, eqGainChange{band, db})
		},
		OnMuteToggle: func(band int) {
			log.muteToggles = append(log.muteToggles, band)
		},
		OnChannelChange: func(id string) {
			log.channelChanges = append(log.channelChanges, id)
		},
		OnToggleHPF: func() { log.hpfToggles++ },
		OnToggleLPF: func() { log.lpfToggles++ },
		OnApplyEQ:   func() { log.applyCount++ },
		AnalyzerSnapshot: func(ch string) audio.AnalyzerSnapshot {
			return audio.AnalyzerSnapshot{}
		},
		ActiveRows:     func() []*DrumRow { return rows },
		HPFEnabled:     func() bool { return fs.hpfEnabled },
		HPFCutoffHz:    func() float64 { return fs.hpfCutoff },
		LPFEnabled:     func() bool { return fs.lpfEnabled },
		LPFCutoffHz:    func() float64 { return fs.lpfCutoff },
		OnHPFCutoffChange: func(hz float64) {
			fs.hpfCutoff = hz
			log.hpfCutoffChanges = append(log.hpfCutoffChanges, hz)
		},
		OnLPFCutoffChange: func(hz float64) {
			fs.lpfCutoff = hz
			log.lpfCutoffChanges = append(log.lpfCutoffChanges, hz)
		},
	}
	z := NewEQPanelZone(cb)
	return z, log, fs
}

// TestMobileCurveHandleHitAreaExists verifies that the "eq-curve-area" hit area
// exists on mobile (not just desktop).
func TestMobileCurveHandleHitAreaExists(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	var found bool
	for _, a := range z.HitAreas() {
		if a.Tag == "eq-curve-area" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'eq-curve-area' hit area on mobile layout")
	}
}

// TestMobileCurveHandleDragBandGain verifies that pressing and dragging a
// band handle on mobile changes the gain.
func TestMobileCurveHandleDragBandGain(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	var curveArea *HitArea
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &z.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area on mobile")
	}

	// Band 5 (1 kHz) center position.
	center := math.Sqrt(eqBandDefs[5].loHz * eqBandDefs[5].hiHz)
	hx := freqToX(center, z.rect)
	hy := gainDBToY(0, z.rect)

	result := curveArea.Handler.OnPress(hx, hy)
	if result == InputIgnored {
		t.Fatal("expected press on band 5 handle to be consumed on mobile")
	}

	// Drag upward.
	targetY := gainDBToY(6, z.rect)
	curveArea.Handler.OnDrag(hx, targetY)

	if z.bandGainsDB[5] < 4.0 {
		t.Errorf("expected gain > 4 dB after drag, got %.1f", z.bandGainsDB[5])
	}

	curveArea.Handler.OnRelease(hx, targetY)

	if len(log.gainChanges) == 0 {
		t.Error("expected OnGainChange during mobile drag")
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ on release")
	}
}

// TestMobileCurveHandleDragUsesLargerHitRadius verifies that a press just
// outside the desktop radius (12px) but inside the mobile radius (22px)
// still captures on mobile.
func TestMobileCurveHandleDragUsesLargerHitRadius(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	var curveArea *HitArea
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &z.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area on mobile")
	}

	center := math.Sqrt(eqBandDefs[5].loHz * eqBandDefs[5].hiHz)
	hx := freqToX(center, z.rect)
	hy := gainDBToY(0, z.rect)

	// Press 15px away — outside desktop radius (12) but inside mobile radius (22).
	result := curveArea.Handler.OnPress(hx+15, hy)
	if result == InputIgnored {
		t.Fatal("expected mobile hit radius to capture at 15px distance")
	}

	curveArea.Handler.OnRelease(hx+15, hy)
}

// TestDesktopHPFHandleDrag verifies HPF handle drag on desktop.
func TestDesktopHPFHandleDrag(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 200

	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	// Rebuild the curve with HPF included.
	z.curveDirty = true

	var curveArea *HitArea
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &z.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area")
	}

	// HPF handle position.
	hx := freqToX(200, z.rect)
	hy := z.curveYAtX(hx)

	result := curveArea.Handler.OnPress(hx, hy)
	if result == InputIgnored {
		t.Fatal("expected HPF handle press to be captured")
	}
	if z.curveDragFilter != "hpf" {
		t.Fatalf("expected curveDragFilter='hpf', got %q", z.curveDragFilter)
	}

	// Drag rightward (toward higher frequency).
	newX := freqToX(500, z.rect)
	curveArea.Handler.OnDrag(newX, hy)

	if len(log.hpfCutoffChanges) == 0 {
		t.Fatal("expected OnHPFCutoffChange callback during drag")
	}
	lastHz := log.hpfCutoffChanges[len(log.hpfCutoffChanges)-1]
	if lastHz < 300 || lastHz > 700 {
		t.Errorf("expected HPF cutoff near 500Hz after drag, got %.0f", lastHz)
	}

	// Release.
	curveArea.Handler.OnRelease(newX, hy)
	if z.curveDragFilter != "" {
		t.Errorf("expected curveDragFilter='' after release, got %q", z.curveDragFilter)
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ on release")
	}
}

// TestDesktopLPFHandleDrag verifies LPF handle drag on desktop.
func TestDesktopLPFHandleDrag(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.lpfEnabled = true
	fs.lpfCutoff = 10000

	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	z.curveDirty = true

	var curveArea *HitArea
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &z.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area")
	}

	// LPF handle position.
	lx := freqToX(10000, z.rect)
	ly := z.curveYAtX(lx)

	result := curveArea.Handler.OnPress(lx, ly)
	if result == InputIgnored {
		t.Fatal("expected LPF handle press to be captured")
	}
	if z.curveDragFilter != "lpf" {
		t.Fatalf("expected curveDragFilter='lpf', got %q", z.curveDragFilter)
	}

	// Drag leftward (toward lower frequency).
	newX := freqToX(5000, z.rect)
	curveArea.Handler.OnDrag(newX, ly)

	if len(log.lpfCutoffChanges) == 0 {
		t.Fatal("expected OnLPFCutoffChange callback during drag")
	}
	lastHz := log.lpfCutoffChanges[len(log.lpfCutoffChanges)-1]
	if lastHz < 3000 || lastHz > 7000 {
		t.Errorf("expected LPF cutoff near 5000Hz after drag, got %.0f", lastHz)
	}

	curveArea.Handler.OnRelease(newX, ly)
	if z.curveDragFilter != "" {
		t.Errorf("expected curveDragFilter='' after release, got %q", z.curveDragFilter)
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ on release")
	}
}

// TestMobileHPFHandleDrag verifies HPF handle drag on mobile.
func TestMobileHPFHandleDrag(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 200

	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update()

	z.curveDirty = true

	var curveArea *HitArea
	for i := range z.hitAreas {
		if z.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &z.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area on mobile")
	}

	hx := freqToX(200, z.rect)
	hy := z.curveYAtX(hx)

	result := curveArea.Handler.OnPress(hx, hy)
	if result == InputIgnored {
		t.Fatal("expected HPF handle press to be captured on mobile")
	}
	if z.curveDragFilter != "hpf" {
		t.Fatalf("expected curveDragFilter='hpf', got %q", z.curveDragFilter)
	}

	newX := freqToX(400, z.rect)
	curveArea.Handler.OnDrag(newX, hy)

	if len(log.hpfCutoffChanges) == 0 {
		t.Fatal("expected OnHPFCutoffChange callback during mobile drag")
	}

	curveArea.Handler.OnRelease(newX, hy)
}

// --- Integration tests: full DrumView → tree dispatch path ---

// TestMobileEQDragNotBlockedByTouchDeadZone verifies that on mobile with
// mobileEQMode=true, a touch in the EQ panel area is NOT blocked by the
// touch dead zone. Before the fix, rowsRect() extended to Bounds.Max.Y
// (because eqH=0 on mobile), so EQ touches triggered the dead zone,
// dragBlocked=true, and the eq-curve-area handler at z=130 never received
// the press.
func TestMobileEQDragNotBlockedByTouchDeadZone(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Add one row so the DrumView is non-empty.
	dv.Rows = append(dv.Rows, &DrumRow{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	})
	dv.Length = 8

	// Toggle to mobile EQ mode (hides rows, shows EQ full-screen).
	dv.mobileEQMode = true

	// Warm-up frame to initialize layout.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if dv.eqPanelZone == nil {
		t.Fatal("eqPanelZone not initialized after warm-up")
	}
	if dv.tree == nil {
		t.Fatal("tree not initialized after warm-up")
	}

	// Find the EQ curve area hit area and its band 5 handle position.
	var curveArea *HitArea
	for i := range dv.eqPanelZone.hitAreas {
		if dv.eqPanelZone.hitAreas[i].Tag == "eq-curve-area" {
			curveArea = &dv.eqPanelZone.hitAreas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area in EQ panel zone")
	}

	// Compute the center of band 5 handle in the curve area.
	center := math.Sqrt(eqBandDefs[5].loHz * eqBandDefs[5].hiHz)
	hx := freqToX(center, dv.eqPanelZone.rect)
	hy := gainDBToY(0, dv.eqPanelZone.rect)

	// Simulate a real mobile touch press on the EQ band handle.
	// SetInputForTest resets touchOverride, so set it AFTER.
	r := SetInputForTest(
		func() (int, int) { return hx, hy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(hx, hy)

	dv.Update()

	// The tree should have dispatched to the EQ curve handler.
	if !dv.tree.Capturing() {
		t.Fatal("expected tree to be capturing after touch on EQ band handle; " +
			"touch dead zone likely blocked dispatch")
	}
	if tag := dv.tree.CapturedTag(); tag != "eq-curve-area" {
		t.Fatalf("expected captured tag 'eq-curve-area', got %q", tag)
	}

	// The dead zone should NOT be active in mobileEQMode.
	if dv.anyDragActive() {
		// anyDragActive may be true due to eqCurveDragBand being set (which is
		// correct — the EQ curve handler captured). But it must NOT be true
		// because of the touch dead zone.
		if dv.eqCurveDragBand < 0 && dv.eqCurveDragFilter == "" {
			t.Fatal("anyDragActive is true but no EQ drag is active — " +
				"touch dead zone is incorrectly blocking in mobileEQMode")
		}
	}

	r()
}

// TestMobileEQDragTouchDeadZoneStillBlocksRows verifies that the touch dead
// zone still works correctly in normal row view (mobileEQMode=false). This is
// a regression guard: the fix must only disable the dead zone during EQ mode,
// not during normal row scrolling.
func TestMobileEQDragTouchDeadZoneStillBlocksRows(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))

	// Create rows so the DrumView has scrollable content.
	for i := 0; i < 10; i++ {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			Volume:     1.0,
		})
	}
	dv.Length = 8

	// mobileEQMode is false (default) — normal row view.
	dv.mobileEQMode = false

	// Warm-up frame.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	// Pick a point in the rows area (well below the header).
	ry := dv.Bounds.Min.Y + dv.headerH + 50

	// Simulate a real mobile touch press in the rows area.
	r := SetInputForTest(
		func() (int, int) { return W / 2, ry },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	SetTouchOverrideActiveForTest(true)
	SetTouchOverrideXYForTest(W/2, ry)

	dv.Update()

	// anyDragActive should be true because the touch dead zone fires.
	if !dv.anyDragActive() {
		t.Fatal("anyDragActive should be true for mobile touch in rows area " +
			"(touch dead zone should block to allow scroll disambiguation)")
	}

	r()
}

