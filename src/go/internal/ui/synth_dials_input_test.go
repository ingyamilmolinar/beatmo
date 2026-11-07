//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// Through real input: a drag sets the audio layer to exactly Turn(start, target) and commits once, a tap opens the
// wheel, a tap on the card body drills, and the reading is a pure function of the document.

// newDialGame is the violin macro game with the Synth tab open at the root.
func newDialGame(t *testing.T) *Game {
	t.Helper()
	return openDialRoot(t, newViolinMacroGame(t))
}

// newDialGameFor is newDialGame on another instrument.
func newDialGameFor(t *testing.T, inst string) *Game {
	t.Helper()
	g := newViolinMacroGame(t)
	g.drum.Rows[0].Instrument = instrument.ID(inst)
	t.Cleanup(func() { audio.ResetInstrumentParams(instrument.ID(inst)) })
	return openDialRoot(t, g)
}

func openDialRoot(t *testing.T, g *Game) *Game {
	t.Helper()
	dv := g.drum
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.SetActiveChannel(channel.Of(dv.Rows[0].Instrument))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	idleFrame(g)
	if !dv.synthAtMacroRoot(instrument.ID(dialInst(g))) {
		t.Fatal("the Synth tab did not open at the category root")
	}
	return g
}

func dialInst(g *Game) string {
	return string(g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument()))
}

// dialRect finds a dial's laid-out screen rect on the root.
func dialRect(t *testing.T, g *Game, card, dial string) image.Rectangle {
	t.Helper()
	for _, c := range g.drum.instEditorMacroCards {
		for j := 0; j < c.nDials; j++ {
			if c.id == card && c.dials[j].id == dial && !c.dialRects[j].Empty() {
				return c.dialRects[j]
			}
		}
	}
	t.Fatalf("dial %s/%s is not laid out on the root (cards: %d)", card, dial, len(g.drum.instEditorMacroCards))
	return image.Rectangle{}
}

func cardRect(t *testing.T, g *Game, card string) image.Rectangle {
	t.Helper()
	for _, c := range g.drum.instEditorMacroCards {
		if c.id == card {
			return c.r
		}
	}
	t.Fatalf("card %s not laid out", card)
	return image.Rectangle{}
}

// With the given keys held.
func frameAt(g *Game, x, y int, down bool, keys ...ebiten.Key) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool {
			for _, want := range keys {
				if k == want {
					return true
				}
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
}

// idleFrame parks the pointer off the panel for one frame.
func idleFrame(g *Game) { frameAt(g, 1, 1, false) }

// dragFrom presses at p, moves vertically by dy in steps, and releases.
func dragFrom(g *Game, p image.Point, dy int) {
	frameAt(g, p.X, p.Y, true)
	step := 6
	if dy < 0 {
		step = -6
	}
	y := p.Y
	for (dy > 0 && y < p.Y+dy) || (dy < 0 && y > p.Y+dy) {
		y += step
		frameAt(g, p.X, y, true)
	}
	frameAt(g, p.X, p.Y+dy, true)
	frameAt(g, p.X, p.Y+dy, false)
	idleFrame(g)
}

func tapAtPoint(g *Game, p image.Point) {
	frameAt(g, p.X, p.Y, true)
	frameAt(g, p.X, p.Y, false)
	idleFrame(g)
}

func centre(r image.Rectangle) image.Point {
	return image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
}

// Through the real shortcut path, then a frame.
func keyTap(g *Game, k ebiten.Key) {
	pressKeyThroughShortcuts(g, k)
	idleFrame(g)
}

// The laid-out slot the frame just refreshed.
func dialSlotReading(t *testing.T, g *Game, card, dial string) int {
	t.Helper()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	for _, c := range g.drum.instEditorMacroCards {
		for j := 0; j < c.nDials; j++ {
			if c.id == card && c.dials[j].id == dial {
				return c.dials[j].reading
			}
		}
	}
	t.Fatalf("dial %s/%s is not on the root", card, dial)
	return -1
}

func mustDial(t *testing.T, g *Game, card, dial string) synthdial.Resolved {
	t.Helper()
	d, ok := g.drum.synthDialByID(card, dial)
	if !ok {
		t.Fatalf("%s has no %s/%s dial", dialInst(g), card, dial)
	}
	return d
}

func dialStart(g *Game, d synthdial.Resolved) map[string]float64 {
	cur := audio.MergedInstrumentParamsRO(instrument.ID(dialInst(g)))
	out := map[string]float64{}
	for _, tm := range d.Terms {
		out[tm.Param] = cur[tm.Param]
	}
	return out
}

// Target = press reading + drag travel; ONE undo step, never a drill or an audition.
func TestSynthDialDragAppliesTurnExactlyAndCommitsOnce(t *testing.T) {
	g := newDialGame(t)
	dv := g.drum
	inst := dialInst(g)
	auditions := 0
	prev := SwapSynthAuditionFnForTest(func(instrument.ID) { auditions++ })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	d := mustDial(t, g, "brightness", "bright")
	start := dialStart(g, d)
	r0 := synthDialPercent(d.Reading(start))
	if r0 > 60 {
		t.Fatalf("premise: violin Bright reads %d, need room to turn up 40", r0)
	}
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)

	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -80) // up 80 px = +40

	want := d.Turn(start, float64(r0+40)/100)
	got := audio.MergedInstrumentParamsRO(instrument.ID(inst))
	for p, v := range want {
		if got[p] != v {
			t.Errorf("%s = %v in the audio layer, want exactly Turn's %v", p, got[p], v)
		}
	}
	if r := dialSlotReading(t, g, "brightness", "bright"); r != r0+40 {
		t.Errorf("the dial shows %d after the drag, want %d", r, r0+40)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("one dial gesture recorded %d undo steps, want 1", steps)
	}
	if !dv.synthAtMacroRoot(instrument.ID(inst)) {
		t.Fatal("a press on a dial must never drill into its card")
	}
	if auditions != 0 {
		t.Fatalf("turning a dial auditioned %d notes; dials must stay silent", auditions)
	}
}

// The press-reading magnet writes the start values back bit for bit and records nothing.
func TestSynthDialDragBackRestoresExactly(t *testing.T) {
	g := newDialGame(t)
	inst := dialInst(g)
	before := audio.GetInstrumentParams(instrument.ID(inst))
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	p := centre(dialRect(t, g, "brightness", "bright"))
	frameAt(g, p.X, p.Y, true)
	for y := p.Y; y >= p.Y-80; y -= 8 {
		frameAt(g, p.X, y, true)
	}
	if paramsEqual(audio.GetInstrumentParams(instrument.ID(inst)), before) {
		t.Fatal("the drag away moved nothing — the test would pass vacuously")
	}
	for y := p.Y - 80; y <= p.Y+1; y += 8 {
		frameAt(g, p.X, y, true)
	}
	frameAt(g, p.X, p.Y+1, true)
	frameAt(g, p.X, p.Y+1, false)
	idleFrame(g)
	if after := audio.GetInstrumentParams(instrument.ID(inst)); !paramsEqual(after, before) {
		t.Fatalf("back at the press point the params differ:\n before %v\n after  %v", before, after)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 0 {
		t.Fatalf("a drag that came back recorded %d undo steps", steps)
	}
}

// Arrows move the reading, Esc restores verbatim with no record, Enter after a change commits once; a tap off the
// dials drills.
func TestSynthDialTapOpensTheWheelAndCardTapDrills(t *testing.T) {
	g := newDialGame(t)
	dv := g.drum
	inst := dialInst(g)
	before := audio.GetInstrumentParams(instrument.ID(inst))
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	r0 := dialSlotReading(t, g, "brightness", "edge")

	tapAtPoint(g, centre(dialRect(t, g, "brightness", "edge")))
	if dv.ValueWheelPopupRect().Empty() {
		t.Fatal("a tap on a dial must open the value wheel")
	}
	if !dv.synthAtMacroRoot(instrument.ID(inst)) {
		t.Fatal("a tap on a dial drilled into its card")
	}
	keyTap(g, ebiten.KeyArrowUp)
	keyTap(g, ebiten.KeyArrowUp)
	if r := dialSlotReading(t, g, "brightness", "edge"); r <= r0 {
		t.Fatalf("arrow-up in the dial's wheel did not raise the dial (%d → %d)", r0, r)
	}
	keyTap(g, ebiten.KeyEscape)
	if !dv.ValueWheelPopupRect().Empty() {
		t.Fatal("Esc did not close the wheel")
	}
	if after := audio.GetInstrumentParams(instrument.ID(inst)); !paramsEqual(after, before) {
		t.Fatalf("Esc must restore verbatim:\n before %v\n after  %v", before, after)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 0 {
		t.Fatalf("a cancelled wheel recorded %d undo steps", steps)
	}

	tapAtPoint(g, centre(dialRect(t, g, "brightness", "edge")))
	keyTap(g, ebiten.KeyArrowUp)
	keyTap(g, ebiten.KeyEnter)
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("an accepted wheel recorded %d undo steps, want 1", steps)
	}

	cr := cardRect(t, g, "brightness")
	tapAtPoint(g, image.Pt(cr.Min.X+12, cr.Min.Y+8)) // the title, off the dials
	if dv.synthAtMacroRoot(instrument.ID(inst)) {
		t.Fatal("a tap on the card body must drill into the card")
	}
}

// The reading projects the new values; nothing is remembered.
func TestRawKnobEditMovesTheReading(t *testing.T) {
	g := newDialGame(t)
	inst := dialInst(g)
	d := mustDial(t, g, "brightness", "bright")
	r0 := dialSlotReading(t, g, "brightness", "bright")
	audio.SetInstrumentParam(instrument.ID(inst), "filter_cutoff", 300)
	idleFrame(g)
	want := synthDialPercent(d.Reading(audio.MergedInstrumentParamsRO(instrument.ID(inst))))
	if got := dialSlotReading(t, g, "brightness", "bright"); got != want || got >= r0 {
		t.Fatalf("after a cutoff of 300 Hz Bright shows %d, want %d (below %d)", got, want, r0)
	}
}

// Undo + redo, leaving and returning to the tab, and a fresh Game importing the export all show the same readings.
func TestSynthDialReadingIsAFunctionOfTheDocument(t *testing.T) {
	g := newDialGame(t)
	inst := dialInst(g)
	type key struct{ card, dial string }
	keys := []key{{"brightness", "bright"}, {"brightness", "edge"}, {"length", "tail"}}
	for i, k := range keys {
		dragFrom(g, centre(dialRect(t, g, k.card, k.dial)), -30-10*i)
	}
	snapshot := func(g *Game) map[key]int {
		out := map[key]int{}
		for _, k := range keys {
			out[k] = dialSlotReading(t, g, k.card, k.dial)
		}
		return out
	}
	want := snapshot(g)
	file := mustExport(t, g)

	// (b) undo + redo on the original game.
	g.performUndo()
	idleFrame(g)
	if got := snapshot(g); fmt.Sprint(got) == fmt.Sprint(want) {
		t.Fatal("undo changed no reading — the redo check would pass vacuously")
	}
	g.performRedo()
	idleFrame(g)
	if got := snapshot(g); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after undo + redo the dials read %v, want %v", got, want)
	}

	// (c) away from the Synth tab and back.
	g.drum.eqPanelZone.SetActiveTab(TabSpectrum)
	idleFrame(g)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	idleFrame(g)
	if got := snapshot(g); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after leaving and reopening the tab the dials read %v, want %v", got, want)
	}

	// Last: the instrument store is process-global, so the second Game resets what the first plays.
	audio.ResetInstrumentParams(instrument.ID(inst))
	g2 := newDialGame(t)
	if err := g2.Import(file); err != nil {
		t.Fatal(err)
	}
	idleFrame(g2)
	g2 = openDialRoot(t, g2)
	if got := snapshot(g2); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("a fresh Game importing the export reads %v, want %v", got, want)
	}
}

// Violin Drive reads 0, so dragging down writes and records nothing.
func TestDialAtItsEdgeCommitsNothing(t *testing.T) {
	g := newDialGame(t)
	inst := dialInst(g)
	// Set rather than assumed: whether the shipped violin carries a little drive is a tuning decision.
	audio.SetInstrumentParam(instrument.ID(inst), "drive", 0)
	t.Cleanup(func() { audio.ResetInstrumentParams(instrument.ID(inst)) })
	idleFrame(g)
	if r := dialSlotReading(t, g, "warmth", "drive"); r != 0 {
		t.Fatalf("premise: violin Drive reads %d, want 0 (clean)", r)
	}
	before := audio.GetInstrumentParams(instrument.ID(inst))
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	dragFrom(g, centre(dialRect(t, g, "warmth", "drive")), 80) // DOWN
	if after := audio.GetInstrumentParams(instrument.ID(inst)); !paramsEqual(after, before) {
		t.Fatal("a drag below 0 wrote params")
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 0 {
		t.Fatalf("a drag below 0 recorded %d undo steps", steps)
	}
	dragFrom(g, centre(dialRect(t, g, "warmth", "drive")), -80) // and UP drives
	if audio.MergedInstrumentParamsRO(instrument.ID(inst))["drive"] <= 0 {
		t.Error("turning Drive up did not drive")
	}
}

// Length past the voice hides Hold, which must wait for the release.
func TestDialRowFrozenDuringGesture(t *testing.T) {
	inst := ""
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		m := audio.MergedInstrumentParamsRO(id)
		if len(schema) == 0 || m["amp_sustain"] >= 0.8 || m["amp_sustain"] <= 0 {
			continue
		}
		g0 := newDialGameFor(t, string(id))
		_, hasLen := g0.drum.synthDialByID("length", "length")
		_, hasHold := g0.drum.synthDialByID("length", "hold")
		secs := g0.drum.synthVoiceSecs(id)
		if hasLen && hasHold && secs > 0 && secs < 3.5 && m["amp_decay"] < secs {
			inst = string(id)
			break
		}
	}
	if inst == "" {
		t.Fatal("no shipped instrument shows Length + Hold with a decay that can outgrow its voice")
	}
	g := newDialGameFor(t, inst)
	ids := func() string {
		for _, c := range g.drum.instEditorMacroCards {
			if c.id == "length" {
				var s []string
				for j := 0; j < c.nDials; j++ {
					s = append(s, c.dials[j].id)
				}
				return fmt.Sprint(s)
			}
		}
		return ""
	}
	row := ids()
	p := centre(dialRect(t, g, "length", "length"))
	frameAt(g, p.X, p.Y, true)
	for y := p.Y; y >= p.Y-200; y -= 10 {
		frameAt(g, p.X, y, true)
		if got := ids(); got != row {
			t.Fatalf("%s: the Length card's dials changed mid-drag: %s → %s", inst, row, got)
		}
	}
	frameAt(g, p.X, p.Y-200, false)
	idleFrame(g)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if ids() == row {
		t.Logf("%s: Hold still shows after the release (row %s)", inst, row)
	}
}

// Esc (or any capture dropped without a release) cancels: start values back, nothing committed, the row unfrozen,
// and another instrument shows its own cards.
func TestEscMidDragCancelsTheDialGesture(t *testing.T) {
	g := newDialGame(t)
	dv := g.drum
	inst := dialInst(g)
	before := audio.GetInstrumentParams(instrument.ID(inst))
	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)
	p := centre(dialRect(t, g, "brightness", "bright"))
	frameAt(g, p.X, p.Y, true)
	for y := p.Y; y >= p.Y-80; y -= 8 {
		frameAt(g, p.X, y, true)
	}
	if paramsEqual(audio.GetInstrumentParams(instrument.ID(inst)), before) {
		t.Fatal("premise: the drag moved nothing")
	}
	pressKeyThroughShortcuts(g, ebiten.KeyEscape)
	frameAt(g, p.X, p.Y-80, false)
	idleFrame(g)
	if dv.synthDialActive != nil {
		t.Fatal("the dial gesture is still registered after Esc dropped the drag")
	}
	if after := audio.GetInstrumentParams(instrument.ID(inst)); !paramsEqual(after, before) {
		t.Fatalf("Esc mid-drag left half-turned values live:\n before %v\n after  %v", before, after)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 0 {
		t.Fatalf("a cancelled drag recorded %d undo steps", steps)
	}
	dv.Rows[0].Instrument = roster.Kick
	dv.eqPanelZone.SetActiveChannel(channel.Of(roster.Kick))
	idleFrame(g)
	dv.Rows[0].Instrument = instrument.ID(inst)
	dv.eqPanelZone.SetActiveChannel(channel.Of(instrument.ID(inst)))
	idleFrame(g)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	if dv.macroConceptByID("brightness") == nil || dv.macroConceptByID("punch") != nil {
		t.Fatalf("back on %s the root shows the wrong cards (key inst %q)", inst, dv.instEditorMacrosKey.inst)
	}
}
