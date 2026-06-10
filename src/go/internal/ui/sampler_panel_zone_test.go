//go:build test

package ui

import (
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func newSamplerTabGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	return g
}

func layoutSamplerTab(t *testing.T, g *Game) {
	t.Helper()
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
}

func TestSamplerTabBuildsKnobsAndButtons(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick") // knobs only lay out once a buffer exists
	layoutSamplerTab(t, g)
	dv := g.drum

	if len(dv.sampler.knobs) != samplerKnobCount {
		t.Fatalf("got %d knobs, want %d", len(dv.sampler.knobs), samplerKnobCount)
	}
	for i, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			t.Errorf("knob %d has empty rect", i)
		}
	}
	for _, tag := range []string{"sampler-load-wav", "sampler-preview", "sampler-save", "sampler-save-as"} {
		if dv.samplerButtonByTag(tag) == nil {
			t.Errorf("missing button %q", tag)
		}
	}
}

func TestSamplerTabReusesKnobsAcrossLayout(t *testing.T) {
	g := newSamplerTabGame(t)
	layoutSamplerTab(t, g)
	first := append([]*Knob(nil), g.drum.sampler.knobs...)
	layoutSamplerTab(t, g)
	for i := range first {
		if first[i] != g.drum.sampler.knobs[i] {
			t.Errorf("knob %d recreated across layout (want stable instance to preserve drags)", i)
		}
	}
}

func TestSamplerTabHitAreasSitAbovePanelCatchAll(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick") // load a buffer so handles appear
	layoutSamplerTab(t, g)

	areas := g.drum.samplerTabHitAreas()
	if len(areas) == 0 {
		t.Fatal("no sampler hit areas")
	}
	var sawKnob, sawHandle, sawButton bool
	for _, a := range areas {
		if a.ZIndex <= ZEQPanel {
			t.Errorf("hit area %q at z=%d must sit above the panel catch-all (z=%d)", a.Tag, a.ZIndex, ZEQPanel)
		}
		switch {
		case strings.HasPrefix(a.Tag, "sampler-handle"):
			sawHandle = true
		case strings.HasPrefix(a.Tag, "sampler-knob"):
			sawKnob = true
		case a.Tag == "sampler-save":
			sawButton = true
		}
	}
	if !sawKnob || !sawHandle || !sawButton {
		t.Errorf("expected knob+handle+button areas, got knob=%v handle=%v button=%v", sawKnob, sawHandle, sawButton)
	}
}

func TestSamplerTabHitAreasFlowFromEQPanel(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)

	areas := g.drum.eqPanelZone.HitAreas()
	var sawCatchAll, sawSampler bool
	for _, a := range areas {
		switch a.Tag {
		case "eq-panel-capture":
			sawCatchAll = true
		case "sampler-save":
			sawSampler = true
		}
	}
	if !sawCatchAll {
		t.Error("EQ panel must still publish its opaque catch-all when TabSampler is active")
	}
	if !sawSampler {
		t.Error("EQ panel must append sampler hit areas when TabSampler is active")
	}
}

func TestSamplerPreviewUsesPlayPath(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")

	var played []string
	restore := SwapSamplerAuditionFnForTest(func(id string) { played = append(played, id) })
	defer SwapSamplerAuditionFnForTest(restore)

	g.drum.samplerPreview()
	if len(played) != 1 || played[0] != samplerPreviewID {
		t.Errorf("preview played %v, want exactly [%s]", played, samplerPreviewID)
	}
}

func TestSamplerHandleDragSetsTrim(t *testing.T) {
	g := newSamplerTabGame(t)
	g.drum.sampler.captureFromSynth("kick")
	layoutSamplerTab(t, g)
	dv := g.drum

	w := dv.sampler.waveformRect
	if w.Dx() <= 0 {
		t.Fatal("waveform rect empty after layout")
	}
	// Drag the start handle to ~25% and the end handle to ~75%.
	dv.samplerHandleDrag(0, w.Min.X+w.Dx()/4)
	dv.samplerHandleDrag(1, w.Min.X+w.Dx()*3/4)
	if dv.sampler.startFrac < 0.2 || dv.sampler.startFrac > 0.3 {
		t.Errorf("start handle drag → startFrac=%.2f, want ~0.25", dv.sampler.startFrac)
	}
	if dv.sampler.endFrac < 0.7 || dv.sampler.endFrac > 0.8 {
		t.Errorf("end handle drag → endFrac=%.2f, want ~0.75", dv.sampler.endFrac)
	}
}

func TestSamplerLoadWAVPopulatesRaw(t *testing.T) {
	g := newSamplerTabGame(t)
	fixture := []float32{0.1, -0.2, 0.3, -0.4, 0.5}
	restore := SwapSamplerWAVLoadFnForTest(func() ([]float32, int, bool) {
		return fixture, 44100, true
	})
	defer SwapSamplerWAVLoadFnForTest(restore)

	g.drum.samplerLoadWAV()
	if len(g.drum.sampler.raw) != len(fixture) {
		t.Fatalf("raw len = %d, want %d after Load WAV", len(g.drum.sampler.raw), len(fixture))
	}
	if g.drum.sampler.source != samplerSourceWAV {
		t.Errorf("source = %v, want samplerSourceWAV", g.drum.sampler.source)
	}
	if g.drum.sampler.rawSampleRate != 44100 {
		t.Errorf("rawSampleRate = %d, want 44100", g.drum.sampler.rawSampleRate)
	}
}

func TestSamplerLoadWAVCancelLeavesBufferEmpty(t *testing.T) {
	g := newSamplerTabGame(t)
	restore := SwapSamplerWAVLoadFnForTest(func() ([]float32, int, bool) {
		return nil, 0, false // user cancelled
	})
	defer SwapSamplerWAVLoadFnForTest(restore)

	g.drum.samplerLoadWAV()
	if g.drum.sampler.hasBuffer() {
		t.Error("cancelled Load WAV should leave the buffer empty")
	}
}
