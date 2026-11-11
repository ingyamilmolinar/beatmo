package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

func TestResolveWaveGainSmoothsAcrossRefreshes(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: true, waveYGain: 1.0}

	loud := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.9, -0.9}}}
	g1, auto1 := z.resolveWaveGain(loud, &loud.Master, nil)
	if !auto1 {
		t.Fatalf("expected auto on")
	}
	if math.Abs(g1-1.0) > 1e-6 { // peak 0.9 -> 0.9/0.9 = 1.0
		t.Fatalf("loud gain = %v, want 1.0", g1)
	}

	quiet := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.1, -0.1}}}
	g2, _ := z.resolveWaveGain(quiet, &quiet.Master, nil)
	if g2 <= g1 {
		t.Fatalf("quiet gain %v should exceed loud gain %v", g2, g1)
	}
	if g2 >= 9.0 {
		t.Fatalf("quiet gain %v should be smoothed below the 9x steady-state on first refresh", g2)
	}

	g3, _ := z.resolveWaveGain(quiet, &quiet.Master, nil)
	if math.Abs(g3-g2) > 1e-9 {
		t.Fatalf("same-pointer refresh changed gain: %v -> %v", g2, g3)
	}
}

func TestResolveWaveGainManualIgnoresAuto(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: false, waveYGain: 3.5}
	st := &analyzer.State{Master: analyzer.ChannelMetrics{Active: true, Waveform: []float64{0.05}}}
	g, auto := z.resolveWaveGain(st, &st.Master, nil)
	if auto {
		t.Fatalf("expected auto off")
	}
	if math.Abs(g-3.5) > 1e-9 {
		t.Fatalf("manual gain = %v, want 3.5", g)
	}
}

// TestWaveControlsAutoPillFires verifies the AUTO pill toggle fires. The Wave
// tab now also carries a freeze pill — that is covered by
// TestWaveControlsHasBothAutoAndFreeze (freeze_pause_play_test.go).
func TestWaveControlsAutoPillFires(t *testing.T) {
	autoToggled := false
	c := newWaveControls(5,
		func() bool { autoToggled = !autoToggled; return autoToggled },
		func() bool { return false },
	)
	c.Layout(image.Rect(0, 0, 300, 26))

	foundAuto := false
	for _, h := range c.HitAreas() {
		if h.Tag == "wave-auto-btn" {
			foundAuto = true
			h.Handler.OnPress(h.Rect.Min.X+1, h.Rect.Min.Y+1)
		}
	}
	if !foundAuto {
		t.Fatalf("expected a wave-auto-btn hit area")
	}
	if !autoToggled {
		t.Fatalf("AUTO pill OnPress did not fire the toggle callback")
	}

	// Interface compliance: SyncFreeze + SyncAuto must not panic.
	c.SyncFreeze(true)
	c.SyncAuto(false)
}

func TestWaveWheelZoomDisablesAuto(t *testing.T) {
	z := &EQPanelZone{waveAutoGain: true, waveYGain: 1.0}
	h := &waveZoomHandler{zone: z}

	if res := h.OnWheel(10, 10, 2); res != InputConsumed {
		t.Fatalf("OnWheel result = %v, want InputConsumed", res)
	}
	if z.waveAutoGain {
		t.Fatalf("wheel zoom must disable auto-gain")
	}
	if z.waveYGain <= 1.0 {
		t.Fatalf("scroll-up should increase yGain, got %v", z.waveYGain)
	}

	for i := 0; i < 100; i++ {
		h.OnWheel(10, 10, 5)
	}
	if z.waveYGain > 16.0 {
		t.Fatalf("yGain exceeded clamp: %v", z.waveYGain)
	}

	for i := 0; i < 200; i++ {
		h.OnWheel(10, 10, -5)
	}
	if z.waveYGain < 0.25 {
		t.Fatalf("yGain below clamp: %v", z.waveYGain)
	}
}
