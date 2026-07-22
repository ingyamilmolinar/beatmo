//go:build test

package ui

import "testing"

// TestNewPopupCloseButton_DesktopVsMobile pins the shared close-button policy:
// the × exists on desktop and is absent (nil) on mobile, where tap-away is the
// sole dismiss. Every floating pop-up routes its close × through this helper, so
// this one test guards the platform gate for all of them.
func TestNewPopupCloseButton_DesktopVsMobile(t *testing.T) {
	// Desktop: a real close button with the close icon.
	withSmallScreen(t, false)
	if !desktopCloseVisible() {
		t.Fatalf("desktopCloseVisible() = false on desktop; want true")
	}
	called := false
	b := newPopupCloseButton(func() { called = true })
	if b == nil {
		t.Fatalf("newPopupCloseButton returned nil on desktop; want a button")
	}
	if b.Icon != string(IconClose) {
		t.Fatalf("close button icon = %q; want %q", b.Icon, string(IconClose))
	}
	if !b.ConsumeOnPress {
		t.Fatalf("close button ConsumeOnPress = false; want true")
	}
	if b.OnClick != nil {
		b.OnClick()
	}
	if !called {
		t.Fatalf("close button OnClick did not invoke onClose")
	}

	// Mobile: no × — tap-away closes.
	withSmallScreen(t, true)
	if desktopCloseVisible() {
		t.Fatalf("desktopCloseVisible() = true on mobile; want false")
	}
	if mb := newPopupCloseButton(func() {}); mb != nil {
		t.Fatalf("newPopupCloseButton returned non-nil on mobile; want nil")
	}
}
