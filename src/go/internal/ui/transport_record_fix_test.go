//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
)

// A full touch-size cell on mobile, never a ~6 px sliver between Stop and the BPM stepper.
func TestMobileRecordButtonNeverSliver(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	if !Profile().IsMobile() {
		t.Fatal("forceSmallScreenForTest did not select the mobile profile")
	}
	want := TransportBtnSize()
	if want <= 0 {
		t.Fatalf("TransportBtnSize() = %d, want > 0", want)
	}

	z, _ := newTestTransportZone()

	// Never a sliver, not a full TransportBtnSize: like every mobile transport button it draws at the narrow
	// cell width and relies on ExpandHitArea for the 44 px target, so it tracks play/stop's width and stays
	// above half a cell in height.
	sliverFloor := want / 2 // ~22px: well above the ~6px collapse, well below a full cell
	for _, w := range []int{320, 360, 390, 414, 480, 600} {
		z.Layout(image.Rect(0, 0, w, 44))
		rec := z.recordBtn.Rect()
		if rec.Dx() < sliverFloor {
			t.Errorf("width=%d: record Dx=%d < sliver floor %d (horizontal collapse — the ~6px bug)", w, rec.Dx(), sliverFloor)
		}
		if rec.Dy() < sliverFloor {
			t.Errorf("width=%d: record Dy=%d < sliver floor %d (vertical collapse)", w, rec.Dy(), sliverFloor)
		}
	}
}

// The icon tint moves to the record-active token while recording and back when stopped.
func TestRecordingStateChangesRecordButtonStyle(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	// Idle: one decay tick settles the icon to the idle tint.
	z.isRecording = false
	z.decayAnims()
	idle := z.recordBtn.IconColor
	if idle != colRecordIdle {
		t.Fatalf("idle record icon = %v, want colRecordIdle %v", idle, colRecordIdle)
	}

	// Recording: icon tint must differ from idle (record-active, alpha-pulsed).
	z.SetRecording(true)
	z.decayAnims()
	rec := z.recordBtn.IconColor
	if rec == idle {
		t.Error("recording record-icon color must differ from idle")
	}
	// WithAlpha is non-premultiplied, so R/G/B hold the true hue whatever the breathing alpha.
	nr, ok := rec.(color.NRGBA)
	if !ok {
		t.Fatalf("recording icon color = %T, want color.NRGBA", rec)
	}
	if nr.R != colRecordActive.R || nr.G != colRecordActive.G || nr.B != colRecordActive.B {
		t.Errorf("recording icon RGB = (%d,%d,%d), want record-active RGB (%d,%d,%d)",
			nr.R, nr.G, nr.B, colRecordActive.R, colRecordActive.G, colRecordActive.B)
	}

	// Stop: must return to the idle tint.
	z.SetRecording(false)
	z.decayAnims()
	if z.recordBtn.IconColor != colRecordIdle {
		t.Errorf("after stop, record icon = %v, want colRecordIdle", z.recordBtn.IconColor)
	}
}

// Through the toolbar cache hash, which includes isRecording, so idle and recording always differ in pixels.
func TestRecordIndicatorGatedByRecordingState(t *testing.T) {
	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 800, 40))

	z.SetRecording(false)
	idleHash := z.toolbarStateHash()

	z.SetRecording(true)
	recHash := z.toolbarStateHash()

	if idleHash == recHash {
		t.Error("toolbar hash identical idle vs recording; recording state would render pixel-identically")
	}
}
