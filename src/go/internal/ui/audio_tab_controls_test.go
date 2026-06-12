package ui

import "testing"

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
