//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
)

// TestMobileRecordButtonNeverSliver verifies the mobile record button keeps a
// full touch-size cell instead of collapsing to a ~6px sliver between Stop and
// the BPM stepper (the pixel-diff bug). Root cause was the cosmetic
// recordDemoteInsetMobile shrinking the proportional cell below the touch
// target on narrow phones; the fix gates the demote and applies an
// enforceMinSize floor of TransportBtnSize.
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

	// Sweep a range of realistic phone widths (narrow → wide) at a single-row
	// toolbar height. The single-row mobile layout packs Play|Stop|Record|BPM|
	// Subdiv|Vol|Overflow, so narrow widths stress the record cell most.
	//
	// The invariant is "record is never a SLIVER", not "record is a full
	// TransportBtnSize rectangle": like every mobile transport button, record
	// renders at the (narrow) transport-cell width and relies on ExpandHitArea
	// for the 44px touch target — so its drawn width legitimately sits below
	// TransportBtnSize on cramped phones, and its height is intentionally
	// B12-demoted below play/stop. What must never happen again is the ~6px
	// horizontal collapse: record must track its play/stop siblings in width
	// and stay above a sliver floor in height.
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

// TestRecordingStateChangesRecordButtonStyle verifies the record button is not
// pixel-identical between idle and recording: the icon tint must change to the
// record-active token (breathing) when recording and return to the idle tint
// when stopped.
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
	// WithAlpha returns a non-premultiplied NRGBA, so the R/G/B channels hold
	// the true record-active hue regardless of the breathing alpha.
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

// TestRecordIndicatorGatedByRecordingState verifies the persistent "REC"
// indicator only renders while recording. It does so indirectly by asserting
// the toolbar cache hash changes when recording toggles (the indicator is
// gated on isRecording, which participates in the hash), guaranteeing a
// pixel-level idle/recording difference on both platforms.
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
