package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Phase 4 — tests for the Synth tab inside the EQ panel zone. The tab is
// reached by switching the EQ panel's active tab to TabSynth; the panel
// then delegates layout/draw/hit-areas to DrumView through the EQCallbacks.

func newSynthTabGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	return g
}

func TestSynthTab_RegisteredInPanelTabList(t *testing.T) {
	tabs := AllPanelTabs()
	found := false
	for _, t2 := range tabs {
		if t2 == TabSynth {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AllPanelTabs() must include TabSynth; got %v", tabs)
	}
	if PanelTabSlug(TabSynth) != "synth" {
		t.Errorf("PanelTabSlug(TabSynth)=%q want %q", PanelTabSlug(TabSynth), "synth")
	}
	if PanelTabLabel(TabSynth) != "Synth" {
		t.Errorf("PanelTabLabel(TabSynth)=%q want %q", PanelTabLabel(TabSynth), "Synth")
	}
}

func TestSynthTab_LayoutPopulatesSliders(t *testing.T) {
	g := newSynthTabGame(t)
	// Switch the EQ panel to TabSynth and force a layout pass.
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sliders := g.drum.SynthTabSliders()
	bindings := g.drum.SynthTabBindings()
	// Synth-tab redesign: per-recipe wired params only. drum-snare wires
	// pitch + decay + tone + drive (4); attack/color/body/brightness are
	// no-ops on this recipe and intentionally not rendered.
	want := len(audio.WiredParamsForRecipe("drum-snare"))
	if want != 4 {
		t.Fatalf("test invariant broken: drum-snare wired param count = %d, expected 4", want)
	}
	if len(sliders) != want {
		t.Errorf("got %d sliders, want %d (wired params for drum-snare)", len(sliders), want)
	}
	if len(bindings) != want {
		t.Errorf("got %d bindings, want %d", len(bindings), want)
	}
}

func TestSynthTab_NoSilentNoOpSlidersAcrossRecipes(t *testing.T) {
	// Across every shipped recipe, neither attack nor color should appear
	// in the bindings — no C renderer reads them. Locks in the Synth-tab
	// redesign's #1 UX fix at the slider-binding layer.
	g := newSynthTabGame(t)
	for _, id := range audio.RecipeOrder() {
		g.drum.Rows[0].Instrument = "test-noop-" + id
		audio.BindInstrumentToRecipe("test-noop-"+id, id)
		t.Cleanup(func() { audio.BindInstrumentToRecipe("test-noop-"+id, "") })

		g.drum.eqPanelZone.SetActiveTab(TabSynth)
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

		for _, b := range g.drum.SynthTabBindings() {
			if b.def.Name == "attack" || b.def.Name == "color" {
				t.Errorf("recipe %q: renders silent no-op slider %q", id, b.def.Name)
			}
		}
	}
}

func TestSynthTab_SlidersInitialiseFromManagerParams(t *testing.T) {
	g := newSynthTabGame(t)
	audio.SetInstrumentParam("snare", "decay", 2.0)

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	bindings := g.drum.SynthTabBindings()
	sliders := g.drum.SynthTabSliders()
	for i, b := range bindings {
		if b.def.Name != "decay" {
			continue
		}
		// decay range [0,4] → 2.0 ≈ 50%.
		if sliders[i].Value < 0.45 || sliders[i].Value > 0.55 {
			t.Errorf("decay slider Value=%v want ~0.5", sliders[i].Value)
		}
		return
	}
	t.Fatal("no decay slider found")
}

func TestSynthTab_ResetButtonClearsParams(t *testing.T) {
	g := newSynthTabGame(t)
	audio.SetInstrumentParam("snare", "decay", 0.5)
	audio.SetInstrumentParam("snare", "drive", 0.3)

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	btns := g.drum.SynthTabButtons()
	if len(btns) < 1 {
		t.Fatalf("expected at least 1 button (reset); got %d", len(btns))
	}
	resetBtn := btns[0]
	if resetBtn.Text != synthResetButtonTag {
		t.Fatalf("expected reset tag, got %q", resetBtn.Text)
	}
	// Reset via the hit adapter (the same path real input uses).
	adapter := &synthResetHitAdapter{instID: "snare"}
	cx := (resetBtn.Rect().Min.X + resetBtn.Rect().Max.X) / 2
	cy := (resetBtn.Rect().Min.Y + resetBtn.Rect().Max.Y) / 2
	adapter.OnPress(cx, cy)

	got := audio.GetInstrumentParams("snare")
	if len(got) != 0 {
		t.Errorf("Reset did not clear params: got %v", got)
	}
}

func TestSynthTab_SliderDragMutatesAudioParams(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sliders := g.drum.SynthTabSliders()
	bindings := g.drum.SynthTabBindings()
	// Find the decay slider and drag it via the hit adapter.
	var idx = -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("decay slider not in bindings")
	}
	// Synth-tab redesign uses Knob widgets (vertical drag). To exercise
	// the propagate + rescale path deterministically without relying on
	// the knob's pixels-per-sweep constant, set the slider value directly
	// — the propagate helper reads from the slider as the value store.
	sliders[idx].Value = 0.75
	g.drum.propagateSynthSliderValue(idx, "snare")

	got := audio.GetInstrumentParams("snare")
	val, ok := got["decay"]
	if !ok {
		t.Fatalf("decay not present after value set: %v", got)
	}
	// decay range [0,4] → 75% ≈ 3.0.
	if val < 2.5 || val > 3.5 {
		t.Errorf("decay=%v out of expected [2.5, 3.5] for 75%% slider", val)
	}
}

func TestSynthTab_HitAreasFlowFromEQPanelWhenActive(t *testing.T) {
	g := newSynthTabGame(t)
	// EQ tab: no synth hit areas
	g.drum.eqPanelZone.SetActiveTab(TabEQ)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	eqAreas := g.drum.eqPanelZone.HitAreas()
	for _, a := range eqAreas {
		if a.Tag == "synth-slider-0" {
			t.Error("synth hit areas leaked into TabEQ HitAreas")
		}
	}
	// Synth tab: synth hit areas appended.
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	synthAreas := g.drum.eqPanelZone.HitAreas()
	hasSlider := false
	for _, a := range synthAreas {
		if a.Tag == "synth-slider-0" {
			hasSlider = true
			break
		}
	}
	if !hasSlider {
		t.Error("TabSynth HitAreas did not include synth-slider-0")
	}
}

func TestSynthTab_SectionCardsLayoutPerRecipe(t *testing.T) {
	// Synth-tab redesign replaces inline group dividers with section
	// cards (PITCH | ENVELOPE | TONE | DRIVE | OUT). drum-snare wires
	// pitch + decay + tone + drive, so each of the 4 knob sections has
	// exactly one knob assigned (PITCH=1, ENVELOPE=1, TONE=1 [tone],
	// DRIVE=1) and no section collapses to a placeholder.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sections := g.drum.SynthTabSections()
	if len(sections) != 4 {
		t.Fatalf("got %d sections, want 4 (PITCH/ENVELOPE/TONE/DRIVE)", len(sections))
	}
	wantBySection := map[synthSectionID]int{
		synthSectionPitch:    1,
		synthSectionEnvelope: 1,
		synthSectionTone:     1, // drum-snare has tone but not body/brightness
		synthSectionDrive:    1,
	}
	for _, s := range sections {
		want, ok := wantBySection[s.id]
		if !ok {
			t.Errorf("unexpected section id %v", s.id)
			continue
		}
		if s.KnobCount() != want {
			t.Errorf("section %q (drum-snare): got %d knobs, want %d", s.Label(), s.KnobCount(), want)
		}
		if s.Rect().Empty() {
			t.Errorf("section %q has empty rect", s.Label())
		}
	}
}

func TestSynthTab_SectionCardCollapsesForUnwiredSection(t *testing.T) {
	// drum-hihat wires only decay + drive + brightness — no pitch and no
	// tone knob in the TONE section (only brightness lives there). PITCH
	// should collapse to a placeholder (zero knobs); TONE should have
	// exactly 1 (brightness).
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "hihat"
	audio.BindInstrumentToRecipe("hihat", "drum-hihat")
	t.Cleanup(func() { audio.ResetInstrumentParams("hihat") })

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	wantBySection := map[synthSectionID]int{
		synthSectionPitch:    0, // collapsed placeholder
		synthSectionEnvelope: 1, // decay
		synthSectionTone:     1, // brightness
		synthSectionDrive:    1, // drive
	}
	for _, s := range g.drum.SynthTabSections() {
		if got, want := s.KnobCount(), wantBySection[s.id]; got != want {
			t.Errorf("section %q (drum-hihat): got %d knobs, want %d", s.Label(), got, want)
		}
	}
}

func TestSynthTab_OutColumnHasSendLaunchersOnly(t *testing.T) {
	// User feedback: FX + EQ entries on the Synth tab were redundant with
	// the per-row FX overlay and the EQ tab. The OUT column now exposes
	// only the two surfaces nothing else does: Delay send + Reverb send.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	links := g.drum.SynthTabOutLinks()
	if len(links) != 2 {
		t.Fatalf("got %d OUT launchers, want 2 (Delay, Reverb only)", len(links))
	}
	gotKinds := map[synthOutLinkKind]bool{}
	for _, l := range links {
		gotKinds[l.Kind()] = true
		if l.Rect().Empty() {
			t.Errorf("OUT launcher %q has empty rect", l.DisplayLabel())
		}
	}
	for _, kind := range []synthOutLinkKind{synthOutDelay, synthOutReverb} {
		if !gotKinds[kind] {
			t.Errorf("OUT-column missing send launcher kind %v", kind)
		}
	}
}

func TestSynthTab_OutColumnDelayOpensSendPopover(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	// Find the Delay launcher and dispatch a press to its centre.
	var delay synthOutLink
	for _, l := range g.drum.SynthTabOutLinks() {
		if l.Kind() == synthOutDelay {
			delay = l
			break
		}
	}
	if delay.Rect().Empty() {
		t.Fatal("Delay launcher not laid out")
	}
	cx := (delay.Rect().Min.X + delay.Rect().Max.X) / 2
	cy := (delay.Rect().Min.Y + delay.Rect().Max.Y) / 2
	if !g.drum.handleSynthTabInput(cx, cy, true, "snare") {
		t.Fatal("Delay launcher press not consumed")
	}
	pop := g.drum.SynthTabSendPopover()
	if pop == nil {
		t.Fatal("send popover did not open after Delay launcher press")
	}
	if pop.Kind() != synthSendDelay {
		t.Errorf("popover kind = %v, want synthSendDelay", pop.Kind())
	}
	if !pop.PopoverRect().Overlaps(delay.Rect().Inset(-50)) {
		// popover should be anchored adjacent to the launcher.
		t.Errorf("popover rect %v not anchored adjacent to launcher rect %v", pop.PopoverRect(), delay.Rect())
	}
}

func TestSynthTab_OutColumnExcludesFXAndEQ(t *testing.T) {
	// Regression guard: per user request, the Synth-tab OUT column never
	// surfaces FX or EQ launchers (those have dedicated routes elsewhere).
	// If a future change reintroduces them, this test fails.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	for _, l := range g.drum.SynthTabOutLinks() {
		label := l.DisplayLabel()
		if strings.HasPrefix(label, "FX") || label == "EQ" {
			t.Errorf("OUT column contains a launcher labelled %q; FX and EQ must not appear", label)
		}
	}
}

func TestSynthTab_HeaderLayoutRecordsResolvedInstrument(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	h := g.drum.SynthTabHeader()
	if h.instLabel != "snare" {
		t.Errorf("header instLabel=%q want %q", h.instLabel, "snare")
	}
	if h.recipeID != "drum-snare" {
		t.Errorf("header recipeID=%q want %q", h.recipeID, "drum-snare")
	}
}

func TestSynthTab_SliderDragCoalescesIdenticalValues(t *testing.T) {
	// Regression: in browser builds the slider drag handler fires
	// OnDrag every frame even when the mouse stays still, producing
	// a 60Hz event flood that stalled the JS audio thread. The
	// coalescer in propagateSynthSliderValue must skip a follow-up
	// emit when the rescaled value hasn't changed.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sliders := g.drum.SynthTabSliders()
	bindings := g.drum.SynthTabBindings()
	var idx = -1
	for i, b := range bindings {
		if b.def.Name == "decay" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("decay slider not found")
	}

	// Swap the platform callback so we can count emissions.
	oldCb := audio.SwapPlatformInstrumentParamsChangedForTest(nil)
	t.Cleanup(func() { audio.SwapPlatformInstrumentParamsChangedForTest(oldCb) })

	var fired int
	audio.SwapPlatformInstrumentParamsChangedForTest(func(id string, p audio.RecipeParams) {
		fired++
	})

	// First propagation: should fire.
	sliders[idx].Value = 0.5
	g.drum.propagateSynthSliderValue(idx, "snare")
	if fired != 1 {
		t.Errorf("first propagation: fired=%d, want 1", fired)
	}

	// Subsequent identical propagations: should NOT fire (coalesced).
	for i := 0; i < 10; i++ {
		g.drum.propagateSynthSliderValue(idx, "snare")
	}
	if fired != 1 {
		t.Errorf("after 10 identical follow-ups: fired=%d, want 1 (coalescer should suppress)", fired)
	}

	// Move the slider — should fire once.
	sliders[idx].Value = 0.7
	g.drum.propagateSynthSliderValue(idx, "snare")
	if fired != 2 {
		t.Errorf("after slider move: fired=%d, want 2", fired)
	}
}

func TestSynthTab_NoGroupHeadersForUngroupedParams(t *testing.T) {
	// Register a fake recipe whose ParamDefs have empty Group; assert
	// no headers are emitted. Locks in the "Group empty → no divider"
	// behavior so a future ParamDef rename can't silently add chrome.
	const recipeID = "test-no-group-recipe"
	t.Cleanup(func() { audio.UnregisterRecipeForTest(recipeID) })
	audio.RegisterRecipe(audio.RecipeRegistration{
		ID:          recipeID,
		DisplayName: "test",
		Category:    "test",
		Params: []audio.ParamDef{
			{Name: "x", Min: 0, Max: 1, Default: 0.5},
			{Name: "y", Min: 0, Max: 1, Default: 0.5},
		},
		New: func() audio.SynthRecipe { return audio.NewRecipe("drum-snare") },
	})

	const instID = "test-no-group-inst"
	t.Cleanup(func() { audio.BindInstrumentToRecipe(instID, "") })
	audio.BindInstrumentToRecipe(instID, recipeID)

	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = instID

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	headers := g.drum.SynthTabGroupHeaders()
	if len(headers) != 0 {
		t.Errorf("got %d headers for ungrouped recipe; want 0 (labels: %v)", len(headers), headers)
	}
}

func TestSynthTab_NoActiveInstrumentRendersEmpty(t *testing.T) {
	// Game with no rows → buildSynthTab should bail out cleanly.
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.Rows = nil
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if got := len(g.drum.SynthTabSliders()); got != 0 {
		t.Errorf("got %d sliders for empty game; want 0", got)
	}
}
