//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func TestApplyFreezeVisualUsesIcons(t *testing.T) {
	b := NewButton("||", InstButtonStyle, nil)

	applyFreezeVisual(b, false) // live → pause icon (click to pause)
	if b.Icon != string(IconPause) {
		t.Fatalf("live: Icon=%q, want %q", b.Icon, IconPause)
	}
	if b.Text != "" {
		t.Fatalf("live: Text=%q, want empty (icon-only)", b.Text)
	}

	applyFreezeVisual(b, true) // frozen → play icon (click to resume)
	if b.Icon != string(IconPlay) {
		t.Fatalf("frozen: Icon=%q, want %q", b.Icon, IconPlay)
	}
	if b.Text != "" {
		t.Fatalf("frozen: Text=%q, want empty", b.Text)
	}
}

func TestToggleTabFreezeIsPerTabIndependent(t *testing.T) {
	live1 := &analyzer.State{Timestamp: 1}
	live2 := &analyzer.State{Timestamp: 2}
	cur := live1
	z := &EQPanelZone{}
	z.callbacks.AnalyzerState = func() *analyzer.State { return cur }

	if got := z.toggleTabFreeze(TabWave); !got {
		t.Fatalf("toggleTabFreeze(TabWave) = false, want true (froze)")
	}
	cur = live2 // live advances

	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabWave); s != live1 {
		t.Fatalf("frozen Wave returned %v, want live1", s)
	}
	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabSpectrum); s != live2 {
		t.Fatalf("live Spectrum returned %v, want live2", s)
	}

	if got := z.toggleTabFreeze(TabWave); got {
		t.Fatalf("second toggle = true, want false (unfroze)")
	}
	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabWave); s != live2 {
		t.Fatalf("unfrozen Wave returned %v, want live2", s)
	}
}

func TestWaveControlsHasBothAutoAndFreeze(t *testing.T) {
	autoCalls, freezeCalls := 0, 0
	c := newWaveControls(5,
		func() bool { autoCalls++; return autoCalls%2 == 1 },
		func() bool { freezeCalls++; return freezeCalls%2 == 1 },
	)
	c.Layout(image.Rect(0, 0, 300, 26))

	tags := map[string]bool{}
	for _, h := range c.HitAreas() {
		tags[h.Tag] = true
	}
	if !tags["wave-auto-btn"] {
		t.Fatalf("missing wave-auto-btn; tags=%v", tags)
	}
	if !tags["wave-freeze-btn"] {
		t.Fatalf("missing wave-freeze-btn; tags=%v", tags)
	}

	for _, h := range c.HitAreas() {
		if h.Tag == "wave-freeze-btn" {
			h.Handler.OnPress(h.Rect.Min.X+1, h.Rect.Min.Y+1)
		}
	}
	if freezeCalls != 1 {
		t.Fatalf("freeze OnPress fired %d times, want 1", freezeCalls)
	}

	c.SyncFreeze(true)
	if c.freezeBtn == nil || c.freezeBtn.Icon != string(IconPlay) {
		t.Fatalf("SyncFreeze(true) did not set play icon")
	}
}

func TestLevelsLatchHoldsWhenFrozen(t *testing.T) {
	frozenSnap := &analyzer.State{
		Master: analyzer.ChannelMetrics{ID: "main", PeakDB: -6, RMSDB: -12, Active: true},
	}
	z := &EQPanelZone{}
	z.frozenByTab = map[PanelTab]*analyzer.State{TabMeters: frozenSnap}
	z.levelsLatches = NewMultiLevelsLatch()

	z.applyLevelsFreezeHold(frozenSnap)
	first := z.levelsLatches.Get("main").PeakHoldDB
	z.applyLevelsFreezeHold(frozenSnap)
	second := z.levelsLatches.Get("main").PeakHoldDB

	if first != second {
		t.Fatalf("frozen latch drifted: first=%v second=%v", first, second)
	}
	if first != -6 {
		t.Fatalf("frozen latch PeakHoldDB=%v, want -6 (held at frozen value)", first)
	}
}

func TestWaveFrozenTagSignature(t *testing.T) {
	t.Cleanup(SetWaveTraceCacheForTest(false))
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: make([]float64, 64)}
	for i := range ch.Waveform {
		ch.Waveform[i] = 0.3
	}
	for _, tabFrozen := range []bool{false, true} {
		img := ebiten.NewImage(200, 100)
		rects := collectFilledRects(t, func() {
			drawAnalyzerWaveform(img, img.Bounds(), ch, nil, nil, 1.0, true, tabFrozen)
		})
		if rectsWithColorInside(rects, img.Bounds(), colWaveTrace) == 0 {
			t.Fatalf("tabFrozen=%v: no colWaveTrace rects drawn", tabFrozen)
		}
	}
}
