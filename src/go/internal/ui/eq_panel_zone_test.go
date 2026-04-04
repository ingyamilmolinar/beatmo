//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// --- EQPanelZone test helpers ---

// newTestEQPanelZone creates an EQPanelZone suitable for testing.
// It wires up minimal callbacks and sets a non-empty rect.
func newTestEQPanelZone(rows []*DrumRow) (*EQPanelZone, *eqTestCallbackLog) {
	log := &eqTestCallbackLog{}
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
		ActiveRows: func() []*DrumRow { return rows },
	}
	z := NewEQPanelZone(cb)
	return z, log
}

// registerEQZone is a helper that creates a tree, registers the zone, sets
// portal, and runs initial layout.
func registerEQZone(z *EQPanelZone, rect image.Rectangle) *DrumViewTree {
	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z.SetPortal(tree.Portal())
	tree.RegisterZone(z, 130)
	tree.SetZoneRect("eq-panel", rect)
	return tree
}

type eqGainChange struct {
	band int
	db   float64
}

type eqTestCallbackLog struct {
	gainChanges      []eqGainChange
	muteToggles      []int
	channelChanges   []string
	hpfToggles       int
	lpfToggles       int
	applyCount       int
	hpfCutoffChanges []float64
	lpfCutoffChanges []float64
}

func noInputForTest() func() {
	return SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
}

// --- Zone interface tests ---

func TestEQPanelZoneID(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	if z.ID() != "eq-panel" {
		t.Errorf("expected ID 'eq-panel', got %q", z.ID())
	}
}

func TestEQPanelZoneLayoutSetsGeometry(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)

	// Zone should need layout initially.
	if !z.NeedsLayout() {
		t.Fatal("zone should need layout initially")
	}

	r := image.Rect(100, 400, 700, 580)
	z.Layout(r)

	// After layout, NeedsLayout should be false.
	if z.NeedsLayout() {
		t.Fatal("zone should not need layout after Layout()")
	}

	// HitAreas should be non-empty after layout.
	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("HitAreas should be non-empty after layout")
	}

	// All hit areas should be within the zone rect.
	for _, a := range areas {
		if a.Rect.Empty() {
			continue // some may be conditionally empty (mobile buttons)
		}
		if !a.Rect.In(r) && !a.Rect.Overlaps(r) {
			t.Errorf("hit area %q rect %v is outside zone rect %v", a.Tag, a.Rect, r)
		}
	}
}

func TestEQPanelZoneInvalidate(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	z.Layout(image.Rect(0, 400, 600, 580))

	if z.NeedsLayout() {
		t.Fatal("should not need layout after Layout()")
	}

	z.Invalidate()
	if !z.NeedsLayout() {
		t.Fatal("should need layout after Invalidate()")
	}
}

// --- Tree integration tests ---

func TestEQPanelZoneTreeLifecycle(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	// First update: layout + update should run.
	tree.Update()

	if z.NeedsLayout() {
		t.Error("zone should have been laid out by tree")
	}

	// Find any hit area and query the hit index at its center.
	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("zone should have hit areas after layout")
	}
	// Pick the first tab button — should always be present.
	toggleArea := findHitAreaByTagPrefix(areas, "eq-tab-0")
	if toggleArea == nil {
		t.Fatal("expected eq-tab-0 hit area")
	}
	cx := (toggleArea.Rect.Min.X + toggleArea.Rect.Max.X) / 2
	cy := (toggleArea.Rect.Min.Y + toggleArea.Rect.Max.Y) / 2
	idxAreas := tree.HitIndexRef().At(cx, cy)
	if len(idxAreas) == 0 {
		t.Errorf("expected hit areas at (%d,%d) in hit index after tree update", cx, cy)
	}
}

// --- Mute button test ---

func TestEQPanelZoneMuteButtonCallback(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	// Layout frame.
	tree.Update()

	// Find a mute button hit area.
	muteArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-mute-")
	if muteArea == nil {
		t.Fatal("expected 'eq-mute-*' hit area")
	}

	// Click inside the mute button.
	mx, my = (muteArea.Rect.Min.X+muteArea.Rect.Max.X)/2, (muteArea.Rect.Min.Y+muteArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()

	// Release.
	pressed = false
	tree.Update()

	if len(log.muteToggles) == 0 {
		t.Error("expected mute toggle callback after clicking mute button")
	}
}

// --- Channel button test ---

func TestEQPanelZoneChannelButton(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	rows := []*DrumRow{
		{Name: "Kick", Instrument: "kick"},
		{Name: "Snare", Instrument: "snare"},
	}
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	// Layout.
	tree.Update()

	// Find channel button hit area.
	chArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-channel-btn")
	if chArea == nil {
		t.Fatal("expected 'eq-channel-btn' hit area")
	}

	// Click channel button — should open dropdown as portal overlay.
	mx, my = (chArea.Rect.Min.X+chArea.Rect.Max.X)/2, (chArea.Rect.Min.Y+chArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()

	pressed = false
	tree.Update()

	// Portal should have the channel dropdown open.
	if tree.Portal().TopID() != "eq-channel-dropdown" {
		t.Errorf("expected portal to have 'eq-channel-dropdown' open, got %q", tree.Portal().TopID())
	}
}

// --- Toggle button test ---

func TestEQPanelZoneToggleButton(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	tree.Update()

	// Find Wave tab button (eq-tab-1, since AllPanelTabs = [EQ, Wave, Spectrum, Meters]).
	waveTabArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-tab-1")
	if waveTabArea == nil {
		t.Fatal("expected 'eq-tab-1' hit area")
	}

	if z.WaveformMode() {
		t.Fatal("should start in EQ mode (not waveform)")
	}

	// Click Wave tab.
	mx, my = (waveTabArea.Rect.Min.X+waveTabArea.Rect.Max.X)/2, (waveTabArea.Rect.Min.Y+waveTabArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if !z.WaveformMode() {
		t.Error("expected waveform mode after clicking Wave tab")
	}
}

// --- HPF/LPF button tests ---

func TestEQPanelZoneHPFButton(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	tree.Update()

	hpfArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-hpf-btn")
	if hpfArea == nil {
		t.Fatal("expected 'eq-hpf-btn' hit area")
	}

	mx, my = (hpfArea.Rect.Min.X+hpfArea.Rect.Max.X)/2, (hpfArea.Rect.Min.Y+hpfArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.hpfToggles == 0 {
		t.Error("expected HPF toggle callback")
	}
}

func TestEQPanelZoneLPFButton(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	tree.Update()

	lpfArea := findHitAreaByTagPrefix(z.HitAreas(), "eq-lpf-btn")
	if lpfArea == nil {
		t.Fatal("expected 'eq-lpf-btn' hit area")
	}

	mx, my = (lpfArea.Rect.Min.X+lpfArea.Rect.Max.X)/2, (lpfArea.Rect.Min.Y+lpfArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.lpfToggles == 0 {
		t.Error("expected LPF toggle callback")
	}
}

// --- Responsive layout ---

func TestEQPanelZoneResponsiveLayout(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)

	// Desktop: wider rect.
	desktopRect := image.Rect(0, 400, 800, 580)
	z.Layout(desktopRect)
	desktopAreas := z.HitAreas()

	// Should have curve area hit area on desktop (sliders removed).
	curveArea := findHitAreaByTagPrefix(desktopAreas, "eq-curve-area")
	if curveArea == nil {
		t.Error("desktop layout should have 'eq-curve-area' hit area")
	}

	// Should NOT have slider group hit area.
	sliderArea := findHitAreaByTagPrefix(desktopAreas, "eq-slider-group")
	if sliderArea != nil {
		t.Error("desktop layout should NOT have 'eq-slider-group' hit area")
	}
}

// --- Keyboard ---

func TestEQPanelZoneHandleKeyIgnored(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	// EQ panel doesn't handle keyboard — should return InputIgnored.
	if r := z.HandleKey(ebiten.KeyEnter); r != InputIgnored {
		t.Errorf("expected InputIgnored for key, got %d", r)
	}
}

func TestEQPanelZoneHandleCharsIgnored(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	if r := z.HandleChars([]rune{'a'}); r != InputIgnored {
		t.Errorf("expected InputIgnored for chars, got %d", r)
	}
}

// --- Curve handle drag tests ---

// TestEQPanelZoneCurveHandleDragSyncsSlider verifies that dragging an EQ
// band handle on the curve updates the gain and fires OnGainChange on release.
func TestEQPanelZoneCurveHandleDragSyncsSlider(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)
	tree.Update() // trigger layout

	// Find the "eq-curve-area" hit area.
	areas := z.HitAreas()
	var curveArea *HitArea
	for i := range areas {
		if areas[i].Tag == "eq-curve-area" {
			curveArea = &areas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area to exist on desktop")
	}

	// Band 5 (1 kHz) center position.
	center := math.Sqrt(eqBandDefs[5].loHz * eqBandDefs[5].hiHz)
	hx := freqToX(center, z.rect)
	hy := gainDBToY(0, z.rect) // starts at 0 dB

	// Press on the handle.
	result := curveArea.Handler.OnPress(hx, hy)
	if result == InputIgnored {
		t.Fatal("expected press on band 5 handle to be consumed")
	}
	if z.curveDragBand != 5 {
		t.Fatalf("expected curveDragBand=5, got %d", z.curveDragBand)
	}

	// Drag upward (toward +6 dB).
	targetY := gainDBToY(6, z.rect)
	curveArea.Handler.OnDrag(hx, targetY)

	// Gain should be updated.
	if z.bandGainsDB[5] < 4.0 {
		t.Errorf("expected gain > 4 dB after drag, got %.1f", z.bandGainsDB[5])
	}

	// Curve should be marked dirty.
	if !z.curveDirty {
		t.Error("expected curveDirty after drag")
	}

	// Release.
	curveArea.Handler.OnRelease(hx, targetY)
	if z.curveDragBand != -1 {
		t.Errorf("expected curveDragBand=-1 after release, got %d", z.curveDragBand)
	}

	// OnGainChange should have been called during drag.
	if len(log.gainChanges) == 0 {
		t.Error("expected OnGainChange to be called during drag")
	}

	// OnApplyEQ should have been called on release.
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ to be called on release")
	}
}

// --- dB text input tests ---

// focusDBInput simulates clicking inside a dB TextInput to focus it, then
// runs one frame of updateDBInputs to trigger the focus-gained path.
func focusDBInput(t *testing.T, z *EQPanelZone, band int) func() {
	t.Helper()
	ti := z.eqDBInputs[band]
	if ti == nil {
		t.Fatalf("eqDBInputs[%d] is nil", band)
	}
	r := ti.Rect
	if r.Empty() {
		// Force a rect for testing.
		r = image.Rect(10+band*50, 500, 50+band*50, 520)
		ti.Rect = r
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	z.updateDBInputs() // ti.Update() sees click inside rect -> focused=true, then focus-gained runs
	return restore
}

func TestEQPanelZone_DBInputFocusGainedClearsText(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Set a non-zero gain so we can verify dbInputPrev is saved.
	z.bandGainsDB[3] = 4.5
	z.syncDBInputText(3)

	// Click inside band 3 dB input to trigger focus-gained.
	restore = focusDBInput(t, z, 3)
	restore()

	if z.dbInputPrev != 4.5 {
		t.Errorf("expected dbInputPrev=4.5, got %v", z.dbInputPrev)
	}
	ti := z.eqDBInputs[3]
	if ti.Value() != "" {
		t.Errorf("expected text cleared on focus, got %q", ti.Value())
	}
	if z.dbInputFocused != 3 {
		t.Errorf("expected dbInputFocused=3, got %d", z.dbInputFocused)
	}
}

func TestEQPanelZone_DBInputCommitValid(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Direct test of commitDBText: set text, call commit.
	ti := z.eqDBInputs[2]
	ti.SetText("6.0")
	z.commitDBText(2)

	if z.bandGainsDB[2] != 6.0 {
		t.Errorf("expected gain=6.0, got %v", z.bandGainsDB[2])
	}
	if len(log.gainChanges) == 0 || log.gainChanges[len(log.gainChanges)-1].db != 6.0 {
		t.Error("expected OnGainChange(2, 6.0)")
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired")
	}
	_ = tree
}

func TestEQPanelZone_DBInputCommitInvalid(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.bandGainsDB[0] = 3.0
	z.dbInputPrev = 3.0 // simulate what focus-gained would save
	ti := z.eqDBInputs[0]
	ti.SetText("abc")
	z.commitDBText(0)

	// Should revert to formatted previous value.
	if ti.Value() != "+3.0" {
		t.Errorf("expected revert to '+3.0', got %q", ti.Value())
	}
	if len(log.gainChanges) != 0 {
		t.Error("expected no OnGainChange for invalid input")
	}
	_ = tree
}

func TestEQPanelZone_DBInputCommitEmpty(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.bandGainsDB[1] = -2.0
	z.dbInputPrev = -2.0
	ti := z.eqDBInputs[1]
	ti.SetText("")
	z.commitDBText(1)

	if ti.Value() != "-2.0" {
		t.Errorf("expected revert to '-2.0', got %q", ti.Value())
	}
	if len(log.gainChanges) != 0 {
		t.Error("expected no OnGainChange for empty input")
	}
	_ = tree
}

func TestEQPanelZone_DBInputClamp(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	ti := z.eqDBInputs[4]
	ti.SetText("20.0")
	z.commitDBText(4)

	if z.bandGainsDB[4] != 12.0 {
		t.Errorf("expected clamped to 12.0, got %v", z.bandGainsDB[4])
	}
	if len(log.gainChanges) == 0 || log.gainChanges[len(log.gainChanges)-1].db != 12.0 {
		t.Error("expected OnGainChange with clamped value 12.0")
	}
	_ = tree
}

func TestEQPanelZone_DBInputOnlyOneFocused(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Focus band 3 via simulated click.
	z.bandGainsDB[3] = 1.0
	z.syncDBInputText(3)
	restore = focusDBInput(t, z, 3)
	restore()

	// Set text while focused.
	z.eqDBInputs[3].SetText("5.0")

	// Now focus band 5 — band 3 should be committed and unfocused.
	restore = focusDBInput(t, z, 5)
	restore()

	if z.eqDBInputs[3].Focused() {
		t.Error("expected band 3 unfocused")
	}
	if z.dbInputFocused != 5 {
		t.Errorf("expected dbInputFocused=5, got %d", z.dbInputFocused)
	}
	// Band 3 should have been committed with value 5.0.
	if z.bandGainsDB[3] != 5.0 {
		t.Errorf("expected band 3 gain=5.0, got %v", z.bandGainsDB[3])
	}
	if len(log.gainChanges) == 0 {
		t.Error("expected OnGainChange fired for committed band 3")
	}
	_ = tree
}

func TestEQPanelZone_SyncDBInputText(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	gains := make([]float64, 10)
	gains[0] = 3.5
	gains[4] = -6.0
	gains[9] = 12.0
	z.SyncBandState(gains, nil)

	if z.eqDBInputs[0].Value() != "+3.5" {
		t.Errorf("band 0: expected '+3.5', got %q", z.eqDBInputs[0].Value())
	}
	if z.eqDBInputs[4].Value() != "-6.0" {
		t.Errorf("band 4: expected '-6.0', got %q", z.eqDBInputs[4].Value())
	}
	if z.eqDBInputs[9].Value() != "+12.0" {
		t.Errorf("band 9: expected '+12.0', got %q", z.eqDBInputs[9].Value())
	}
	if !z.curveDirty {
		t.Error("expected curveDirty after SyncBandState")
	}
	_ = tree
}

func TestEQPanelZone_HandleKeyEnterCommits(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Set up focused state directly (HandleKey doesn't call ti.Update).
	ti := z.eqDBInputs[6]
	ti.SetText("8.5")
	ti.focused = true
	z.dbInputFocused = 6

	result := z.HandleKey(ebiten.KeyEnter)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}
	if z.bandGainsDB[6] != 8.5 {
		t.Errorf("expected gain=8.5, got %v", z.bandGainsDB[6])
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired")
	}
	_ = tree
}

func TestEQPanelZone_HandleKeyEscapeReverts(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.bandGainsDB[7] = 2.5
	z.dbInputPrev = 2.5 // simulate what focus-gained would save
	ti := z.eqDBInputs[7]
	ti.SetText("10.0")
	ti.focused = true
	z.dbInputFocused = 7

	result := z.HandleKey(ebiten.KeyEscape)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}
	if ti.Value() != "+2.5" {
		t.Errorf("expected revert to '+2.5', got %q", ti.Value())
	}
	if ti.Focused() {
		t.Error("expected unfocused after Escape")
	}
	if z.dbInputFocused != -1 {
		t.Errorf("expected dbInputFocused=-1, got %d", z.dbInputFocused)
	}
	// Escape should NOT fire OnGainChange or OnApplyEQ.
	if len(log.gainChanges) != 0 {
		t.Error("expected no OnGainChange on Escape")
	}
	_ = tree
}

// --- Channel dropdown scroll handler tests ---

func TestEQPanelZone_ChannelDropdownWheelScroll(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := makeOverflowRows(10) // 11 total items (Master + 10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.eqChannelBtn.OnClick()

	if !z.channelScroll.HasScroll() {
		t.Skip("dropdown does not overflow with 11 items in this layout")
	}

	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()
	handler := &scrollableDropdownHandler{overlay: overlay}

	firstBefore := z.channelScroll.VS.First

	// Wheel scroll down (negative steps).
	result := handler.OnWheel(100, 500, -3)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}
	if z.channelScroll.VS.First == firstBefore {
		t.Error("expected scroll position to change after wheel")
	}
	_ = tree
}

func TestEQPanelZone_ChannelDropdownScrollbarDrag(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := makeOverflowRows(10)
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.eqChannelBtn.OnClick()

	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()
	handler := &scrollableDropdownHandler{overlay: overlay}

	thumb := z.channelScroll.ThumbRect()
	if thumb.Empty() {
		t.Skip("no scrollbar thumb with this layout")
	}

	thumbCX := (thumb.Min.X + thumb.Max.X) / 2
	thumbCY := (thumb.Min.Y + thumb.Max.Y) / 2

	result := handler.OnPress(thumbCX, thumbCY)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", result)
	}
	handler.OnDrag(thumbCX, thumbCY+30)
	handler.OnRelease(thumbCX, thumbCY+30)

	if z.channelScroll.Dragging() {
		t.Error("expected dragging to stop after release")
	}
	_ = tree
}

func TestEQPanelZone_ChannelDropdownTouchScrollCancelsTap(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := makeOverflowRows(10)
	z, log := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.eqChannelBtn.OnClick()

	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()
	handler := &scrollableDropdownHandler{overlay: overlay}

	if overlay.rect.Empty() {
		t.Skip("no overlay rect")
	}

	cx := (overlay.rect.Min.X + overlay.rect.Max.X) / 2
	cy := overlay.rect.Min.Y + 5

	// Touch start in menu area.
	result := handler.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", result)
	}

	// Drag past dead zone to commit scroll.
	for i := 1; i <= 20; i++ {
		handler.OnDrag(cx, cy+i*3)
	}

	channelsBefore := len(log.channelChanges)
	handler.OnRelease(cx, cy+60)

	// No channel change should fire — scroll cancels tap.
	if len(log.channelChanges) != channelsBefore {
		t.Error("expected no channel change after scroll (tap should be cancelled)")
	}
	_ = tree
}

func TestEQPanelZone_ChannelDropdownDeferredTap(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := []*DrumRow{{Name: "Kick", Instrument: "kick"}}
	z, log := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	z.eqChannelBtn.OnClick()

	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()
	handler := &scrollableDropdownHandler{overlay: overlay}

	if len(overlay.buttons) == 0 {
		t.Fatal("expected at least one button")
	}

	// Find the first button's center.
	btn := overlay.buttons[0]
	btnR := btn.Rect()
	bx := (btnR.Min.X + btnR.Max.X) / 2
	by := (btnR.Min.Y + btnR.Max.Y) / 2

	// Touch start, stay still, release — should fire deferred tap.
	handler.OnPress(bx, by)
	handler.OnRelease(bx, by)

	if len(log.channelChanges) == 0 {
		t.Error("expected OnChannelChange after deferred tap on button")
	}
	_ = tree
}

func TestEQPanelZone_ChannelDropdownSelectMaster(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := []*DrumRow{{Name: "Kick", Instrument: "kick"}}
	z, log := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Switch to a non-master channel first.
	z.activeChannel = "kick"
	z.eqChannelBtn.Text = "Kick"

	z.eqChannelBtn.OnClick() // open dropdown

	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()

	// First button is "Master".
	if len(overlay.buttons) == 0 {
		t.Fatal("expected buttons")
	}
	masterBtn := overlay.buttons[0]
	if masterBtn.Text != "Master" {
		t.Fatalf("expected first button 'Master', got %q", masterBtn.Text)
	}

	masterBtn.OnClick()

	if len(log.channelChanges) == 0 || log.channelChanges[0] != "main" {
		t.Error("expected OnChannelChange('main')")
	}
	if z.channelOpen {
		t.Error("expected dropdown closed after selection")
	}
	if z.activeChannel != "main" {
		t.Errorf("expected activeChannel='main', got %q", z.activeChannel)
	}
	_ = tree
}

// --- HPF/LPF curve handle drag tests ---

func TestEQPanelZone_HPFHandleDrag(t *testing.T) {
	assertDefaultParityState(t)

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 100.0
	fs.lpfEnabled = true
	fs.lpfCutoff = 15000.0
	tree := registerEQZone(z, image.Rect(0, 100, 600, 400))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Compute HPF handle position (100 Hz).
	r := z.rect
	hx := freqToX(100.0, r)
	hy := z.curveYAtX(hx)

	adapter := &curveHandleHitAdapter{zone: z}
	result := adapter.OnPress(hx, hy)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured on HPF handle, got %d", result)
	}
	if z.curveDragFilter != "hpf" {
		t.Fatalf("expected curveDragFilter='hpf', got %q", z.curveDragFilter)
	}

	// Drag to a new X position (higher frequency).
	newX := freqToX(500.0, r)
	adapter.OnDrag(newX, hy)

	if len(log.hpfCutoffChanges) == 0 {
		t.Error("expected OnHPFCutoffChange fired during drag")
	}
	if z.curveDragDBText == "" {
		t.Error("expected drag label text to be set")
	}

	adapter.OnRelease(newX, hy)
	if z.curveDragFilter != "" {
		t.Error("expected curveDragFilter cleared after release")
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired on release")
	}
}

func TestEQPanelZone_LPFHandleDrag(t *testing.T) {
	assertDefaultParityState(t)

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 100.0
	fs.lpfEnabled = true
	fs.lpfCutoff = 15000.0
	tree := registerEQZone(z, image.Rect(0, 100, 600, 400))

	restore := noInputForTest()
	tree.Update()
	restore()

	r := z.rect
	lx := freqToX(15000.0, r)
	ly := z.curveYAtX(lx)

	adapter := &curveHandleHitAdapter{zone: z}
	result := adapter.OnPress(lx, ly)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured on LPF handle, got %d", result)
	}
	if z.curveDragFilter != "lpf" {
		t.Fatalf("expected curveDragFilter='lpf', got %q", z.curveDragFilter)
	}

	newX := freqToX(5000.0, r)
	adapter.OnDrag(newX, ly)

	if len(log.lpfCutoffChanges) == 0 {
		t.Error("expected OnLPFCutoffChange fired during drag")
	}

	adapter.OnRelease(newX, ly)
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ on release")
	}
	_ = tree
}

func TestEQPanelZone_FilterDragReleaseClears(t *testing.T) {
	assertDefaultParityState(t)

	z, _, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 100.0
	fs.lpfEnabled = true
	fs.lpfCutoff = 15000.0
	tree := registerEQZone(z, image.Rect(0, 100, 600, 400))

	restore := noInputForTest()
	tree.Update()
	restore()

	r := z.rect
	hx := freqToX(100.0, r)
	hy := z.curveYAtX(hx)

	adapter := &curveHandleHitAdapter{zone: z}
	adapter.OnPress(hx, hy)

	newX := freqToX(300.0, r)
	adapter.OnDrag(newX, hy)
	if z.curveDragDBText == "" {
		t.Error("expected drag label during drag")
	}

	adapter.OnRelease(newX, hy)

	if z.curveDragDBText != "" {
		t.Errorf("expected drag label cleared after release, got %q", z.curveDragDBText)
	}
	if z.curveDragFilter != "" {
		t.Errorf("expected curveDragFilter cleared, got %q", z.curveDragFilter)
	}
	_ = tree
}

// --- Helper ---

func findHitAreaByTagPrefix(areas []HitArea, prefix string) *HitArea {
	for i := range areas {
		if len(areas[i].Tag) >= len(prefix) && areas[i].Tag[:len(prefix)] == prefix {
			return &areas[i]
		}
	}
	return nil
}
