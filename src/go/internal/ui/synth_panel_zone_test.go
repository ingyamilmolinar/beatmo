package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The EQ panel delegates the Synth tab's layout, draw and hit areas to DrumView through EQCallbacks.

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
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
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
	// One control per ParamDef of the instrument's document.
	want := len(audio.InstrumentParamDefs(roster.Snare))
	if want == 0 {
		t.Fatal("test invariant broken: snare has no ParamDefs")
	}
	if len(sliders) != want {
		t.Errorf("got %d sliders, want %d (ParamDefs of snare)", len(sliders), want)
	}
	if len(bindings) != want {
		t.Errorf("got %d bindings, want %d", len(bindings), want)
	}
}

func TestSynthTab_NoSilentNoOpSlidersAcrossRecipes(t *testing.T) {
	// No C renderer reads attack or color, so neither may be bound on any instrument.
	g := newSynthTabGame(t)
	for _, id := range roster.All {
		g.drum.Rows[0].Instrument = instrument.ID("test-noop-" + string(id))
		_ = audio.InstallInstrumentDocForTest("test-noop-"+id, id)
		t.Cleanup(func() { audio.RemoveInstrumentDocForTest("test-noop-" + id) })

		g.drum.eqPanelZone.SetActiveTab(TabSynth)
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

		for _, b := range g.drum.SynthTabBindings() {
			if b.def.Name == "attack" || b.def.Name == "color" {
				t.Errorf("instrument %q: renders silent no-op slider %q", id, b.def.Name)
			}
		}
	}
}

func TestSynthTab_SlidersInitialiseFromManagerParams(t *testing.T) {
	g := newSynthTabGame(t)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.5)

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	bindings := g.drum.SynthTabBindings()
	sliders := g.drum.SynthTabSliders()
	for i, b := range bindings {
		if b.def.Name != "post_decay" {
			continue
		}
		// Length range [0,1] → 0.5 ≈ 50%.
		if sliders[i].Value < 0.45 || sliders[i].Value > 0.55 {
			t.Errorf("decay slider Value=%v want ~0.5", sliders[i].Value)
		}
		return
	}
	t.Fatal("no decay slider found")
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
		if b.def.Name == "post_decay" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("decay slider not in bindings")
	}
	// Set directly: the propagate helper reads the knob as its value store, independent of pixels per sweep.
	sliders[idx].Value = 0.75
	g.drum.propagateSynthSliderValue(idx, roster.Snare)

	got := audio.GetInstrumentParams(roster.Snare)
	val, ok := got["post_decay"]
	if !ok {
		t.Fatalf("decay not present after value set: %v", got)
	}
	// Length range [0,1] → 75% ≈ 0.75.
	if val < 0.7 || val > 0.8 {
		t.Errorf("decay=%v out of expected [0.7, 0.8] for 75%% slider", val)
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
	// Only the selected stage's knobs publish hit areas, so open knob 0's stage at the expanded height first.
	expandSynthPanelForTest(t, g)
	selectSectionForKnobIdx(t, g, roster.Snare, 0)
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
	// Every synth instrument shows the standard sequence VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE ·
	// FILTER · FILTER ENV · FORMANT · RESONATOR · POST, pruned of empty stages (the snare has no FM); stages carry
	// enable pills, VOICE/ENSEMBLE/RESONATOR none.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sections := g.drum.SynthTabSections()
	if len(sections) != 13 {
		t.Fatalf("got %d sections, want 13 (VOICE/OSC/ENSEMBLE/PITCH/LFO/BURST/ENVELOPE/FILTER/FILTER ENV/FORMANT/RESONATOR/POST/FINISH)", len(sections))
	}
	wantBySection := map[synthSectionID]int{
		synthSectionVoice:     12,
		synthSectionOsc:       3,
		synthSectionEnsemble:  10,
		synthSectionPitch:     2,
		synthSectionLFO:       4,
		synthSectionBurst:     9,
		synthSectionEnvelope:  5,
		synthSectionFilter:    3,
		synthSectionFilterEnv: 3,
		synthSectionFormant:   9,
		synthSectionResonator: 3,
		synthSectionPost:      3,
		synthSectionFinish:    8, // resample/fade/fade speed/low boost/air/darken/crunch/limit
	}
	wantEnable := map[synthSectionID]string{
		synthSectionVoice:     "",
		synthSectionOsc:       "osc_enabled",
		synthSectionEnsemble:  "",
		synthSectionPitch:     "pitchenv_enabled",
		synthSectionLFO:       "lfo_enabled",
		synthSectionBurst:     "burst_enabled",
		synthSectionEnvelope:  "env_enabled",
		synthSectionFilter:    "filter_enabled",
		synthSectionFilterEnv: "filtenv_enabled",
		synthSectionFormant:   "formant_enabled",
		synthSectionResonator: "",
		synthSectionPost:      "drive_enabled",
		synthSectionFinish:    "",
	}
	for _, s := range sections {
		want, ok := wantBySection[s.id]
		if !ok {
			t.Errorf("unexpected section id %v", s.id)
			continue
		}
		if s.KnobCount() != want {
			t.Errorf("section %q (synth-modular-snare): got %d knobs, want %d", s.Label(), s.KnobCount(), want)
		}
		if s.EnableParam() != wantEnable[s.id] {
			t.Errorf("section %q (synth-modular-snare): enableParam=%q want %q", s.Label(), s.EnableParam(), wantEnable[s.id])
		}
	}

	// Sections are the stage model in pipeline order: ON stages as pills, OFF ones under "+".
	on, off := g.drum.synp().activePipelineStages(g.drum.synthTabActiveInstrument())
	if got := len(on) + len(off); got != len(sections) {
		t.Fatalf("pipeline model covers %d stages, want one per section (%d)", got, len(sections))
	}
	prev := -1
	for _, i := range on {
		if i <= prev {
			t.Errorf("strip pills out of pipeline order at section %d", i)
		}
		prev = i
	}
	for _, s := range sections {
		if !s.Rect().Empty() {
			t.Errorf("section %q has a layout rect %v — sections are a model, cards come from the drilled view", s.Label(), s.Rect())
		}
	}
}

func TestSynthTab_EmptySectionsArePruned(t *testing.T) {
	// Empty sections are pruned, never shown as placeholders: the bare modular voice has no FM operators to show.
	g := setupModularSynthGame(t)
	audio.SetInstrumentParam(roster.Modular, "osc_type", 0) // back to the sine oscillator
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	for _, s := range g.drum.SynthTabSections() {
		if s.id == synthSectionFM {
			t.Errorf("FM section shown with %d knobs on a sine oscillator — it must be pruned", s.KnobCount())
		}
		if s.KnobCount() == 0 && s.EnableParam() == "" {
			t.Errorf("section %q is empty (no knob, no pill) yet not pruned", s.Label())
		}
	}
}

// A WAV instrument gets the single no-synth banner: no sections, no knobs.
func TestSynthTab_NoSynthInstrumentShowsBanner(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.Rows[0].Name = "Kick"
	g.drum.Rows[0].Instrument = "kick-wav"
	// No instrument document → this instrument plays a sample, not the synth.
	audio.RemoveInstrumentDocForTest("kick-wav")
	if audio.HasInstrumentDoc("kick-wav") {
		t.Fatalf("test invariant broken: kick-wav must have no instrument document")
	}

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if !g.drum.SynthTabNoSynth() {
		t.Fatal("expected SynthTabNoSynth()==true for a sample (no-recipe) instrument")
	}
	if n := len(g.drum.SynthTabSections()); n != 0 {
		t.Errorf("no-synth instrument must render no section cards; got %d sections", n)
	}
	if n := len(g.drum.SynthTabBindings()); n != 0 {
		t.Errorf("no-synth instrument must wire no knobs; got %d bindings", n)
	}
	if g.drum.SynthTabBannerRect().Empty() {
		t.Error("no-synth banner rect must be non-empty so the banner has somewhere to draw")
	}
}

// The control: a synth instrument keeps its cards.
func TestSynthTab_RecipeInstrumentDoesNotShowBanner(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if g.drum.SynthTabNoSynth() {
		t.Error("synth instrument (synth-modular-snare) must not enter the no-synth banner state")
	}
	if !g.drum.SynthTabBannerRect().Empty() {
		t.Error("synth instrument must not reserve a no-synth banner rect")
	}
	if len(g.drum.SynthTabSections()) == 0 {
		t.Error("synth instrument must still render section cards")
	}
}

func TestSynthTab_HeaderLayoutRecordsResolvedInstrument(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	h := g.drum.SynthTabHeader()
	if h.instLabel != roster.Snare {
		t.Errorf("header instLabel=%q want %q", h.instLabel, "snare")
	}
}

func TestSynthTab_SliderDragCoalescesIdenticalValues(t *testing.T) {
	// The browser fires OnDrag every frame of a still drag; an unchanged value must emit nothing, or the event flood
	// stalls the JS audio thread.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sliders := g.drum.SynthTabSliders()
	bindings := g.drum.SynthTabBindings()
	var idx = -1
	for i, b := range bindings {
		if b.def.Name == "post_decay" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("decay slider not found")
	}

	// Swap the platform callback so we can count emissions.
	oldCb := audio.SwapPlatformInstrumentDocChangedForTest(nil)
	t.Cleanup(func() { audio.SwapPlatformInstrumentDocChangedForTest(oldCb) })

	var fired int
	audio.SwapPlatformInstrumentDocChangedForTest(func(instrument.ID, instrument.Doc) {
		fired++
	})

	// First propagation: should fire.
	sliders[idx].Value = 0.5
	g.drum.propagateSynthSliderValue(idx, roster.Snare)
	if fired != 1 {
		t.Errorf("first propagation: fired=%d, want 1", fired)
	}

	// Subsequent identical propagations: should NOT fire (coalesced).
	for i := 0; i < 10; i++ {
		g.drum.propagateSynthSliderValue(idx, roster.Snare)
	}
	if fired != 1 {
		t.Errorf("after 10 identical follow-ups: fired=%d, want 1 (coalescer should suppress)", fired)
	}

	// Move the slider — should fire once.
	sliders[idx].Value = 0.7
	g.drum.propagateSynthSliderValue(idx, roster.Snare)
	if fired != 2 {
		t.Errorf("after slider move: fired=%d, want 2", fired)
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

// So a fixture routes exactly as the shipped schema does.
func stageDefForTest(t *testing.T, name string) audio.ParamDef {
	t.Helper()
	def, ok := audio.StageParamDefByName(name)
	if !ok {
		t.Fatalf("no stage ParamDef named %q", name)
	}
	return def
}
