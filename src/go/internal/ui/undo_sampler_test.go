//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A real project row, so its sample_edit is exported; laid out so the production handlers are wired.
func setupSamplerForUndo(t *testing.T, g *Game) string {
	t.Helper()
	instID := g.drum.Rows[0].Instrument
	// sampleEdits is package-global in internal/audio: a stale descriptor from an earlier test would sit in the exported
	// baseline while the UI reads clean. Cleared here and on cleanup so the tests are order-independent.
	audio.ClearSampleEdit(instID)
	t.Cleanup(func() { audio.ClearSampleEdit(instID) })
	g.drum.sampler.captureFromSynth(instID)
	if !g.drum.sampler.hasBuffer() || g.drum.sampler.captureID == "" {
		t.Skipf("could not capture a sampler buffer for %q", instID)
	}
	layoutSamplerTab(t, g)
	return string(instID)
}

func clickSamplerButton(t *testing.T, g *Game, tag string) {
	t.Helper()
	btn := g.drum.samplerButtonByTag(tag)
	if btn == nil || btn.OnClick == nil {
		t.Fatalf("sampler button %q not wired", tag)
	}
	btn.OnClick()
}

// A value change plus the debounced wheel commit (bump → flush = one commit), as the ParamCard does; the card has no
// rotary drag.
func samplerKnobRelease(t *testing.T, g *Game, idx int, value float64) {
	t.Helper()
	ks := g.drum.sampler.knobs
	if idx < 0 || idx >= len(ks) || ks[idx] == nil {
		t.Fatalf("sampler knob %d not built", idx)
	}
	ks[idx].Value = value
	dv := g.drum
	dv.applySamplerKnob(idx)
	dv.samplerWheelNudge.bump(func() { dv.commitSamplerEdit() })
	dv.samplerWheelNudge.flush()
}

// Through the production handle adapter, the real trim gesture.
func samplerTrimHandleDrag(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	w := dv.sampler.waveformRect
	if w.Dx() <= 0 {
		t.Fatal("waveform rect empty after layout")
	}
	x := w.Min.X + w.Dx()/4
	y := w.Min.Y + w.Dy()/2
	h := &samplerHandleHitAdapter{dv: dv, which: 0}
	h.OnPress(x, y)
	h.OnRelease(x, y)
}

// Through the value wheel: six detents down live, then Accept, one commit and one step.
func samplerNumericEntry(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobGain)
	w := dv.samplerWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatal("sampler gain wheel did not open")
	}
	for i := 0; i < 6; i++ {
		w.StepValue(-1)
	}
	w.Accept()
}

// A real desktop wheel nudge over the gain card through g.Update → HitIndex → samplerKnobHitAdapter.OnWheel; Undo
// must revert both the document and the Sampler UI state.
func TestSamplerKnobWheelViaTreeIsUndoable(t *testing.T) {
	g := samplerLayoutGame(t) // 1280x720 to match the dispatchWheelAt window
	setupSamplerForUndo(t, g)
	g.Update()

	img := newTrackedImage("test.samplerknobwheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate card rects

	idx := samplerKnobGain
	k := g.drum.sampler.knobs[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("sampler gain card rect empty after draw")
	}

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)
	// Compared against, not hardcoded to 0: the invariant is "undo reverts to baseline".
	gainBefore := g.drum.sampler.gainDB

	// The whole spin collapses into ONE debounced step.
	dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
	if g.drum.sampler.gainDB == gainBefore {
		t.Fatal("real sampler card wheel did not change samplerState.gainDB")
	}
	for f := 0; f < 60; f++ { // run the debounce down → single trailing commit
		g.Update()
	}

	after := g.undoCapture()
	if string(after) == string(before) {
		t.Fatal("real sampler card wheel did not change the exported document")
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("real sampler card wheel recorded %d undo steps, want exactly 1", steps)
	}

	g.undoManager.Undo()
	layoutSamplerTab(t, g)
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("Undo did not restore the pre-wheel document")
	}
	if g.drum.sampler.gainDB != gainBefore {
		t.Fatalf("after undo the sampler knob did not revert: gainDB=%.3f want %.3f", g.drum.sampler.gainDB, gainBefore)
	}
}

// ensureSamplerLoaded keeps a loaded instrument's in-progress edit and never re-seeds from the restored document, so
// after undo the knobs stayed put and the next gesture re-applied the stale edit; the UI state must match the
// reverted document.
func TestUndoSamplerEditRevertsUIState(t *testing.T) {
	t.Run("gain-knob", func(t *testing.T) {
		g := newSamplerTabGame(t)
		setupSamplerForUndo(t, g)
		g.undoManager.OnExternalLoad()

		samplerKnobRelease(t, g, samplerKnobGain, 0.9)
		if g.drum.sampler.gainDB == 0 {
			t.Fatal("precondition: gain gesture should change samplerState.gainDB")
		}

		edited := g.drum.sampler.gainDB

		g.undoManager.Undo()
		layoutSamplerTab(t, g) // the next frame re-lays out the tab

		if g.drum.sampler.gainDB != 0 {
			t.Fatalf("after undo, sampler UI state not reverted: gainDB=%.3f want 0 "+
				"(undo reverted the document but the sampler knob stayed edited)",
				g.drum.sampler.gainDB)
		}

		// Redo must re-apply the edit to the UI too (the resync is symmetric).
		g.undoManager.Redo()
		layoutSamplerTab(t, g)
		if g.drum.sampler.gainDB != edited {
			t.Fatalf("after redo, sampler UI state not re-applied: gainDB=%.3f want %.3f",
				g.drum.sampler.gainDB, edited)
		}
	})

	t.Run("reverse", func(t *testing.T) {
		g := newSamplerTabGame(t)
		setupSamplerForUndo(t, g)
		g.undoManager.OnExternalLoad()

		clickSamplerButton(t, g, "sampler-reverse")
		if !g.drum.sampler.reverse {
			t.Fatal("precondition: reverse gesture should set samplerState.reverse")
		}

		g.undoManager.Undo()
		layoutSamplerTab(t, g)

		if g.drum.sampler.reverse {
			t.Fatal("after undo, sampler reverse state not reverted (buffer/flag stayed reversed)")
		}
	})
}

// Every gesture applies live to the exported document and records exactly one reversible step, through the real
// handler.
func TestUndoSamplerEditGestures(t *testing.T) {
	cases := []struct {
		name string
		do   func(t *testing.T, g *Game)
	}{
		{"reverse", func(t *testing.T, g *Game) { clickSamplerButton(t, g, "sampler-reverse") }},
		{"normalize", func(t *testing.T, g *Game) { clickSamplerButton(t, g, "sampler-normalize") }},
		{"fade", func(t *testing.T, g *Game) { clickSamplerButton(t, g, "sampler-fade") }},
		{"gain-knob", func(t *testing.T, g *Game) { samplerKnobRelease(t, g, samplerKnobGain, 0.9) }},
		{"trim-start-knob", func(t *testing.T, g *Game) { samplerKnobRelease(t, g, samplerKnobStart, 0.3) }},
		{"trim-handle", func(t *testing.T, g *Game) { samplerTrimHandleDrag(t, g) }},
		{"numeric-entry", func(t *testing.T, g *Game) { samplerNumericEntry(t, g) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newSamplerTabGame(t)
			setupSamplerForUndo(t, g)

			g.undoManager.OnExternalLoad()
			before := g.undoCapture()
			depth0 := len(g.undoManager.undo)

			tc.do(t, g)

			assertOneUndoStepRoundTrip(t, g, tc.name, before, depth0)
		})
	}
}
