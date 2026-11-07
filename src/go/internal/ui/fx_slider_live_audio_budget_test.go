//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// A held slider wrote the audio every tick, moved or not, and each write rebuilt the channel's whole WebAudio
// insert chain (a fresh worklet, an empty delay line): kick notes cut. A still slider writes nothing; each
// change is at most one in-place param update; no part of a gesture rebuilds the chain.
func TestHeldFXSliderWritesAudioOnlyOnValueChange(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 2)
	const row = 1 // the kick row
	inst := g.drum.Rows[row].Instrument
	prev := audio.GetInsertEffects(inst)
	t.Cleanup(func() { audio.SetInsertEffects(inst, prev) })
	audio.SetInsertEffects(inst, nil)
	audio.AddInsertEffect(inst, audio.EffectTape, nil)
	audio.AddInsertEffect(inst, audio.EffectPitchShift, nil)

	const w, h = 1280, 720
	g.Layout(w, h)
	var px, py int
	var down bool
	restore := SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	frame := func(p image.Point, pressed bool) {
		px, py, down = p.X, p.Y, pressed
		_ = g.Update()
	}
	mid := func(r image.Rectangle) image.Point {
		return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	}
	advanceFrames(g, 3)

	// Open the kick row's FX panel the way a pointer does: hover, press, release.
	btns := g.drum.rowFXBtns()
	if row >= len(btns) || btns[row] == nil {
		t.Fatalf("row %d has no FX button", row)
	}
	fx := mid(btns[row].Rect())
	frame(fx, false)
	frame(fx, true)
	frame(fx, false)
	frame(fx, false)
	if !g.drum.IsFXPanelOpen() || g.drum.FXPanelRow() != row {
		t.Fatalf("FX panel not open on row %d (open=%v row=%d)", row, g.drum.IsFXPanelOpen(), g.drum.FXPanelRow())
	}

	idx := -1
	for i, b := range g.drum.fxSliderBindings {
		if b.slotIndex == 1 && b.paramName == "pitch" {
			idx = i
		}
	}
	if idx < 0 || idx >= len(g.drum.fxPanelSliders) {
		t.Fatalf("no Pitch slider for the Pitch Shifter (bindings %+v)", g.drum.fxSliderBindings)
	}
	thumb := mid(g.drum.fxPanelSliders[idx].Rect())
	frame(thumb, false)
	frame(thumb, true)
	if !g.drum.fxSliderDragging {
		t.Fatal("pressing the Pitch slider did not start a drag")
	}

	s0, p0 := audio.InsertFXPushCounts()
	for i := 0; i < 30; i++ {
		frame(thumb, true)
	}
	s1, p1 := audio.InsertFXPushCounts()
	if p1 != p0 || s1 != s0 {
		t.Errorf("holding the slider still for 30 frames wrote the audio graph %d times (%d whole-chain rebuilds); want 0",
			(p1-p0)+(s1-s0), s1-s0)
	}

	const moves = 8
	for i := 1; i <= moves; i++ {
		frame(thumb.Add(image.Pt(6*i, 0)), true)
	}
	_, p2 := audio.InsertFXPushCounts()
	if p2-p1 < 1 || p2-p1 > moves {
		t.Errorf("%d slider moves made %d in-place param updates; want 1..%d (one per value change)", moves, p2-p1, moves)
	}

	frame(thumb.Add(image.Pt(6*moves, 0)), false)
	frame(thumb.Add(image.Pt(6*moves, 0)), false)
	if s3, _ := audio.InsertFXPushCounts(); s3 != s0 {
		t.Errorf("one param gesture rebuilt the insert chain %d times; a param edit must update the live effect in place", s3-s0)
	}
	if g.drum.fxSliderDragging {
		t.Error("slider still dragging after release")
	}
}

// The curve handle's OnDrag runs every captured frame and must write the channel EQ only when the value moves.
func TestHeldEQHandleWritesAudioOnlyOnValueChange(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 2)
	const w, h = 1280, 720
	g.Layout(w, h)
	var px, py int
	var down bool
	restore := SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	frame := func(p image.Point, pressed bool) {
		px, py, down = p.X, p.Y, pressed
		_ = g.Update()
	}
	g.split.Y = 287 // the app's default grid pane height at 1280x720
	z := g.drum.eqPanelZone
	if z.ActiveTab() != TabEQ {
		z.SetActiveTab(TabEQ)
	}
	advanceFrames(g, 3)
	// The 2-row test circuit leaves the panel 32 px tall, too little for a handle to travel.
	g.drum.widgets.ResizeAxis("row", 1, -120)
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	advanceFrames(g, 3)
	if pr := z.eqPlotRect(); pr.Dy() < 60 {
		t.Fatalf("the EQ plot is %v — too short to drag a band", pr)
	}

	const band = 5
	hx, hy := z.eqBandHandlePos(band)
	handle := image.Pt(hx, hy)
	frame(handle, false)
	frame(handle, true)
	if z.curveDragBand != band {
		t.Fatalf("pressing band %d's handle did not grab it (curveDragBand=%d)", band, z.curveDragBand)
	}
	frame(handle.Add(image.Pt(0, -20)), true) // off 0 dB

	w0 := audio.ChannelEQWriteCount()
	for i := 0; i < 30; i++ {
		frame(handle.Add(image.Pt(0, -20)), true)
	}
	if n := audio.ChannelEQWriteCount() - w0; n != 0 {
		t.Errorf("holding an EQ handle still for 30 frames wrote the channel EQ %d times; want 0", n)
	}

	const moves = 6
	w1 := audio.ChannelEQWriteCount()
	for i := 1; i <= moves; i++ {
		frame(handle.Add(image.Pt(0, -20-4*i)), true)
	}
	if n := audio.ChannelEQWriteCount() - w1; n < 1 || n > moves {
		t.Errorf("%d handle moves wrote the channel EQ %d times; want 1..%d (one per value change)", moves, n, moves)
	}
	frame(handle.Add(image.Pt(0, -20-4*moves)), false)
}
