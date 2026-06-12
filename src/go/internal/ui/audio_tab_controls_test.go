package ui

import (
	"image"
	"testing"
)

// controlHeaderHeight mirrors stickyBarHeight: 26 desktop, 36 mobile.
func TestControlHeaderHeight(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	r2 := SetDensityForTest(DensityComfortable)
	defer r2()
	if got := controlHeaderHeight(); got != audioControlHeaderH {
		t.Fatalf("desktop control header height = %d, want %d", got, audioControlHeaderH)
	}
}

// newFreezePill builds a "||" pill that flips to ">" / colAccent when the
// freeze callback reports frozen, and back to "||" / colTextSecondary.
func TestNewFreezePillTogglesVisual(t *testing.T) {
	frozen := false
	b := newFreezePill(func() bool { frozen = !frozen; return frozen })
	if b.Text != "||" {
		t.Fatalf("initial freeze text = %q, want ||", b.Text)
	}
	b.OnClick()
	if b.Text != ">" {
		t.Fatalf("after first toggle text = %q, want >", b.Text)
	}
	b.OnClick()
	if b.Text != "||" {
		t.Fatalf("after second toggle text = %q, want ||", b.Text)
	}
}

// With no components built, the active controls are nil on every tab, the
// header height is 0, and bodyRect equals contentRect (zero behavior change).
func TestPhase0NoControlHeader(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.Layout(image.Rect(0, 400, 600, 600))
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		if z.controlHeaderH() != 0 {
			t.Fatalf("tab %v: controlHeaderH = %d, want 0", tab, z.controlHeaderH())
		}
		if z.bodyRect() != z.contentRect() {
			t.Fatalf("tab %v: bodyRect %v != contentRect %v", tab, z.bodyRect(), z.contentRect())
		}
	}
}
