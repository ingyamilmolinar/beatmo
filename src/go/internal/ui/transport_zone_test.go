//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- TransportZone test helpers ---

type transportTestCallbackLog struct {
	playToggles   int
	stops         int
	bpmChanges    []int
	followChanges []bool
	uploadClicks  int
	importClicks  int
	exportClicks  int
	viewCycles    int
	subdivClicks  int
	overflowOpens int
	volClicks     int
	volSets       []float64
}

func newTestTransportZone() (*TransportZone, *transportTestCallbackLog) {
	log := &transportTestCallbackLog{}
	cb := TransportCallbacks{
		OnPlayToggle: func() {
			log.playToggles++
		},
		OnStop: func() {
			log.stops++
		},
		OnBPMChange: func(bpm int) {
			log.bpmChanges = append(log.bpmChanges, bpm)
		},
		OnFollowChange: func(follow bool) {
			log.followChanges = append(log.followChanges, follow)
		},
		OnUploadClick: func() {
			log.uploadClicks++
		},
		OnImportClick: func() {
			log.importClicks++
		},
		OnExportClick: func() {
			log.exportClicks++
		},
		OnViewCycle: func() {
			log.viewCycles++
		},
		OnSubdivClick: func() {
			log.subdivClicks++
		},
		OnOverflowOpen: func() {
			log.overflowOpens++
		},
		OnMasterVolClick: func() {
			log.volClicks++
		},
		IsPlaying:     func() bool { return false },
		GetMainVolume: func() float64 { return 0.5 },
		SetMainVolume: func(v float64) {
			log.volSets = append(log.volSets, v)
		},
	}
	z := NewTransportZone(cb)
	return z, log
}

func registerTransportZone(z *TransportZone, rect image.Rectangle) *DrumViewTree {
	tree := NewDrumViewTree()
	tree.SetBounds(image.Rect(0, 0, 800, 600))
	z.SetPortal(tree.Portal())
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("transport", rect)
	return tree
}

// --- Zone interface tests ---

func TestTransportZoneID(t *testing.T) {
	z, _ := newTestTransportZone()
	if z.ID() != "transport" {
		t.Errorf("expected ID 'transport', got %q", z.ID())
	}
}

func TestTransportZoneLayoutSetsGeometry(t *testing.T) {
	z, _ := newTestTransportZone()

	if !z.NeedsLayout() {
		t.Fatal("zone should need layout initially")
	}

	r := image.Rect(10, 10, 500, 90)
	z.Layout(r)

	if z.NeedsLayout() {
		t.Fatal("zone should not need layout after Layout()")
	}

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("HitAreas should be non-empty after layout")
	}

	// All hit areas should overlap with the zone rect.
	for _, a := range areas {
		if a.Rect.Empty() {
			continue
		}
		if !a.Rect.Overlaps(r) {
			t.Errorf("hit area %q rect %v does not overlap zone rect %v", a.Tag, a.Rect, r)
		}
	}
}

func TestTransportZone_VolSliderTouchFlag(t *testing.T) {
	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	volArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-vol-slider")
	if volArea == nil {
		t.Skip("no volume slider hit area (may be hidden on mobile layout)")
	}
	if !volArea.Touch {
		t.Fatal("transport-vol-slider HitArea must have Touch == true for mobile touch expansion")
	}
}

func TestTransportZoneInvalidate(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 600, 80))

	if z.NeedsLayout() {
		t.Fatal("should not need layout after Layout()")
	}

	z.Invalidate()
	if !z.NeedsLayout() {
		t.Fatal("should need layout after Invalidate()")
	}
}

// --- Button callback tests ---

func TestTransportZonePlayButtonCallback(t *testing.T) {
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

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))

	// Frame 1: layout.
	tree.Update()

	playArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-play")
	if playArea == nil {
		t.Fatal("expected 'transport-play' hit area")
	}

	// Click play button.
	mx, my = (playArea.Rect.Min.X+playArea.Rect.Max.X)/2, (playArea.Rect.Min.Y+playArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()

	pressed = false
	tree.Update()

	if log.playToggles == 0 {
		t.Error("expected OnPlayToggle callback after clicking play button")
	}
	if !z.PlayPressed() {
		t.Error("expected PlayPressed() to return true after click")
	}
}

func TestTransportZoneStopButtonCallback(t *testing.T) {
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

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	stopArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-stop")
	if stopArea == nil {
		t.Fatal("expected 'transport-stop' hit area")
	}

	mx, my = (stopArea.Rect.Min.X+stopArea.Rect.Max.X)/2, (stopArea.Rect.Min.Y+stopArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.stops == 0 {
		t.Error("expected OnStop callback after clicking stop button")
	}
}

// --- BPM repeat capture test ---

func TestTransportZoneBPMRepeatCapture(t *testing.T) {
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

	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	incArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-bpm-inc")
	if incArea == nil {
		t.Fatal("expected 'transport-bpm-inc' hit area")
	}

	initialBPM := z.BPM()

	// Press BPM+ → should capture input.
	mx, my = (incArea.Rect.Min.X+incArea.Rect.Max.X)/2, (incArea.Rect.Min.Y+incArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()

	if !tree.Capturing() {
		t.Error("tree should be capturing after BPM+ press (repeat button)")
	}

	// Hold for a few frames to accumulate delta.
	tree.Update()
	tree.Update()

	// Release.
	pressed = false
	tree.Update()

	// BPM should have increased (delta accumulated during hold, applied by zone Update).
	if z.BPM() <= initialBPM {
		t.Errorf("BPM should have increased from %d, got %d", initialBPM, z.BPM())
	}
}

// --- Volume slider capture test ---

func TestTransportZoneVolumeSliderCapture(t *testing.T) {
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

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	volArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-vol-slider")
	if volArea == nil {
		t.Skip("no volume slider hit area (may be hidden on mobile layout)")
	}

	// Press at the center of the slider area.
	sr := z.mainVolSlider.Rect()
	if sr.Empty() {
		t.Skip("slider rect is empty (mobile layout)")
	}
	mx, my = sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2
	pressed = true
	tree.Update()

	if !tree.Capturing() {
		t.Error("tree should be capturing after slider press")
	}

	// Drag to the right.
	mx = sr.Max.X - 2
	tree.Update()

	// Release.
	pressed = false
	tree.Update()

	if tree.Capturing() {
		t.Error("tree should not be capturing after release")
	}

	if len(log.volSets) == 0 {
		t.Error("expected SetMainVolume callback after slider interaction")
	}
}

// --- Responsive layout test ---

func TestTransportZoneResponsiveLayout(t *testing.T) {
	z, _ := newTestTransportZone()

	// Desktop layout.
	desktopRect := image.Rect(10, 10, 500, 90)
	z.Layout(desktopRect)
	desktopAreas := make([]HitArea, len(z.HitAreas()))
	copy(desktopAreas, z.HitAreas())

	// Should have upload button visible on desktop.
	uploadArea := findHitAreaByTagPrefix(desktopAreas, "transport-upload")
	if uploadArea == nil {
		t.Error("desktop layout should have 'transport-upload' hit area")
	}

	// Mobile layout: vol-icon / view-switch / overflow now live in
	// DrumView.bottomActionBarRect (B3 critique). The transport zone
	// alone does NOT register hit areas for them on mobile — DrumView
	// places them after zone Layout completes and re-runs hit-area
	// registration. Asserting the new contract here.
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	// Re-init buttons for mobile styles (constructor checks isSmallScreen).
	zm, _ := newTestTransportZone()
	mobileRect := image.Rect(0, 0, 400, 120)
	zm.Layout(mobileRect)
	mobileAreas := zm.HitAreas()

	if a := findHitAreaByTagPrefix(mobileAreas, "transport-overflow"); a != nil {
		t.Error("zone-only mobile layout should NOT register transport-overflow; DrumView places it in bottom action bar")
	}
	if a := findHitAreaByTagPrefix(mobileAreas, "transport-view-switch"); a != nil {
		t.Error("zone-only mobile layout should NOT register transport-view-switch; DrumView places it in bottom action bar")
	}

	// Upload should be hidden on mobile.
	uploadMobile := findHitAreaByTagPrefix(mobileAreas, "transport-upload")
	if uploadMobile != nil {
		t.Error("upload should be hidden on mobile layout")
	}
}

// --- BPM text input test ---

func TestTransportZoneBPMTextInput(t *testing.T) {
	z, log := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	// Simulate typing into BPM box.
	z.bpmBox.focused = true
	z.bpmPrev = 120
	z.bpmBox.SetText("150")

	// HandleKey(Enter) should commit the BPM.
	result := z.HandleKey(ebiten.KeyEnter)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from HandleKey(Enter), got %d", result)
	}

	if z.BPM() != 150 {
		t.Errorf("expected BPM 150 after commit, got %d", z.BPM())
	}

	if z.bpmBox.Focused() {
		t.Error("bpmBox should be unfocused after Enter commit")
	}

	if len(log.bpmChanges) == 0 {
		t.Error("expected OnBPMChange callback after BPM commit")
	}
}

func TestTransportZoneBPMTextInputInvalid(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	z.SetBPM(120)
	z.bpmBox.focused = true
	z.bpmPrev = 120
	z.bpmBox.SetText("abc")

	z.HandleKey(ebiten.KeyEnter)

	// Should revert to previous BPM.
	if z.BPM() != 120 {
		t.Errorf("expected BPM 120 after invalid input, got %d", z.BPM())
	}

	if z.bpmErrorAnim <= 0 {
		t.Error("expected bpmErrorAnim > 0 after invalid input")
	}
}

func TestTransportZoneBPMEscape(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	z.SetBPM(120)
	z.bpmBox.focused = true
	z.bpmPrev = 120
	z.bpmBox.SetText("999")

	result := z.HandleKey(ebiten.KeyEscape)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from HandleKey(Escape), got %d", result)
	}

	// Should revert to previous BPM.
	if z.BPM() != 120 {
		t.Errorf("expected BPM 120 after Escape, got %d", z.BPM())
	}

	if z.bpmBox.Focused() {
		t.Error("bpmBox should be unfocused after Escape")
	}
}

// --- Tree lifecycle test ---

func TestTransportZoneTreeLifecycle(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))

	tree.Update()

	if z.NeedsLayout() {
		t.Error("zone should have been laid out by tree")
	}

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("zone should have hit areas after layout")
	}

	// Pick the play button — should always be present.
	playArea := findHitAreaByTagPrefix(areas, "transport-play")
	if playArea == nil {
		t.Fatal("expected transport-play hit area")
	}
	cx := (playArea.Rect.Min.X + playArea.Rect.Max.X) / 2
	cy := (playArea.Rect.Min.Y + playArea.Rect.Max.Y) / 2
	idxAreas := tree.HitIndexRef().At(cx, cy)
	if len(idxAreas) == 0 {
		t.Errorf("expected hit areas at (%d,%d) in hit index after tree update", cx, cy)
	}
}

// --- State accessor tests ---

func TestTransportZoneSetPlaying(t *testing.T) {
	z, _ := newTestTransportZone()
	z.SetPlaying(true)
	if z.playBtn.Icon != "pause" {
		t.Errorf("expected icon 'pause' when playing, got %q", z.playBtn.Icon)
	}
	z.SetPlaying(false)
	if z.playBtn.Icon != "play" {
		t.Errorf("expected icon 'play' when stopped, got %q", z.playBtn.Icon)
	}
}

func TestTransportZoneSetFollow(t *testing.T) {
	z, log := newTestTransportZone()
	z.SetFollow(false)
	if z.FollowPlayback() {
		t.Error("expected FollowPlayback() false after SetFollow(false)")
	}
	if z.trackBtn.Icon != "track-off" {
		t.Errorf("expected icon 'track-off', got %q", z.trackBtn.Icon)
	}
	if len(log.followChanges) == 0 || log.followChanges[len(log.followChanges)-1] != false {
		t.Error("expected OnFollowChange(false) callback")
	}
}

func TestTransportZoneHandleKeyIgnoredWhenUnfocused(t *testing.T) {
	z, _ := newTestTransportZone()
	// BPM box is not focused — HandleKey should return InputIgnored.
	if r := z.HandleKey(ebiten.KeyEnter); r != InputIgnored {
		t.Errorf("expected InputIgnored for key when unfocused, got %d", r)
	}
}

func TestTransportZoneHandleCharsIgnored(t *testing.T) {
	z, _ := newTestTransportZone()
	if r := z.HandleChars([]rune{'1'}); r != InputIgnored {
		t.Errorf("expected InputIgnored for chars, got %d", r)
	}
}

// --- Update() and animation decay tests ---

func TestTransportZoneUpdateDecaysAnimations(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update() // initial layout

	// Set all animation values to 1.0.
	z.playAnim = 1.0
	z.stopAnim = 1.0
	z.bpmDecAnim = 1.0
	z.bpmIncAnim = 1.0
	z.uploadAnim = 1.0
	z.bpmErrorAnim = 1.0

	// Run a few Update() frames via tree.
	tree.Update()

	// After one decay, values should be 0.85 (since 1.0 * 0.85 = 0.85).
	if z.playAnim < 0.84 || z.playAnim > 0.86 {
		t.Errorf("expected playAnim ~0.85, got %f", z.playAnim)
	}
	if z.stopAnim < 0.84 || z.stopAnim > 0.86 {
		t.Errorf("expected stopAnim ~0.85, got %f", z.stopAnim)
	}
	if z.bpmDecAnim < 0.84 || z.bpmDecAnim > 0.86 {
		t.Errorf("expected bpmDecAnim ~0.85, got %f", z.bpmDecAnim)
	}
	if z.bpmIncAnim < 0.84 || z.bpmIncAnim > 0.86 {
		t.Errorf("expected bpmIncAnim ~0.85, got %f", z.bpmIncAnim)
	}
	if z.uploadAnim < 0.84 || z.uploadAnim > 0.86 {
		t.Errorf("expected uploadAnim ~0.85, got %f", z.uploadAnim)
	}
	if z.bpmErrorAnim < 0.84 || z.bpmErrorAnim > 0.86 {
		t.Errorf("expected bpmErrorAnim ~0.85, got %f", z.bpmErrorAnim)
	}

	// Run many more frames — animations should decay to 0.
	for i := 0; i < 100; i++ {
		tree.Update()
	}
	if z.playAnim != 0 {
		t.Errorf("expected playAnim to decay to 0, got %f", z.playAnim)
	}
	if z.bpmErrorAnim != 0 {
		t.Errorf("expected bpmErrorAnim to decay to 0, got %f", z.bpmErrorAnim)
	}
}

func TestTransportZoneUpdateAppliesBPMDelta(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update() // initial layout

	z.SetBPM(100)
	log.bpmChanges = nil // reset

	// Manually set bpmDelta as if +/- buttons accumulated it.
	z.bpmDelta = 5
	tree.Update()

	if z.BPM() != 105 {
		t.Errorf("expected BPM 105 after delta +5, got %d", z.BPM())
	}
	if z.bpmDelta != 0 {
		t.Errorf("expected bpmDelta reset to 0, got %d", z.bpmDelta)
	}
	if len(log.bpmChanges) == 0 {
		t.Error("expected OnBPMChange callback after delta applied")
	}

	// Negative delta.
	log.bpmChanges = nil
	z.bpmDelta = -10
	tree.Update()

	if z.BPM() != 95 {
		t.Errorf("expected BPM 95 after delta -10, got %d", z.BPM())
	}
}

func TestTransportZoneUpdateNoDeltaNoChange(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	z.SetBPM(120)
	log.bpmChanges = nil

	// No delta — Update should not change BPM.
	tree.Update()

	if z.BPM() != 120 {
		t.Errorf("expected BPM unchanged at 120, got %d", z.BPM())
	}
	if len(log.bpmChanges) != 0 {
		t.Error("expected no OnBPMChange when delta is 0")
	}
}

// --- commitBPMText() edge cases ---

func TestTransportZoneCommitBPMTextEmpty(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	z.SetBPM(100)
	z.bpmPrev = 100
	z.bpmBox.focused = true
	z.bpmBox.SetText("")

	z.HandleKey(ebiten.KeyEnter)

	// Empty text should revert to bpmPrev.
	if z.BPM() != 100 {
		t.Errorf("expected BPM 100 on empty commit, got %d", z.BPM())
	}
	if z.bpmBox.Focused() {
		t.Error("bpmBox should be unfocused after commit")
	}
}

func TestTransportZoneCommitBPMTextEmptyNoPrev(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	z.SetBPM(80)
	z.bpmPrev = 0 // no previous saved
	z.bpmBox.focused = true
	z.bpmBox.SetText("")

	z.HandleKey(ebiten.KeyEnter)

	// When bpmPrev < 1, should fall back to current bpm.
	if z.BPM() != 80 {
		t.Errorf("expected BPM 80 on empty commit with no prev, got %d", z.BPM())
	}
}

// --- syncTrackBtnVisual() tests ---

func TestTransportZoneSyncTrackBtnVisualFollow(t *testing.T) {
	z, _ := newTestTransportZone()
	z.follow = true
	z.syncTrackBtnVisual()

	if z.trackBtn.Icon != "track" {
		t.Errorf("expected icon 'track' when following, got %q", z.trackBtn.Icon)
	}
}

func TestTransportZoneSyncTrackBtnVisualNoFollow(t *testing.T) {
	z, _ := newTestTransportZone()
	z.follow = false
	z.syncTrackBtnVisual()

	if z.trackBtn.Icon != "track-off" {
		t.Errorf("expected icon 'track-off' when not following, got %q", z.trackBtn.Icon)
	}
}

// --- StopPressed() one-frame flag test ---

func TestTransportZoneStopPressedFlag(t *testing.T) {
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

	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	stopArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-stop")
	if stopArea == nil {
		t.Fatal("expected 'transport-stop' hit area")
	}

	// Click stop button.
	mx, my = (stopArea.Rect.Min.X+stopArea.Rect.Max.X)/2, (stopArea.Rect.Min.Y+stopArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	// First read should return true and clear the flag.
	if !z.StopPressed() {
		t.Error("expected StopPressed() true after click")
	}
	// Second read should return false (flag consumed).
	if z.StopPressed() {
		t.Error("expected StopPressed() false after consuming the flag")
	}
}

// --- Accessor tests ---

func TestTransportZoneBPMErrorAnimAccessor(t *testing.T) {
	z, _ := newTestTransportZone()
	if z.BPMErrorAnim() != 0 {
		t.Errorf("expected initial BPMErrorAnim 0, got %f", z.BPMErrorAnim())
	}
	z.bpmErrorAnim = 0.5
	if z.BPMErrorAnim() != 0.5 {
		t.Errorf("expected BPMErrorAnim 0.5, got %f", z.BPMErrorAnim())
	}
}

func TestTransportZoneMainVolSliderAccessor(t *testing.T) {
	z, _ := newTestTransportZone()
	if z.MainVolSlider() == nil {
		t.Fatal("expected non-nil MainVolSlider()")
	}
	if z.MainVolSlider() != z.mainVolSlider {
		t.Error("MainVolSlider() should return the internal slider")
	}
}

func TestTransportZoneMainVolGroupAccessor(t *testing.T) {
	z, _ := newTestTransportZone()
	if z.MainVolGroup() == nil {
		t.Fatal("expected non-nil MainVolGroup()")
	}
	if z.MainVolGroup() != z.mainVolGroup {
		t.Error("MainVolGroup() should return the internal group")
	}
}

func TestTransportZoneMainVolIconRectAccessor(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))
	// On desktop, mainVolIconRect should be set.
	r := z.MainVolIconRect()
	if r.Empty() {
		t.Skip("mainVolIconRect empty on this layout (ok for some configurations)")
	}
}

// --- SetBPM edge cases ---

func TestTransportZoneSetBPMClampLow(t *testing.T) {
	z, log := newTestTransportZone()

	z.SetBPM(0)
	if z.BPM() != 1 {
		t.Errorf("expected BPM clamped to 1, got %d", z.BPM())
	}
	if z.bpmErrorAnim <= 0 {
		t.Error("expected bpmErrorAnim > 0 after clamping to low bound")
	}
	if len(log.bpmChanges) == 0 || log.bpmChanges[len(log.bpmChanges)-1] != 1 {
		t.Error("expected OnBPMChange(1) after clamping")
	}
}

func TestTransportZoneSetBPMClampHigh(t *testing.T) {
	z, log := newTestTransportZone()

	z.SetBPM(9999)
	if z.BPM() != 1000 {
		t.Errorf("expected BPM clamped to 1000, got %d", z.BPM())
	}
	if z.bpmErrorAnim <= 0 {
		t.Error("expected bpmErrorAnim > 0 after clamping to high bound")
	}
	if len(log.bpmChanges) == 0 || log.bpmChanges[len(log.bpmChanges)-1] != 1000 {
		t.Error("expected OnBPMChange(1000) after clamping")
	}
}

// --- toolbarStateHash changes on state change ---

func TestTransportZoneToolbarStateHashChanges(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	h1 := z.toolbarStateHash()

	// Change playing state.
	z.SetPlaying(true)
	h2 := z.toolbarStateHash()
	if h1 == h2 {
		t.Error("hash should change when play state changes")
	}

	// Change BPM.
	z.SetBPM(200)
	h3 := z.toolbarStateHash()
	if h2 == h3 {
		t.Error("hash should change when BPM changes")
	}

	// Change follow.
	z.SetFollow(false)
	h4 := z.toolbarStateHash()
	if h3 == h4 {
		t.Error("hash should change when follow state changes")
	}
}

// --- toolbarBounds returns non-empty rect after layout ---

func TestTransportZoneToolbarBoundsAfterLayout(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(10, 10, 500, 90))

	bounds := z.toolbarBounds()
	if bounds.Empty() {
		t.Error("toolbarBounds() should not be empty after layout")
	}
}

// --- PlayPressed consumed on read ---

func TestTransportZonePlayPressedConsumed(t *testing.T) {
	z, _ := newTestTransportZone()
	// Not pressed — should be false.
	if z.PlayPressed() {
		t.Error("expected PlayPressed() false when not pressed")
	}

	// Manually set the flag.
	z.playPressed = true
	if !z.PlayPressed() {
		t.Error("expected PlayPressed() true when flag set")
	}
	// Second read should be consumed.
	if z.PlayPressed() {
		t.Error("expected PlayPressed() false after consuming")
	}
}

// --- P3 additional tests ---

// TestTransport_BPMFocusGainClearsText verifies that when the BPM text box
// gains focus during Update(), the text is cleared to "" and bpmPrev is saved.
func TestTransport_BPMFocusGainClearsText(t *testing.T) {
	z, _ := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))

	// No-input first frame for layout.
	restore := noInputForTest()
	tree.Update()
	restore()

	z.SetBPM(180)

	// BPM box is unfocused, text shows "180".
	if z.bpmBox.Focused() {
		t.Fatal("bpmBox should start unfocused")
	}

	// To trigger the focus-gain path in Update(), we need bpmBox.Update()
	// to transition focused from false to true. bpmBox.Update() sets
	// focused=true when mouse is pressed inside its Rect.
	bpmR := z.bpmBox.Rect
	if bpmR.Empty() {
		t.Fatal("bpmBox Rect is empty after layout")
	}
	cx := (bpmR.Min.X + bpmR.Max.X) / 2
	cy := (bpmR.Min.Y + bpmR.Max.Y) / 2

	// Set input: cursor inside bpmBox, mouse pressed.
	restore = SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	suppressClicksUntilRelease = false
	tree.Update() // bpmBox.Update() gains focus, then z.Update() sees the transition
	restore()

	if z.bpmPrev != 180 {
		t.Errorf("expected bpmPrev=180 after focus gain, got %d", z.bpmPrev)
	}
	if z.bpmBox.Value() != "" {
		t.Errorf("expected bpmBox text cleared to empty on focus gain, got %q", z.bpmBox.Value())
	}
}

// TestTransport_InputBlockedBlursBPM verifies that when inputBlocked returns
// true while the BPM box is focused, the box is auto-blurred and the value committed.
func TestTransport_InputBlockedBlursBPM(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update() // initial layout

	z.SetBPM(120)
	log.bpmChanges = nil

	// Focus the BPM box and type a new value.
	z.bpmBox.focused = true
	z.bpmPrev = 120
	z.bpmBox.SetText("160")

	// Set inputBlocked to return true (simulating a popup/overlay opening).
	z.SetInputBlocked(func() bool { return true })

	// Run Update() — should detect blocked + focused and call forceBlurBPM().
	z.Update()

	if z.bpmBox.Focused() {
		t.Error("expected bpmBox to be unfocused after inputBlocked blurs it")
	}

	// The value "160" should have been committed.
	if z.BPM() != 160 {
		t.Errorf("expected BPM 160 after blocked commit, got %d", z.BPM())
	}
	if len(log.bpmChanges) == 0 {
		t.Error("expected OnBPMChange callback after blocked commit")
	}
}

// TestTransport_OverflowButtonMobile verifies that on mobile, when DrumView
// places the overflow button into the bottom action bar (simulated here by
// directly setting its rect and re-registering hit areas with the bar as
// ClipRect), clicking it fires OnOverflowOpen. The zone's mobile layout
// alone leaves the button rect empty — DrumView is the sole authority that
// positions it (B3 critique).
func TestTransport_OverflowButtonMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	defer restore()

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(0, 0, 400, 120))
	tree.Update()

	// Simulate DrumView's bar placement.
	bar := image.Rect(0, 600, 400, 644) // 44 px tall (TouchMinTarget on mobile)
	z.overflowBtn.SetRect(image.Rect(260, 600, 380, 644))
	z.rebuildHitAreas()
	z.SetBottomBarHostedHitClip(bar)
	tree.HitIndexRef().Update("transport", z.HitAreas())

	overflowArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-overflow")
	if overflowArea == nil {
		t.Fatal("expected 'transport-overflow' hit area after DrumView bar placement")
	}

	mx, my = (overflowArea.Rect.Min.X+overflowArea.Rect.Max.X)/2, (overflowArea.Rect.Min.Y+overflowArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.overflowOpens == 0 {
		t.Error("expected OnOverflowOpen callback after clicking overflow button")
	}
}

// TestTransport_ViewSwitchButtonMobile verifies that on mobile, when DrumView
// places the view-switch button into the bottom action bar, clicking it
// fires OnViewCycle. See TestTransport_OverflowButtonMobile for the contract.
func TestTransport_ViewSwitchButtonMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 800 },
	)
	defer restore()

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(0, 0, 400, 120))
	tree.Update()

	// Simulate DrumView's bar placement.
	bar := image.Rect(0, 600, 400, 644)
	z.viewSwitchBtn.SetRect(image.Rect(140, 600, 250, 644))
	z.rebuildHitAreas()
	z.SetBottomBarHostedHitClip(bar)
	tree.HitIndexRef().Update("transport", z.HitAreas())

	viewArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-view-switch")
	if viewArea == nil {
		t.Fatal("expected 'transport-view-switch' hit area after DrumView bar placement")
	}

	mx, my = (viewArea.Rect.Min.X+viewArea.Rect.Max.X)/2, (viewArea.Rect.Min.Y+viewArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.viewCycles == 0 {
		t.Error("expected OnViewCycle callback after clicking view switch button")
	}
}

// TestTransport_SubdivButtonCallback verifies that clicking the subdiv button
// fires the OnSubdivClick callback.
func TestTransport_SubdivButtonCallback(t *testing.T) {
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

	z, log := newTestTransportZone()
	tree := registerTransportZone(z, image.Rect(10, 10, 500, 90))
	tree.Update()

	subdivArea := findHitAreaByTagPrefix(z.HitAreas(), "transport-subdiv")
	if subdivArea == nil {
		t.Fatal("expected 'transport-subdiv' hit area")
	}

	mx, my = (subdivArea.Rect.Min.X+subdivArea.Rect.Max.X)/2, (subdivArea.Rect.Min.Y+subdivArea.Rect.Max.Y)/2
	pressed = true
	tree.Update()
	pressed = false
	tree.Update()

	if log.subdivClicks == 0 {
		t.Error("expected OnSubdivClick callback after clicking subdiv button")
	}
}
