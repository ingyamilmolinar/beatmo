package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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
	// Across every shipped instrument, neither attack nor color should appear
	// in the bindings — no C renderer reads them. Locks in the Synth-tab
	// redesign's #1 UX fix at the slider-binding layer.
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
	// Synth-tab redesign uses Knob widgets (vertical drag). To exercise
	// the propagate + rescale path deterministically without relying on
	// the knob's pixels-per-sweep constant, set the slider value directly
	// — the propagate helper reads from the slider as the value store.
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
	// Synth tab: synth hit areas appended. Under the chip-strip +
	// expand-one-detail-pane layout only the SELECTED stage's knobs publish
	// hit areas, so open the stage that owns knob index 0 (and force the panel
	// to its expanded synth height) before asserting synth-slider-0 flows
	// through the EQ panel zone.
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
	// Phase 8B unified Synth tab (+ Phase-8C modulator stages + the Phase-8E
	// filter-envelope / unison additions + the Phase-15 voice/choir stages,
	// Task 8): every synth instrument shows the standardized
	// VOICE · OSC · ENSEMBLE · FM · PITCH · LFO · BURST · ENVELOPE · FILTER ·
	// FILTER ENV · FORMANT · RESONATOR · POST
	// sequence. For synth-modular-snare:
	//   VOICE     = the 12 snare family/voice knobs (generator + fundamental + the
	//               tone/noise/tail decays + mixes + attack + the base Pitch
	//               voice_freq_hz the seed sets — the knobs that ARE the snare
	//               sound),
	//   OSC       = the 3 standardized oscillator knobs (osc_type/detune/octave),
	//   ENSEMBLE  = the 5 unison/ensemble knobs (Voices/Detune/Mix + 2 drift)
	//               which stack the oscillator, PLUS the 4 Phase-15 humanization
	//               knobs (Scatter/Vibrato Rate/Vibrato Depth/Humanize) PLUS the
	//               Phase-16 Roughness (ens_jitter) knob → 10,
	//   (no FM: the operators belong to the FM oscillator module, and the
	//    snare's oscillator is the sine — spec 2026-10-04 §6),
	//   PITCH     = the 2 pitch-env knobs (pitchenv_amt/decay),
	//   LFO       = the 4 LFO knobs (lfo_rate/depth/target/delay),
	//   BURST     = sharpness + 4×(off, amp) = 9,
	//   ENV       = the 5 amp ADSR knobs (env_enabled is the pill),
	//   FILTER    = the 3 standardized static-filter knobs,
	//   FILTER ENV= the 3 filter-envelope knobs (filtenv_amt/decay/attack),
	//   FORMANT   = the 9 formant knobs (Vowel/Voice Type/Mix/Head Size/Breath/
	//               Shine/Morph Speed/Morph To + the Phase-16 Dry Blend
	//               (formant_dry) knob — formant_enabled is the pill),
	//   RESONATOR = the 3 body-resonator knobs (Body Model/Body Mix/Bow Dynamics),
	//   POST      = drive + pitch + gain = 3 (drive_enabled is the pill).
	// Each stage section also carries its enable pill; VOICE/ENSEMBLE/RESONATOR
	// carry none.
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

	// Category layer: sections are the stage MODEL (never laid out as cards).
	// Every section is part of the pipeline strip model — ON stages as pills,
	// OFF stages under the "+" pill — in pipeline order.
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
	// Empty standardized sections are PRUNED (never shown as "not used"
	// placeholders). The bare modular voice plays the sine oscillator and no
	// generator layer, so the FM section (the FM oscillator's operators) has no
	// knob to show and must be absent; every section that IS shown carries at
	// least one knob or its enable pill.
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

// TestSynthTab_NoSynthInstrumentShowsBanner verifies that when the active
// row's instrument has no synth recipe (it plays a loaded WAV sample), the
// synth tab abandons its grid of empty section
// cards and instead enters the single-banner no-synth state: no sections,
// no wired knobs, and a non-empty banner rect. Removing the section cards
// also removes the distracting trigger-pulse borders that used to blink
// around cards that the sample doesn't use.
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

// TestSynthTab_RecipeInstrumentDoesNotShowBanner is the negative control:
// a real synth instrument (snare → synth-modular-snare) must NOT enter the no-synth
// banner state and must still render its section cards.
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

// stageDefForTest returns the real modular ParamDef for a stage param, so a
// test fixture routes exactly as the shipped schema does.
func stageDefForTest(t *testing.T, name string) audio.ParamDef {
	t.Helper()
	def, ok := audio.StageParamDefByName(name)
	if !ok {
		t.Fatalf("no stage ParamDef named %q", name)
	}
	return def
}
