//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- sliderGroupHitAdapter unit tests ---
//
// The sliderGroupHitAdapter wraps a SliderGroup as a HitHandler for the
// hit-index system. These tests exercise the adapter directly (OnPress,
// OnDrag, OnRelease, OnWheel) to cover the 0% functions that the existing
// integration test (TestEQPanelZoneSliderCapture) only reaches indirectly.

func TestSliderGroupHitAdapter_OnPressHit(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(3)
	var released bool
	g := NewSliderGroup(sliders, nil)
	adapter := &sliderGroupHitAdapter{
		group:     g,
		onRelease: func() { released = true },
	}

	// Press inside slider 1's bounds.
	sr := sliders[1].Rect()
	cx, cy := sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2

	result := adapter.OnPress(cx, cy)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured on hit, got %d", result)
	}
	if !adapter.active {
		t.Fatal("adapter should be active after press hit")
	}
	if released {
		t.Fatal("onRelease should not fire on press")
	}
}

func TestSliderGroupHitAdapter_OnPressMiss(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(2)
	g := NewSliderGroup(sliders, nil)
	adapter := &sliderGroupHitAdapter{group: g}

	// Press outside all slider bounds.
	result := adapter.OnPress(999, 999)
	if result != InputIgnored {
		t.Fatalf("expected InputIgnored on miss, got %d", result)
	}
	if adapter.active {
		t.Fatal("adapter should not be active after miss")
	}
}

func TestSliderGroupHitAdapter_OnDragRoutesToGroup(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	var lastVal float64
	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, func(idx int, val float64) {
		lastVal = val
	})
	adapter := &sliderGroupHitAdapter{group: g}

	// Press to activate.
	sr := sliders[0].Rect()
	adapter.OnPress(sr.Min.X+1, sr.Min.Y+sr.Dy()/2)
	if !adapter.active {
		t.Fatal("adapter should be active after press")
	}

	// Drag to the right edge.
	track := sliders[0].TrackRect()
	adapter.OnDrag(track.Max.X-1, sr.Min.Y+sr.Dy()/2)

	if lastVal < 0.9 {
		t.Fatalf("expected value near 1.0 after drag to right, got %f", lastVal)
	}
}

func TestSliderGroupHitAdapter_OnDragInactiveIsNoop(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(1)
	called := false
	g := NewSliderGroup(sliders, func(idx int, val float64) { called = true })
	adapter := &sliderGroupHitAdapter{group: g}

	// Drag without prior press.
	adapter.OnDrag(50, 10)
	if called {
		t.Fatal("OnDrag should not route to group when adapter is not active")
	}
}

func TestSliderGroupHitAdapter_OnReleaseFiresCallback(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, nil)
	var releaseFired bool
	adapter := &sliderGroupHitAdapter{
		group:     g,
		onRelease: func() { releaseFired = true },
	}

	// Press.
	sr := sliders[0].Rect()
	adapter.OnPress(sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2)

	// Release.
	adapter.OnRelease(sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2)
	if adapter.active {
		t.Fatal("adapter should not be active after release")
	}
	if !releaseFired {
		t.Fatal("onRelease callback should fire on release")
	}
}

func TestSliderGroupHitAdapter_OnReleaseInactiveIsNoop(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, nil)
	var releaseFired bool
	adapter := &sliderGroupHitAdapter{
		group:     g,
		onRelease: func() { releaseFired = true },
	}

	// Release without prior press.
	adapter.OnRelease(50, 10)
	if releaseFired {
		t.Fatal("onRelease should not fire when adapter was not active")
	}
}

func TestSliderGroupHitAdapter_OnReleaseNilCallback(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, nil)
	adapter := &sliderGroupHitAdapter{
		group:     g,
		onRelease: nil, // nil callback must not panic
	}

	sr := sliders[0].Rect()
	adapter.OnPress(sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2)
	adapter.OnRelease(sr.Min.X+sr.Dx()/2, sr.Min.Y+sr.Dy()/2) // must not panic
}

func TestSliderGroupHitAdapter_OnWheelReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)

	sliders := makeTestSliders(1)
	g := NewSliderGroup(sliders, nil)
	adapter := &sliderGroupHitAdapter{group: g}

	result := adapter.OnWheel(50, 10, -1)
	if result != InputIgnored {
		t.Fatalf("expected InputIgnored from OnWheel, got %d", result)
	}
}

// --- eqChannelDropdownOverlay tests ---

func TestEQChannelDropdownOverlay_ShouldCloseReturnsFalse(t *testing.T) {
	overlay := &eqChannelDropdownOverlay{}
	if overlay.ShouldClose() {
		t.Fatal("ShouldClose should return false")
	}
}

// --- scrollableDropdownHandler: scrollbar thumb drag path ---
//
// The existing eq_channel_scroll_test.go covers touch scroll, wheel scroll,
// and deferred tap. This test specifically covers the scrollbar thumb drag
// path in OnPress → OnDrag → OnRelease.

func TestScrollableDropdownHandler_ThumbDrag(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := makeOverflowRows(10) // 11 total items, will overflow
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open the dropdown — this creates a portal overlay.
	z.eqChannelBtn.OnClick()

	if !z.channelScroll.HasScroll() {
		t.Fatal("expected scrollable dropdown with 11 items")
	}

	// Build a handler directly referencing the overlay.
	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()
	handler := &scrollableDropdownHandler{overlay: overlay}

	// Get the scrollbar thumb rect.
	thumb := z.channelScroll.ThumbRect()
	if thumb.Empty() {
		t.Fatal("expected non-empty thumb rect")
	}

	// Press on the thumb.
	thumbCX := (thumb.Min.X + thumb.Max.X) / 2
	thumbCY := (thumb.Min.Y + thumb.Max.Y) / 2
	result := handler.OnPress(thumbCX, thumbCY)
	if result != InputCaptured {
		t.Fatalf("expected InputCaptured on thumb press, got %d", result)
	}

	if !z.channelScroll.Dragging() {
		t.Fatal("expected scroll to be dragging after thumb press")
	}

	// Drag down.
	handler.OnDrag(thumbCX, thumbCY+40)

	// Release.
	handler.OnRelease(thumbCX, thumbCY+40)
	if z.channelScroll.Dragging() {
		t.Fatal("expected scroll to stop dragging after release")
	}

	_ = tree // keep ref
}

// --- CloseChannelDropdown ---

func TestCloseChannelDropdown_ClosesOpenDropdown(t *testing.T) {
	assertDefaultParityState(t)

	rows := []*DrumRow{{Name: "Kick", Instrument: "kick"}}
	z, _ := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open dropdown.
	z.eqChannelBtn.OnClick()
	if !z.ChannelDropdownOpen() {
		t.Fatal("expected dropdown to be open")
	}

	// Close it.
	z.CloseChannelDropdown()
	if z.ChannelDropdownOpen() {
		t.Error("expected dropdown to be closed")
	}
}

func TestCloseChannelDropdown_NilPortalDoesNotPanic(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	z.channelOpen = true
	z.portal = nil

	z.CloseChannelDropdown() // must not panic
	if z.channelOpen {
		t.Error("channelOpen should be false after CloseChannelDropdown")
	}
}

// --- SetWaveformMode ---

func TestSetWaveformMode(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)

	if z.WaveformMode() {
		t.Fatal("should start in EQ mode")
	}

	z.SetWaveformMode(true)
	if !z.WaveformMode() {
		t.Fatal("expected waveform mode after SetWaveformMode(true)")
	}
	if z.eqToggleBtn.Text != "Wave" {
		t.Errorf("button text should be 'Wave' (current tab), got %q", z.eqToggleBtn.Text)
	}

	z.SetWaveformMode(false)
	if z.WaveformMode() {
		t.Fatal("expected EQ mode after SetWaveformMode(false)")
	}
	if z.eqToggleBtn.Text != "EQ" {
		t.Errorf("button text should be 'EQ' (current tab), got %q", z.eqToggleBtn.Text)
	}
}

// --- ActiveChannel ---

func TestActiveChannel_DefaultsToMain(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	if ch := z.ActiveChannel(); ch != "main" {
		t.Errorf("expected 'main', got %q", ch)
	}
}

func TestActiveChannel_EmptyStringReturnsMain(t *testing.T) {
	z, _ := newTestEQPanelZone(nil)
	z.activeChannel = ""
	if ch := z.ActiveChannel(); ch != "main" {
		t.Errorf("expected 'main' for empty activeChannel, got %q", ch)
	}
}

// --- P3 additional tests ---

// TestEQ_CurveDragBandGainChange verifies that pressing on a curve handle,
// dragging vertically, and releasing updates gain and fires OnGainChange + OnApplyEQ.
func TestEQ_CurveDragBandGainChange(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	rect := image.Rect(0, 200, 800, 400)
	tree := registerEQZone(z, rect)

	restore := noInputForTest()
	tree.Update()
	restore()

	// Find the curve area hit area.
	areas := z.HitAreas()
	var curveArea *HitArea
	for i := range areas {
		if areas[i].Tag == "eq-curve-area" {
			curveArea = &areas[i]
			break
		}
	}
	if curveArea == nil {
		t.Fatal("expected 'eq-curve-area' hit area")
	}

	// Target band 3 (~500 Hz). Compute its handle position.
	center := math.Sqrt(eqBandDefs[3].loHz * eqBandDefs[3].hiHz)
	hx := freqToX(center, z.rect)
	hy := gainDBToY(0, z.rect) // starts at 0 dB

	// Press on band 3 handle.
	result := curveArea.Handler.OnPress(hx, hy)
	if result == InputIgnored {
		t.Fatal("expected press on band 3 handle to be consumed")
	}
	if z.curveDragBand != 3 {
		t.Fatalf("expected curveDragBand=3, got %d", z.curveDragBand)
	}

	// Drag upward (toward +8 dB).
	targetY := gainDBToY(8, z.rect)
	curveArea.Handler.OnDrag(hx, targetY)

	if z.bandGainsDB[3] < 5.0 {
		t.Errorf("expected gain > 5 dB after drag, got %.1f", z.bandGainsDB[3])
	}
	if !z.curveDirty {
		t.Error("expected curveDirty after drag")
	}
	if len(log.gainChanges) == 0 {
		t.Error("expected OnGainChange fired during drag")
	}

	// Release.
	curveArea.Handler.OnRelease(hx, targetY)
	if z.curveDragBand != -1 {
		t.Errorf("expected curveDragBand=-1 after release, got %d", z.curveDragBand)
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired on release")
	}
}

// TestEQ_HPFHandleDrag verifies that dragging the HPF handle changes the cutoff
// frequency and fires OnHPFCutoffChange + OnApplyEQ.
func TestEQ_HPFHandleDrag(t *testing.T) {
	assertDefaultParityState(t)

	z, log, fs := newTestEQPanelZoneWithFilters(nil)
	fs.hpfEnabled = true
	fs.hpfCutoff = 100.0
	fs.lpfEnabled = false
	tree := registerEQZone(z, image.Rect(0, 100, 600, 400))

	restore := noInputForTest()
	tree.Update()
	restore()

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

	// Drag to a higher frequency.
	newX := freqToX(400.0, r)
	adapter.OnDrag(newX, hy)

	if len(log.hpfCutoffChanges) == 0 {
		t.Error("expected OnHPFCutoffChange fired during drag")
	}
	// Verify cutoff was clamped within [20, 2000].
	lastCutoff := log.hpfCutoffChanges[len(log.hpfCutoffChanges)-1]
	if lastCutoff < 20 || lastCutoff > 2000 {
		t.Errorf("expected HPF cutoff in [20, 2000], got %.1f", lastCutoff)
	}

	// Release.
	adapter.OnRelease(newX, hy)
	if z.curveDragFilter != "" {
		t.Error("expected curveDragFilter cleared after release")
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired on release")
	}
}

// TestEQ_DBInputCommit verifies that focusing a dB input, changing text to
// "6.0", and simulating Enter commits the value via OnGainChange + OnApplyEQ.
func TestEQ_DBInputCommit(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Set up focused state: simulate what focus-gained would do.
	ti := z.eqDBInputs[4]
	z.dbInputPrev = 0
	z.dbInputFocused = 4
	ti.focused = true
	ti.SetText("6.0")

	// Simulate Enter key via HandleKey.
	result := z.HandleKey(ebiten.KeyEnter)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from HandleKey(Enter), got %d", result)
	}

	if z.bandGainsDB[4] != 6.0 {
		t.Errorf("expected gain=6.0, got %v", z.bandGainsDB[4])
	}
	if ti.Focused() {
		t.Error("expected dB input to be unfocused after Enter commit")
	}
	if len(log.gainChanges) == 0 || log.gainChanges[len(log.gainChanges)-1].db != 6.0 {
		t.Error("expected OnGainChange(4, 6.0)")
	}
	if log.applyCount == 0 {
		t.Error("expected OnApplyEQ fired after Enter commit")
	}
	_ = tree
}

// TestEQ_DBInputEscapeReverts verifies that focusing a dB input, changing text,
// and pressing Escape reverts to the previous value without firing gain/apply callbacks.
func TestEQ_DBInputEscapeReverts(t *testing.T) {
	assertDefaultParityState(t)

	z, log := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Pre-set gain for band 5 to 3.0.
	z.bandGainsDB[5] = 3.0
	z.dbInputPrev = 3.0 // simulate what focus-gained saves

	ti := z.eqDBInputs[5]
	ti.focused = true
	z.dbInputFocused = 5
	ti.SetText("10.0") // user typed this

	// Simulate Escape key.
	result := z.HandleKey(ebiten.KeyEscape)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed from HandleKey(Escape), got %d", result)
	}

	// Text should revert to previous formatted value.
	if ti.Value() != "+3.0" {
		t.Errorf("expected text reverted to '+3.0', got %q", ti.Value())
	}
	if ti.Focused() {
		t.Error("expected dB input unfocused after Escape")
	}
	if z.dbInputFocused != -1 {
		t.Errorf("expected dbInputFocused=-1, got %d", z.dbInputFocused)
	}
	// Escape should NOT fire OnGainChange or OnApplyEQ.
	if len(log.gainChanges) != 0 {
		t.Error("expected no OnGainChange on Escape")
	}
	if log.applyCount != 0 {
		t.Error("expected no OnApplyEQ on Escape")
	}
	_ = tree
}

// TestEQ_ChannelDropdownSelectSwitches verifies that opening the channel dropdown,
// selecting a channel entry, fires OnChannelChange and closes the dropdown.
func TestEQ_ChannelDropdownSelectSwitches(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	rows := []*DrumRow{
		{Name: "Kick", Instrument: "kick"},
		{Name: "Snare", Instrument: "snare"},
	}
	z, log := newTestEQPanelZone(rows)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))

	restore := noInputForTest()
	tree.Update()
	restore()

	// Open the channel dropdown.
	z.eqChannelBtn.OnClick()
	if !z.ChannelDropdownOpen() {
		t.Fatal("expected channel dropdown to be open")
	}

	// Build overlay buttons to simulate selection.
	overlay := &eqChannelDropdownOverlay{zone: z}
	overlay.buildButtons()

	if len(overlay.buttons) < 2 {
		t.Fatalf("expected at least 2 buttons (Master + Kick), got %d", len(overlay.buttons))
	}

	// Click the "Kick" entry (index 1).
	kickBtn := overlay.buttons[1]
	if kickBtn.OnClick == nil {
		t.Fatal("expected Kick button to have OnClick")
	}
	kickBtn.OnClick()

	// Verify OnChannelChange fired with "kick".
	if len(log.channelChanges) == 0 {
		t.Fatal("expected OnChannelChange callback after selecting Kick")
	}
	if log.channelChanges[len(log.channelChanges)-1] != "kick" {
		t.Errorf("expected OnChannelChange('kick'), got %q", log.channelChanges[len(log.channelChanges)-1])
	}

	// Dropdown should be closed.
	if z.channelOpen {
		t.Error("expected dropdown closed after selection")
	}
	if z.activeChannel != "kick" {
		t.Errorf("expected activeChannel='kick', got %q", z.activeChannel)
	}
	if z.eqChannelBtn.Text != "Kick" {
		t.Errorf("expected channel button text 'Kick', got %q", z.eqChannelBtn.Text)
	}
	_ = tree
}
