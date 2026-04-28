//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTransportZoneRecordButton verifies the record button is created and laid out.
func TestTransportZoneRecordButton(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	if z.recordBtn == nil {
		t.Fatal("recordBtn is nil")
	}

	rect := z.recordBtn.Rect()
	if rect.Empty() {
		t.Error("recordBtn has empty rect after desktop layout")
	}

	if z.recordBtn.Icon != "record" {
		t.Errorf("recordBtn.Icon = %q, want %q", z.recordBtn.Icon, "record")
	}
}

// TestTransportZoneRecordPressedFlag verifies the one-frame pulse behavior.
func TestTransportZoneRecordPressedFlag(t *testing.T) {
	z, _ := newTestTransportZone()

	// Not pressed initially
	if z.RecordPressed() {
		t.Error("RecordPressed should be false initially")
	}

	// Simulate press
	z.recordPressed = true
	if !z.RecordPressed() {
		t.Error("RecordPressed should be true after setting flag")
	}

	// Consumed — second read should be false
	if z.RecordPressed() {
		t.Error("RecordPressed should be false after being consumed")
	}
}

// TestTransportZoneSetRecording verifies the recording state toggle.
func TestTransportZoneSetRecording(t *testing.T) {
	z, _ := newTestTransportZone()

	if z.IsRecording() {
		t.Error("IsRecording should be false initially")
	}

	z.SetRecording(true)
	if !z.IsRecording() {
		t.Error("IsRecording should be true after SetRecording(true)")
	}

	z.SetRecording(false)
	if z.IsRecording() {
		t.Error("IsRecording should be false after SetRecording(false)")
	}
}

// TestTransportZoneRecordCallback verifies the OnRecordToggle callback fires.
func TestTransportZoneRecordCallback(t *testing.T) {
	recordToggles := 0
	cb := TransportCallbacks{
		OnRecordToggle: func() { recordToggles++ },
		IsPlaying:      func() bool { return false },
		GetMainVolume:  func() float64 { return 1.0 },
		SetMainVolume:  func(v float64) {},
	}

	z := NewTransportZone(cb)
	z.Layout(image.Rect(0, 0, 800, 40))

	// Click the record button
	z.recordBtn.OnClick()

	if recordToggles != 1 {
		t.Errorf("OnRecordToggle called %d times, want 1", recordToggles)
	}
	if !z.recordPressed {
		t.Error("recordPressed should be true after click")
	}
}

// TestTransportZoneRecordButtonInHitAreas verifies the record button is registered in hit areas.
func TestTransportZoneRecordButtonInHitAreas(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	hitAreas := z.HitAreas()
	found := false
	for _, ha := range hitAreas {
		if ha.Tag == "transport-record" {
			found = true
			if ha.Rect.Empty() {
				t.Error("transport-record hit area has empty rect")
			}
			break
		}
	}
	if !found {
		t.Error("transport-record not found in hit areas")
	}
}

// TestTransportZoneRecordPulseAnimation verifies pulsing animation when recording.
func TestTransportZoneRecordPulseAnimation(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	// Not recording — pulse should stay 0
	z.decayAnims()
	if z.recordPulse != 0 {
		t.Errorf("recordPulse = %f, want 0 when not recording", z.recordPulse)
	}

	// Start recording — pulse should advance
	z.isRecording = true
	z.decayAnims()
	if z.recordPulse == 0 {
		t.Error("recordPulse should advance when recording")
	}

	// Icon color should be different from idle
	idleColor := colRecordIdle
	currentColor := z.recordBtn.IconColor
	if currentColor == idleColor {
		t.Error("recording icon color should differ from idle")
	}

	// Stop recording — pulse should reset
	z.isRecording = false
	z.decayAnims()
	if z.recordPulse != 0 {
		t.Error("recordPulse should reset to 0 when not recording")
	}
	if z.recordBtn.IconColor != colRecordIdle {
		t.Error("icon should return to idle color when not recording")
	}
}

// TestTransportZoneRecordInToolbarHash verifies the toolbar cache invalidates on record state change.
func TestTransportZoneRecordInToolbarHash(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	hash1 := z.toolbarStateHash()
	z.isRecording = true
	hash2 := z.toolbarStateHash()

	if hash1 == hash2 {
		t.Error("toolbar hash should change when recording state changes")
	}
}

// TestTransportZoneDesktopLayoutColumnCount verifies the 11-column desktop layout.
func TestTransportZoneDesktopLayoutColumnCount(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 1000, 40))

	// Play, Stop, Record should have non-empty rects
	if z.playBtn.Rect().Empty() {
		t.Error("playBtn rect empty")
	}
	if z.stopBtn.Rect().Empty() {
		t.Error("stopBtn rect empty")
	}
	if z.recordBtn.Rect().Empty() {
		t.Error("recordBtn rect empty")
	}
	// Record should be between stop and BPM
	if z.recordBtn.Rect().Min.X <= z.stopBtn.Rect().Min.X {
		t.Error("record button should be to the right of stop button")
	}
	if z.bpmBox.Rect.Min.X <= z.recordBtn.Rect().Min.X {
		t.Error("BPM box should be to the right of record button")
	}
}

// TestTransportZoneRecordButtonMobileLayout verifies mobile layout doesn't panic
// and record button is hidden.
func TestTransportZoneRecordButtonMobileLayout(t *testing.T) {
	// Mobile layout runs through layoutMobile which hides record button.
	// We verify by checking that the mobile code path sets an empty rect.
	z, _ := newTestTransportZone()
	z.layoutMobile(image.Rect(0, 0, 400, 80), 2, MobileTopBarSpec())
	if !z.recordBtn.Rect().Empty() {
		t.Error("recordBtn should have empty rect in mobile layout")
	}
}

// TestDrawRecordIcon verifies drawRecordIcon executes cleanly. Real
// pixel rendering is exercised by icon_visual_consistency_test.go in
// the non-test build (the test stub's vector package is no-op).
func TestDrawRecordIcon(t *testing.T) {
	img := ebiten.NewImage(32, 32)
	r := image.Rect(0, 0, 32, 32)
	col := color.RGBA{200, 50, 50, 255}
	drawRecordIcon(img, r, col)
	// Should not panic; pixel-output guarantees live in the
	// non-test-build visual consistency suite.
}

// TestDrawRecordIconEmpty verifies no panic on empty rect.
func TestDrawRecordIconEmpty(t *testing.T) {
	img := ebiten.NewImage(32, 32)
	drawRecordIcon(img, image.Rectangle{}, color.White)
	// Should not panic
}

// TestDrawRecordIconSmall verifies icon renders on small rects.
func TestDrawRecordIconSmall(t *testing.T) {
	img := ebiten.NewImage(4, 4)
	drawRecordIcon(img, image.Rect(0, 0, 4, 4), color.White)
	// Should not panic
}

// TestDrumViewRecordPressed verifies the DrumView bridge for RecordPressed.
func TestDrumViewRecordPressed(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Initially false
	if g.drum.RecordPressed() {
		t.Error("RecordPressed should be false initially")
	}

	// Set via transport zone
	g.drum.transportZone.recordPressed = true
	if !g.drum.RecordPressed() {
		t.Error("RecordPressed should be true after setting transport zone flag")
	}

	// Consumed
	if g.drum.RecordPressed() {
		t.Error("RecordPressed should be consumed after first read")
	}
}

// TestDrumViewSetRecording verifies the DrumView bridge for SetRecording.
func TestDrumViewSetRecording(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.SetRecording(true)
	if !g.drum.IsRecording() {
		t.Error("IsRecording should be true after SetRecording(true)")
	}

	g.drum.SetRecording(false)
	if g.drum.IsRecording() {
		t.Error("IsRecording should be false after SetRecording(false)")
	}
}

// TestRecordBtnDrawDoesNotPanic verifies the record button draws without panic.
func TestRecordBtnDrawDoesNotPanic(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	screen := ebiten.NewImage(800, 40)
	// In test builds, the fallback drawRecordIcon path is used
	z.recordBtn.Draw(screen)
	// Should not panic
}
