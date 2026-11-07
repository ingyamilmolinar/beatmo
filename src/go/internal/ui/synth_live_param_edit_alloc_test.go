//go:build test

package ui

import (
	"image"
	"runtime"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Bytes per frame through the real path (playing, tab open, a real armed spin on the kick's Punch): re-deriving the
// full merged param map every frame churned ~1 MB of garbage, and on the one WASM heap the GC stalls collapsed
// the frame rate. Alloc counts miss it: one merge is a few allocs of 115 KB.
const (
	// Idle: tab open, playing, no knob touched.
	synthTabIdleFrameByteBudget = 128 << 10
	// TotalAlloc also counts the graph worker's off-thread renders, so a drag frame reads far above idle on a
	// multi-core machine; the budget has headroom for that variance.
	synthTabDragFrameByteBudget = 1536 << 10
)

func measureFrameBytes(g *Game, screen *ebiten.Image, frames int, perFrame func(i int)) float64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for i := 0; i < frames; i++ {
		if perFrame != nil {
			perFrame(i)
		}
		_ = g.Update()
		g.Draw(screen)
	}
	runtime.ReadMemStats(&after)
	return float64(after.TotalAlloc-before.TotalAlloc) / float64(frames)
}

func TestSynthTab_LiveControlEditFrameByteBudget(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// dnb-kick, a modular recipe: the largest schema.
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Kick"
	g.drum.Rows[0].Instrument = roster.DnbKick
	if !audio.HasInstrumentDoc(roster.DnbKick) {
		if _, ok := audio.EnsureInstrumentDoc(roster.DnbKick); !ok {
			t.Fatal("dnb-kick has no instrument document — harness assumption broken")
		}
	}
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.DnbKick) })
	g.updateBeatInfos()

	expandSynthPanelForTest(t, g)

	// Opens the owning stage and scrolls the knob into view.
	idx := synthBindingIdxByName(t, g, "gen1_kick_attack")
	selectSectionForKnobIdx(t, g, roster.DnbKick, idx)
	knobs := g.drum.SynthTabControls()
	knob := knobs[idx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("Punch knob (gen1_kick_attack) has empty rect after opening its stage")
	}

	// Through the real play path, so the sequencer runs.
	g.drum.playPressed = true
	screen := ebiten.NewImage(1280, 720)
	for i := 0; i < 30; i++ { // warm-up: caches, glyphs, first mirror render
		_ = g.Update()
		g.Draw(screen)
	}

	idleBytes := measureFrameBytes(g, screen, 60, nil)
	t.Logf("idle synth-tab frame: %.0f KB/frame (budget %d KB)", idleBytes/1024, synthTabIdleFrameByteBudget>>10)

	// An armed spin, the path a user edits a value through.
	g.drum.armSynthControl(roster.DnbKick, idx)
	// One more layout so the armed card grows, and re-fetch the knob: a layout can rebuild the controls wholesale.
	_ = g.Update()
	knobs = g.drum.SynthTabControls()
	knob = knobs[idx]
	e := g.drum.instEditorArmedEditor
	if e == nil {
		t.Fatal("armSynthControl did not create an ArmedControlEditor")
	}
	// So the lane rects exist before measuring, whatever the Draw order.
	e.SetCardRect(knob.Rect())
	valRect := e.ValueLaneRect()
	if valRect.Empty() {
		t.Fatalf("armed editor's value lane rect is empty after SetCardRect(%v)", knob.Rect())
	}
	valCenter := image.Pt((valRect.Min.X+valRect.Max.X)/2, (valRect.Min.Y+valRect.Max.Y)/2)

	hits := g.drum.eqPanelZone.HitAreas()
	var pressHit *HitArea
	for i, h := range hits {
		if h.Tag != "" && strings.HasPrefix(h.Tag, "synth-control-") && valCenter.In(h.Rect) {
			pressHit = &hits[i]
			break
		}
	}
	if pressHit == nil {
		t.Fatalf("no synth-control HitArea covers the armed Punch control's value-lane center %v", valCenter)
	}
	dragBytes := measureFrameBytes(g, screen, 60, func(i int) {
		// Alternating so the value changes every frame; Update ticks the armed editor, which crosses detents and reaches the
		// engine.
		dir := 1
		if (i/8)%2 == 1 {
			dir = -1
		}
		pressHit.Handler.OnWheel(valCenter.X, valCenter.Y, dir)
	})
	t.Logf("drag synth-tab frame: %.0f KB/frame (budget %d KB)", dragBytes/1024, synthTabDragFrameByteBudget>>10)

	// So the measured path is the real one.
	if p := audio.GetInstrumentParams(roster.DnbKick); len(p) == 0 {
		t.Fatal("drag did not propagate any param to the audio engine — measurement exercised nothing")
	}

	stopPlaybackForTest(g)

	if idleBytes > synthTabIdleFrameByteBudget {
		t.Errorf("idle Synth-tab frame allocates %.0f KB — over the %d KB budget. The tab re-derives the full merged param map every frame (buildSynthTab / synthParamsHash); on single-threaded WASM this churn stalls the GC and starves the frame loop.",
			idleBytes/1024, synthTabIdleFrameByteBudget>>10)
	}
	if dragBytes > synthTabDragFrameByteBudget {
		t.Errorf("live knob-drag frame allocates %.0f KB — over the %d KB budget. Every drag frame re-merges the ~1030-param schema several times (mirror hash, preview waves, focus graph); this is the 'editing Punch during playback hangs the game' regression.",
			dragBytes/1024, synthTabDragFrameByteBudget>>10)
	}
}
