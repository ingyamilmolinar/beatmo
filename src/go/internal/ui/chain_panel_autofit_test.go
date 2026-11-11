//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// TestChainAutoFitDefaultOn — a fresh zone opens with auto-fit enabled so
// the transient fills the trace without the user touching anything.
func TestChainAutoFitDefaultOn(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.AutoFit() {
		t.Fatal("auto-fit should default to on")
	}
}

// TestChainManualZoomDisablesAutoFit — SetWindowMs and the plain-scroll
// wheel are manual overrides; both must stick by disabling auto-fit.
func TestChainManualZoomDisablesAutoFit(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(40)
	if z.AutoFit() {
		t.Error("SetWindowMs must disable auto-fit")
	}

	z2 := NewChainPanelZone(ChainCallbacks{})
	h := &chainZoomHandler{zone: z2}
	if r := h.OnWheel(0, 0, -1); r != InputConsumed {
		t.Fatalf("wheel zoom returned %v, want InputConsumed", r)
	}
	if z2.AutoFit() {
		t.Error("plain-scroll wheel must disable auto-fit")
	}
}

// TestChainSetAutoFitReenables — SetAutoFit(true) turns it back on after a
// manual zoom disabled it.
func TestChainSetAutoFitReenables(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(40) // disables
	z.SetAutoFit(true)
	if !z.AutoFit() {
		t.Error("SetAutoFit(true) must re-enable auto-fit")
	}
}

// TestChainDoubleClickRestoresAutoFit — a double-click reset restores
// auto-fit (and resets the window) so the user gets the default framing back.
func TestChainDoubleClickRestoresAutoFit(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(40) // disable + non-default window
	h := &chainZoomHandler{zone: z}
	// Two presses in quick succession land within the 300ms double-click
	// window (the test runs in microseconds).
	h.OnPress(0, 0)
	h.OnPress(0, 0)
	if !z.AutoFit() {
		t.Error("double-click must restore auto-fit")
	}
	if z.WindowMs() != 20 {
		t.Errorf("double-click must reset window to 20, got %v", z.WindowMs())
	}
}

// TestChainAutoFitReframesWindow — with auto-fit on and a tiny transient in a
// large buffer, the effective window driving the labels must shrink far below
// the manual 20ms ceiling (proving the trace frames the signal, not the dead
// space). We assert via the time-axis label rendering: drawChainTraces draws
// the max label at the effective window. Instead of pixel-reading text, we
// drive Draw and confirm no panic + that auto-fit produced a sub-slice (the
// fitState samples are shorter than the source).
func TestChainAutoFitReframesWindow(t *testing.T) {
	const n = 4410 // 100ms @ 44.1k
	samples := make([]float64, n)
	for i := 2000; i < 2020; i++ { // ~0.45ms transient mid-buffer
		samples[i] = 1.0
	}
	st := &scope.State{
		TapA: scope.TapData{Stage: scope.StageSynth, Samples: samples, Active: true},
	}
	z := NewChainPanelZone(ChainCallbacks{ScopeState: func() *scope.State { return st }})
	z.SetTapA(scope.StageSynth)
	z.Layout(image.Rect(0, 0, 600, 200))
	dst := ebiten.NewImage(600, 200)
	z.Draw(dst)

	// fitState.TapA.Samples is the sub-slice Draw rendered. It must be much
	// shorter than the full buffer (framed to the transient + padding).
	got := len(z.fitState.TapA.Samples)
	if got == 0 {
		t.Fatal("auto-fit produced empty sub-slice")
	}
	if got >= n/2 {
		t.Errorf("auto-fit sub-slice len=%d not tight vs full buffer %d", got, n)
	}
}

// TestChainAutoFitOffRendersFullBuffer — with auto-fit disabled, Draw must
// NOT sub-slice; the full buffer feeds the trace.
func TestChainAutoFitOffRendersFullBuffer(t *testing.T) {
	const n = 4410
	samples := make([]float64, n)
	samples[2000] = 1.0
	st := &scope.State{
		TapA: scope.TapData{Stage: scope.StageSynth, Samples: samples, Active: true},
	}
	z := NewChainPanelZone(ChainCallbacks{ScopeState: func() *scope.State { return st }})
	z.SetTapA(scope.StageSynth)
	z.SetWindowMs(20) // disables auto-fit
	z.Layout(image.Rect(0, 0, 600, 200))
	dst := ebiten.NewImage(600, 200)
	z.Draw(dst)
	// fitState is untouched scratch (auto-fit branch skipped) → empty.
	if len(z.fitState.TapA.Samples) != 0 {
		t.Errorf("auto-fit off must not populate fitState, got len=%d", len(z.fitState.TapA.Samples))
	}
}
