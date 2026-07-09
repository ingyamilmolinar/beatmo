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
	// pitch + decay + tone + drive (4) plus the snare family knobs;
	// attack/color/body/brightness are no-ops on this recipe and
	// intentionally not rendered.
	want := len(audio.WiredParamsForRecipe("drum-snare"))
	if want < 4 {
		t.Fatalf("test invariant broken: drum-snare wired param count = %d, expected >= 4", want)
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
	var resetBtn *Button
	for _, b := range btns {
		if b != nil && b.Text == synthResetButtonTag {
			resetBtn = b
			break
		}
	}
	if resetBtn == nil {
		t.Fatalf("reset button not present among %d header buttons", len(btns))
	}
	// Reset via the shared buttonHitAdapter (the same path real input uses
	// now that footer buttons dispatch through their own OnClick).
	adapter := &buttonHitAdapter{btn: resetBtn}
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
	// Synth tab: synth hit areas appended. Under the chip-strip +
	// expand-one-detail-pane layout only the SELECTED stage's knobs publish
	// hit areas, so open the stage that owns knob index 0 (and force the panel
	// to its expanded synth height) before asserting synth-slider-0 flows
	// through the EQ panel zone.
	expandSynthPanelForTest(t, g)
	selectSectionForKnobIdx(t, g, "snare", 0)
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
	// sequence. For drum-snare:
	//   VOICE     = the 11 snare family/voice knobs (generator + fundamental + the
	//               tone/noise/tail decays + mixes + attack — the knobs that ARE
	//               the snare sound),
	//   OSC       = the 3 standardized oscillator knobs (osc_type/detune/octave),
	//   ENSEMBLE  = the 5 unison/ensemble knobs (Voices/Detune/Mix + 2 drift)
	//               which stack the oscillator, PLUS the 4 Phase-15 humanization
	//               knobs (Scatter/Vibrato Rate/Vibrato Depth/Humanize) PLUS the
	//               Phase-16 Roughness (ens_jitter) knob → 10,
	//   FM        = the 13 standardized FM operator knobs,
	//   PITCH     = the 2 pitch-env knobs (pitchenv_amt/decay),
	//   LFO       = the 4 LFO knobs (lfo_rate/depth/target/delay),
	//   BURST     = sharpness + 4×(off, amp) = 9,
	//   ENV       = decay (generic) + the 5 amp ADSR knobs = 6,
	//   FILTER    = the 3 standardized static-filter knobs,
	//   FILTER ENV= the 3 filter-envelope knobs (filtenv_amt/decay/attack),
	//   FORMANT   = the 9 formant knobs (Vowel/Voice Type/Mix/Head Size/Breath/
	//               Shine/Morph Speed/Morph To + the Phase-16 Dry Blend
	//               (formant_dry) knob — formant_enabled is the pill),
	//   RESONATOR = the 3 body-resonator knobs (Body Model/Body Mix/Bow Dynamics),
	//   POST      = pitch + tone + drive (generic post) + gain = 4.
	// Each stage section also carries its enable pill; VOICE/ENSEMBLE/RESONATOR
	// carry none.
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	sections := g.drum.SynthTabSections()
	if len(sections) != 13 {
		t.Fatalf("got %d sections, want 13 (VOICE/OSC/ENSEMBLE/FM/PITCH/LFO/BURST/ENVELOPE/FILTER/FILTER ENV/FORMANT/RESONATOR/POST)", len(sections))
	}
	wantBySection := map[synthSectionID]int{
		synthSectionVoice:     11,
		synthSectionOsc:       3,
		synthSectionEnsemble:  10,
		synthSectionFM:        13,
		synthSectionPitch:     2,
		synthSectionLFO:       4,
		synthSectionBurst:     9,
		synthSectionEnvelope:  6,
		synthSectionFilter:    3,
		synthSectionFilterEnv: 3,
		synthSectionFormant:   9,
		synthSectionResonator: 3,
		synthSectionPost:      4,
	}
	wantEnable := map[synthSectionID]string{
		synthSectionVoice:     "",
		synthSectionOsc:       "osc_enabled",
		synthSectionEnsemble:  "",
		synthSectionFM:        "fm_enabled",
		synthSectionPitch:     "pitchenv_enabled",
		synthSectionLFO:       "lfo_enabled",
		synthSectionBurst:     "burst_enabled",
		synthSectionEnvelope:  "env_enabled",
		synthSectionFilter:    "filter_enabled",
		synthSectionFilterEnv: "filtenv_enabled",
		synthSectionFormant:   "formant_enabled",
		synthSectionResonator: "",
		synthSectionPost:      "post_enabled",
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
		if s.EnableParam() != wantEnable[s.id] {
			t.Errorf("section %q (drum-snare): enableParam=%q want %q", s.Label(), s.EnableParam(), wantEnable[s.id])
		}
	}

	// Synth-tab redesign: sections are no longer laid out side-by-side. Every
	// section instead appears as a chip in the pipeline strip; exactly one
	// section (the selected stage) is expanded into the detail pane and is the
	// only one with a non-empty rect. The equivalent per-recipe layout claim
	// is therefore: one chip per section (in order), and exactly one non-empty
	// section rect equal to the detail pane.
	chips := g.drum.instEditorChips
	if len(chips) != len(sections) {
		t.Fatalf("got %d chips, want one per section (%d)", len(chips), len(sections))
	}
	for i, c := range chips {
		if c.id != sections[i].id {
			t.Errorf("chip[%d] id=%v, want section order %v", i, c.id, sections[i].id)
		}
		if c.rect.Empty() {
			t.Errorf("section %q chip has empty rect — every stage must stay visible in the strip", sectionLabel(c.id))
		}
	}
	nonEmpty := 0
	for _, s := range sections {
		if s.Rect().Empty() {
			continue
		}
		nonEmpty++
		if s.Rect() != g.drum.instEditorDetailR {
			t.Errorf("non-empty section %q rect %v != detail pane %v", s.Label(), s.Rect(), g.drum.instEditorDetailR)
		}
	}
	if nonEmpty != 1 {
		t.Errorf("got %d sections with non-empty rects, want exactly 1 (the selected/detail stage)", nonEmpty)
	}
}

func TestSynthTab_EmptySectionsArePruned(t *testing.T) {
	// Phase 8B unification: empty standardized sections are PRUNED (not shown as
	// collapsed "not used" placeholders). A minimal plugin recipe that wires
	// only decay + brightness + drive produces just the ENVELOPE (decay) and
	// POST (brightness + drive) sections — no VOICE/OSC/FM/FILTER cards at all,
	// since the recipe contributes no content to them.
	registerCollapsedSectionTestRecipe(t)
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "hihat"
	audio.BindInstrumentToRecipe("hihat", collapsedTestRecipeID)
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe("hihat", "drum-hihat")
		audio.ResetInstrumentParams("hihat")
	})

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	wantBySection := map[synthSectionID]int{
		synthSectionEnvelope: 1, // decay
		synthSectionPost:     2, // brightness + drive
	}
	got := map[synthSectionID]int{}
	for _, s := range g.drum.SynthTabSections() {
		got[s.id] = s.KnobCount()
		if _, ok := wantBySection[s.id]; !ok {
			t.Errorf("unexpected (non-pruned) section %q with %d knobs", s.Label(), s.KnobCount())
		}
	}
	for id, want := range wantBySection {
		if got[id] != want {
			t.Errorf("section %q: got %d knobs, want %d", sectionLabel(id), got[id], want)
		}
	}
}

// collapsedTestRecipeID is a runtime-registered recipe whose schema leaves
// the PITCH section empty, standing in for the user/plugin recipes that can
// still produce collapsed section cards.
const collapsedTestRecipeID = "test-collapsed-section"

type collapsedTestProvider struct{}

func (collapsedTestProvider) Render(buf []float32, sampleRate, samples, variant int, p audio.RecipeParams) {
	for i := 0; i < samples && i < len(buf); i++ {
		buf[i] = 0.1
	}
}

func registerCollapsedSectionTestRecipe(t *testing.T) {
	t.Helper()
	err := audio.RegisterPluginRecipe(audio.PluginRecipeOptions{
		ID:          collapsedTestRecipeID,
		DisplayName: "Collapsed Section Test",
		Category:    "drum",
		ParamDefs: []audio.ParamDef{
			{Name: "decay", Label: "Decay", Min: 0, Max: 4, Default: 1},
			{Name: "brightness", Label: "Brightness", Min: 0, Max: 1, Default: 0},
			{Name: "drive", Label: "Drive", Min: 0, Max: 1, Default: 0},
		},
		Provider: collapsedTestProvider{},
	})
	if err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("RegisterPluginRecipe: %v", err)
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
	// No recipe binding → this instrument plays a sample, not the synth.
	audio.BindInstrumentToRecipe("kick-wav", "")
	if audio.RecipeForInstrument("kick-wav") != "" {
		t.Fatalf("test invariant broken: kick-wav must have no recipe binding")
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
// a real synth instrument (snare → drum-snare) must NOT enter the no-synth
// banner state and must still render its section cards.
func TestSynthTab_RecipeInstrumentDoesNotShowBanner(t *testing.T) {
	g := newSynthTabGame(t)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if g.drum.SynthTabNoSynth() {
		t.Error("synth instrument (drum-snare) must not enter the no-synth banner state")
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
