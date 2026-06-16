package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthTab_KnobDragViaTreeDispatch verifies the FULL input path the
// user actually hits — mouse position + button state, dispatched through
// DrumViewTree.handleInput → HitIndex.At → HitHandler.OnPress/OnDrag/
// OnRelease. Catches z-ordering bugs, hit-area registration bugs, and
// captured-handler-loss bugs that direct-handler tests can miss.
func TestSynthTab_KnobDragViaTreeDispatch(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	expandSynthPanelForTest(t, g)

	// Locate the decay knob. Decay lives in the ENVELOPE stage — open that
	// stage so the knob gets a non-empty, hit-testable rect under the new
	// chip-strip + expand-one-detail-pane layout.
	decayIdx := synthBindingIdxByName(t, g, "decay")
	selectSectionForKnobIdx(t, g, "snare", decayIdx)
	g.Update() // flush hit index update via tree

	knobs := g.drum.SynthTabKnobs()
	rect := knobs[decayIdx].Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2
	beforeDrag := knobs[decayIdx].Value

	// Drive a press+drag+release via the tree's actual input path.
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

	// Frame 1: press at knob center.
	pressed = true
	g.Update()
	if !knobs[decayIdx].Capturing() {
		t.Fatalf("after press, knob.Capturing() = false — tree did not capture the synth-knob hit area; rect=%v knobIdx=%d", rect, decayIdx)
	}

	// Frames 2-5: drag RIGHT 15 px each frame (60 px total). Knobs are
	// horizontal-only — right increases the value. For endless knobs the
	// delta is proportional to StepMul (not absolute arc sweep), so we
	// assert direction-of-change rather than a specific magnitude
	// (Task 10: endless drag is the intentional new model).
	for step := 1; step <= 4; step++ {
		mouseX = cx + step*15
		g.Update()
	}

	// Frame N: release.
	pressed = false
	g.Update()

	afterRelease := knobs[decayIdx].Value
	if afterRelease == beforeDrag {
		t.Fatalf("knob value unchanged after tree-driven drag (before=%v after=%v) — drag is reaching the knob but not updating its value; rect=%v final-mouse=(%d,%d)", beforeDrag, afterRelease, rect, mouseX, mouseY)
	}
	// Expect a positive delta (drag right = value up).
	if afterRelease <= beforeDrag {
		t.Errorf("knob value did not increase after right drag (before=%v after=%v)", beforeDrag, afterRelease)
	}

	// Verify audio engine received the value change.
	got := audio.GetInstrumentParams("snare")
	if _, ok := got["decay"]; !ok {
		t.Errorf("audio.SetInstrumentParam was never called for decay after tree-driven drag; current params=%v", got)
	}
}

// TestSynthTab_KnobDragSurvivesMidDragRelayout is the regression test for
// the original user bug ("I drag knobs and nothing happens"). The bug was
// that buildSynthTab recreated the knob instances on every Layout, and
// Layout ran every frame because calcLayout sets bgDirty heuristically.
// Mid-drag, the captured *Knob pointed at an orphan widget; the visible
// knob in the slice was a new instance whose drag state was unset, so
// every drag frame just re-latched pressY at the current mouse position
// without ever moving the value.
//
// Fix: buildSynthTab reuses existing knob instances when the binding
// sequence is unchanged; the adapter looks up the current knob by index
// at dispatch time rather than caching a pointer. This test forces a
// re-layout in the middle of a drag and asserts the value still tracks
// the cursor.
func TestSynthTab_KnobDragSurvivesMidDragRelayout(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	expandSynthPanelForTest(t, g)

	// Open the ENVELOPE stage (owns "decay") so its knobs get non-empty,
	// hit-testable rects under the chip-strip + expand-one-detail-pane layout.
	// Only the selected stage's knobs are visible; any of them exercises the
	// same mid-drag-relayout survival path.
	idx := synthBindingIdxByName(t, g, "decay")
	selectSectionForKnobIdx(t, g, "snare", idx)

	knobs := g.drum.SynthTabKnobs()
	if knobs[idx].Rect().Empty() {
		t.Fatal("no visible synth knob")
	}
	rect := knobs[idx].Rect()
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2
	hits := g.drum.eqPanelZone.HitAreas()
	var hit *HitArea
	for i, h := range hits {
		if h.Tag == "synth-knob-"+intToStr(idx) {
			hit = &hits[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("no hit area for knob %d", idx)
	}

	beforeVal := knobs[idx].Value
	hit.Handler.OnPress(cx, cy)

	// Drag right (knobs are horizontal-only).
	hit.Handler.OnDrag(cx+30, cy)
	midVal := knobs[idx].Value
	if midVal <= beforeVal {
		t.Fatalf("first drag did not move knob (before=%v mid=%v)", beforeVal, midVal)
	}

	// Force a re-layout MID-DRAG (the bug scenario). The captured handler
	// + the live knob index must survive this.
	g.drum.eqPanelZone.Invalidate()
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	currentKnobs := g.drum.SynthTabKnobs()
	if currentKnobs[idx].Value != midVal {
		t.Errorf("re-layout clobbered live drag value: was %v after Layout=%v", midVal, currentKnobs[idx].Value)
	}
	if !currentKnobs[idx].Capturing() {
		t.Errorf("re-layout cleared the knob's dragging state — next drag will re-latch and ignore the user's motion")
	}

	// Continue the drag — value should keep tracking from where we left off.
	hit.Handler.OnDrag(cx+60, cy)
	finalVal := currentKnobs[idx].Value
	if finalVal <= midVal {
		t.Errorf("drag did not progress past re-layout (mid=%v final=%v)", midVal, finalVal)
	}

	hit.Handler.OnRelease(cx+60, cy)
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// TestSynthTab_KnobDragViaTreeDispatch_Mobile runs the same drag scenario
// in a mobile-profile game, because the mobile layout stacks sections
// vertically and uses different knob sizes. If the mobile layout breaks
// the hit-area registration, this test catches it.
func TestSynthTab_KnobDragViaTreeDispatch_Mobile(t *testing.T) {
	assertDefaultParityState(t)
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(414, 896)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()

	knobs := g.drum.SynthTabKnobs()
	bindings := g.drum.SynthTabBindings()
	wantKnobs := len(audio.WiredParamsForRecipe("drum-snare"))
	if len(knobs) != wantKnobs {
		t.Fatalf("mobile: expected %d knobs (wired), got %d", wantKnobs, len(knobs))
	}
	var decayIdx = -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob in mobile layout")
	}
	// Decay lives in ENVELOPE; open that stage so the knob gets a non-empty,
	// hit-testable rect under the chip-strip + expand-one-detail-pane layout.
	selectSectionForKnobIdx(t, g, "snare", decayIdx)
	g.Update()
	k := g.drum.SynthTabKnobs()[decayIdx]
	rect := k.Rect()
	if rect.Empty() {
		t.Fatalf("mobile decay knob has empty rect; section layout = %v", g.drum.SynthTabSections())
	}
	beforeDrag := k.Value

	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	mouseX, mouseY := cx, cy
	pressed := false
	restoreInput := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 414, 896 },
	)
	defer restoreInput()

	pressed = true
	g.Update()
	if !k.Capturing() {
		t.Fatalf("mobile: knob not capturing after press at center (%d,%d); rect=%v", cx, cy, rect)
	}

	for step := 1; step <= 4; step++ {
		mouseX = cx + step*15
		g.Update()
	}
	pressed = false
	g.Update()

	if k.Value == beforeDrag {
		t.Fatalf("mobile: knob value unchanged after drag; rect=%v", rect)
	}
}
