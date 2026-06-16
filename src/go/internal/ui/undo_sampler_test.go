//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// setupSamplerForUndo captures a synth-source buffer on row-0's instrument (a
// real project row, so its sample_edit is exported) and lays out the Sampler
// tab so the production knob/handle/button handlers are wired. Returns the
// captured instrument id.
func setupSamplerForUndo(t *testing.T, g *Game) string {
	t.Helper()
	instID := g.drum.Rows[0].Instrument
	g.drum.sampler.captureFromSynth(instID)
	if !g.drum.sampler.hasBuffer() || g.drum.sampler.captureID == "" {
		t.Skipf("could not capture a sampler buffer for %q", instID)
	}
	layoutSamplerTab(t, g)
	return instID
}

func clickSamplerButton(t *testing.T, g *Game, tag string) {
	t.Helper()
	btn := g.drum.samplerButtonByTag(tag)
	if btn == nil || btn.OnClick == nil {
		t.Fatalf("sampler button %q not wired", tag)
	}
	btn.OnClick()
}

// samplerKnobRelease drives a knob to a new value and fires the production
// release handler (samplerKnobHitAdapter.OnRelease), which is where a gesture's
// single undo step must be committed.
func samplerKnobRelease(t *testing.T, g *Game, idx int, value float64) {
	t.Helper()
	ks := g.drum.sampler.knobs
	if idx < 0 || idx >= len(ks) || ks[idx] == nil {
		t.Fatalf("sampler knob %d not built", idx)
	}
	ks[idx].Value = value
	h := &samplerKnobHitAdapter{dv: g.drum, idx: idx, active: true}
	h.OnRelease(0, 0)
}

// samplerTrimHandleDrag drives the start trim handle through the production
// handle adapter (press → release), the real "trim the signal" gesture.
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

// samplerNumericEntry opens the shared value editor over a knob readout and
// commits a typed value through the production editor commit.
func samplerNumericEntry(t *testing.T, g *Game) {
	t.Helper()
	dv := g.drum
	dv.openSamplerParamEditor(samplerKnobGain)
	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("sampler numeric editor did not open")
	}
	dv.paramEditor.ti.SetText("-6")
	dv.paramEditor.commit()
}

// TestSamplerKnobDragViaTreeIsUndoable is the end-to-end functional proof: it
// drives a real gain-knob drag through the PRODUCTION input path (SetInputForTest
// → g.Update → HitIndex → samplerKnobHitAdapter press/drag/release), then undoes
// it — asserting the gesture changed the document, recorded exactly one step,
// and that Undo reverted BOTH the document and the sampler UI state.
func TestSamplerKnobDragViaTreeIsUndoable(t *testing.T) {
	g := newSamplerTabGame(t)
	setupSamplerForUndo(t, g)
	g.Update()

	img := newTrackedImage("test.samplerknobdrag", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate knob rects

	idx := samplerKnobGain
	k := g.drum.sampler.knobs[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("sampler gain knob rect empty after draw")
	}
	// Press on the DIAL (top knobD×knobD square), not the cell center — the cell
	// rect also spans the caption/readout band, where a press opens the numeric
	// editor instead of starting a drag.
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dx()/2
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := cx, cy
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	g.undoManager.OnExternalLoad()
	before := g.undoCapture()
	depth0 := len(g.undoManager.undo)

	pressed = true
	g.Update() // OnPress
	for f := 1; f <= 5; f++ {
		mouseX = cx + f*12 // horizontal drag turns the knob
		g.Update()
	}
	pressed = false
	g.Update() // OnRelease → commit

	if g.drum.sampler.gainDB == 0 {
		t.Fatal("real knob drag did not change samplerState.gainDB")
	}
	after := g.undoCapture()
	if string(after) == string(before) {
		t.Fatal("real sampler knob drag did not change the exported document")
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("real sampler knob drag recorded %d undo steps, want exactly 1", steps)
	}

	g.undoManager.Undo()
	layoutSamplerTab(t, g)
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("Undo did not restore the pre-drag document")
	}
	if g.drum.sampler.gainDB != 0 {
		t.Fatalf("after undo the sampler knob did not revert: gainDB=%.3f want 0", g.drum.sampler.gainDB)
	}
}

// TestUndoSamplerEditRevertsUIState reproduces the real-app bug: undo reverts the
// exported document, but the Sampler tab's editing state (samplerState knobs /
// trim / reverse) stayed edited because ensureSamplerLoaded preserves an
// already-loaded instrument's in-progress edit and never re-seeds from the
// restored document. So the knobs don't move back and the next gesture
// re-applies the stale edit. After undo (+ the next layout) the UI state must
// match the reverted document.
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

// TestUndoSamplerEditGestures proves every Sampler-tab edit gesture (trim,
// knobs, reverse/normalize/fade, numeric entry) applies live to the exported
// document and records exactly one reversible undo step per gesture. Each case
// drives the REAL production handler.
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
