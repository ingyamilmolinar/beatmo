//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

// So the transient fills the trace untouched.
func TestChainAutoFitDefaultOn(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	if !z.AutoFit() {
		t.Fatal("auto-fit should default to on")
	}
}

// Manual zoom sticks by disabling auto-fit.
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

func TestChainSetAutoFitReenables(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(40) // disables
	z.SetAutoFit(true)
	if !z.AutoFit() {
		t.Error("SetAutoFit(true) must re-enable auto-fit")
	}
}

// Restores the default framing.
func TestChainDoubleClickRestoresAutoFit(t *testing.T) {
	z := NewChainPanelZone(ChainCallbacks{})
	z.SetWindowMs(40) // disable + non-default window
	h := &chainZoomHandler{zone: z}
	// Within the 300 ms double-click window.
	h.OnPress(0, 0)
	h.OnPress(0, 0)
	if !z.AutoFit() {
		t.Error("double-click must restore auto-fit")
	}
	if z.WindowMs() != 20 {
		t.Errorf("double-click must reset window to 20, got %v", z.WindowMs())
	}
}

// A tiny transient in a large buffer frames to a sub-slice far shorter than the source.
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

	// Framed to the transient plus padding.
	got := len(z.fitState.TapA.Samples)
	if got == 0 {
		t.Fatal("auto-fit produced empty sub-slice")
	}
	if got >= n/2 {
		t.Errorf("auto-fit sub-slice len=%d not tight vs full buffer %d", got, n)
	}
}

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
