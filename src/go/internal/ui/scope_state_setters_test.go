//go:build test

package ui

import (
	"image"
	"testing"
)

// findHandlerByTag returns the hit handler with the given tag, or fails.
func findChainHandlerByTag(t *testing.T, z *ChainPanelZone, tag string) HitHandler {
	t.Helper()
	for _, ha := range z.HitAreas() {
		if ha.Tag == tag {
			return ha.Handler
		}
	}
	t.Fatalf("hit area with tag %q not found", tag)
	return nil
}

// TestChainSetAutoGainParityWithButton — invoking SetAutoGain produces the
// same {autoGain, yGain} state as clicking the AG button. This is the
// guarantee scenes need: state changes routed through the new setter must
// be indistinguishable from real user interaction.
func TestChainSetAutoGainParityWithButton(t *testing.T) {
	assertDefaultParityState(t)
	clickZ := NewChainPanelZone(ChainCallbacks{})
	clickZ.Layout(image.Rect(0, 0, 800, 160))
	apiZ := NewChainPanelZone(ChainCallbacks{})
	apiZ.Layout(image.Rect(0, 0, 800, 160))

	// First, force a non-default starting condition: manual yGain.
	clickZ.SetYGain(2.5)
	apiZ.SetYGain(2.5)

	// Click path: AG button toggles autoGain; clicking it ON resets yGain.
	if clickZ.autoGainBtn == nil {
		t.Fatal("autoGainBtn nil")
	}
	clickZ.autoGainBtn.OnClick()

	// API path: same effect via SetAutoGain.
	apiZ.SetAutoGain(true)

	if clickZ.AutoGain() != apiZ.AutoGain() {
		t.Errorf("autoGain divergence: click=%v api=%v", clickZ.AutoGain(), apiZ.AutoGain())
	}
	if clickZ.YGain() != apiZ.YGain() {
		t.Errorf("yGain divergence after enabling AG: click=%v api=%v", clickZ.YGain(), apiZ.YGain())
	}
}

// TestChainSetTraceVisibleParityWithSwatchClick — toggling trace visibility
// via SetTraceVisible matches the swatch hit-handler.
func TestChainSetTraceVisibleParityWithSwatchClick(t *testing.T) {
	assertDefaultParityState(t)
	clickZ := NewChainPanelZone(ChainCallbacks{})
	clickZ.Layout(image.Rect(0, 0, 800, 160))
	apiZ := NewChainPanelZone(ChainCallbacks{})
	apiZ.Layout(image.Rect(0, 0, 800, 160))

	// Both start with both traces visible (default).
	if !clickZ.TraceVisible("A") || !clickZ.TraceVisible("B") {
		t.Fatal("preconditions: both traces should start visible")
	}

	// Need the swatch handlers to exist — they're laid out only after a
	// tap is selected. Drive a tap on each side so swatch hit areas appear.
	clickZ.SetTapA(0)
	clickZ.SetTapB(1)
	apiZ.SetTapA(0)
	apiZ.SetTapB(1)
	// Re-layout to refresh hit areas with the swatches present.
	clickZ.Layout(image.Rect(0, 0, 800, 160))
	apiZ.Layout(image.Rect(0, 0, 800, 160))

	// Click path: hide trace A.
	hA := findChainHandlerByTag(t, clickZ, "scope-swatch-a")
	hA.OnPress(0, 0)
	// API path: hide trace A.
	apiZ.SetTraceVisible("A", false)

	if clickZ.TraceVisible("A") != apiZ.TraceVisible("A") {
		t.Errorf("trace A divergence: click=%v api=%v", clickZ.TraceVisible("A"), apiZ.TraceVisible("A"))
	}
	if clickZ.TraceVisible("A") {
		t.Error("trace A should be hidden")
	}
}

// TestChainSetWindowMsClampsToWheelRange — SetWindowMs enforces the same
// clamping the wheel handler does.
func TestChainSetWindowMsClamping(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(0.1)
	if z.WindowMs() != 1 {
		t.Errorf("SetWindowMs(0.1) clamped to %v; want 1", z.WindowMs())
	}
	z.SetWindowMs(10000)
	if z.WindowMs() != 500 {
		t.Errorf("SetWindowMs(10000) clamped to %v; want 500", z.WindowMs())
	}
	z.SetWindowMs(20)
	if z.WindowMs() != 20 {
		t.Errorf("SetWindowMs(20) = %v; want 20", z.WindowMs())
	}
}

// TestChainSetYGainDisablesAutoGain — SetYGain mirrors the shift-scroll
// behaviour: setting a manual gain disables auto-gain.
func TestChainSetYGainDisablesAutoGain(t *testing.T) {
	assertDefaultParityState(t)
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetAutoGain(true)
	if !z.AutoGain() {
		t.Fatal("precondition: AG should be on")
	}
	z.SetYGain(4)
	if z.AutoGain() {
		t.Error("SetYGain should disable auto-gain")
	}
	if z.YGain() != 4 {
		t.Errorf("yGain = %v; want 4", z.YGain())
	}
}

// TestChainSetFrozenInvokesCallback — SetFrozen calls OnFreezeToggle so
// the audio service stays in lockstep with the UI state.
func TestChainSetFrozenInvokesCallback(t *testing.T) {
	assertDefaultParityState(t)
	var audioFrozen bool
	cb := ChainCallbacks{
		OnFreezeToggle: func() bool {
			audioFrozen = !audioFrozen
			return audioFrozen
		},
	}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 800, 160))

	z.SetFrozen(true)
	if !z.Frozen() {
		t.Error("zone.Frozen() = false after SetFrozen(true)")
	}
	if !audioFrozen {
		t.Error("OnFreezeToggle callback was not invoked")
	}
	z.SetFrozen(false)
	if z.Frozen() {
		t.Error("zone.Frozen() = true after SetFrozen(false)")
	}
	if audioFrozen {
		t.Error("audio side did not unfreeze")
	}
}

// TestChainSetFrozenIdempotent — calling SetFrozen with the current state
// is a no-op (no callback invoked).
func TestChainSetFrozenIdempotent(t *testing.T) {
	assertDefaultParityState(t)
	calls := 0
	cb := ChainCallbacks{
		OnFreezeToggle: func() bool { calls++; return false },
	}
	z := NewChainPanelZone(cb)
	z.Layout(image.Rect(0, 0, 800, 160))

	z.SetFrozen(false) // already false
	if calls != 0 {
		t.Errorf("idempotent SetFrozen(false) invoked callback %d times; want 0", calls)
	}
}
