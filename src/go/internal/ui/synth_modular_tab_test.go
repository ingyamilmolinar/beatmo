package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// setupModularSynthGame points row 0 at the unified modular instrument and
// opens the Synth tab.
func setupModularSynthGame(t *testing.T) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = "modular"
	audio.BindInstrumentToRecipe("modular", "synth-modular")
	t.Cleanup(func() {
		audio.BindInstrumentToRecipe("modular", "synth-modular")
		audio.ResetInstrumentParams("modular")
	})
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	return g
}

func TestSynthTab_ModularExposesEveryStage(t *testing.T) {
	g := setupModularSynthGame(t)
	bindings := g.drum.SynthTabBindings()
	want := len(audio.ModularSynthParamDefs())
	if len(bindings) != want {
		t.Fatalf("modular synth tab: got %d controls, want %d (every pipeline param)", len(bindings), want)
	}
	names := map[string]bool{}
	for _, b := range bindings {
		names[b.def.Name] = true
	}
	for _, n := range []string{"osc_type", "fm_op1_ratio", "amp_attack", "amp_release", "filter_cutoff", "filter_type", "drive", "gain"} {
		if !names[n] {
			t.Errorf("modular synth tab missing control for %q", n)
		}
	}
}

func TestSynthTab_ModularSectionsCoverPipeline(t *testing.T) {
	g := setupModularSynthGame(t)
	sections := g.drum.SynthTabSections()
	ids := map[synthSectionID]bool{}
	for _, s := range sections {
		ids[s.id] = true
	}
	for _, want := range []synthSectionID{synthSectionOsc, synthSectionFM, synthSectionEnvelope, synthSectionFilter} {
		if !ids[want] {
			t.Errorf("modular synth tab missing section %v (label %q)", want, sectionLabel(want))
		}
	}
	// Phase 8B unification: a migrated drum recipe (drum-snare) now ALSO shows
	// the full standardized pipeline — VOICE for its family knobs plus the
	// enable-able OSC/FM/ENVELOPE/FILTER/POST stages. This is the deliverable:
	// every synth instrument shares the same section sequence and any stage can
	// be enabled on any instrument.
	g2 := newSynthTabGame(t) // drum-snare
	g2.drum.eqPanelZone.SetActiveTab(TabSynth)
	g2.drum.eqPanelZone.Layout(g2.drum.eqPanelZone.PanelRect())
	drumIDs := map[synthSectionID]bool{}
	for _, s := range g2.drum.SynthTabSections() {
		drumIDs[s.id] = true
	}
	for _, want := range []synthSectionID{
		synthSectionVoice, synthSectionOsc, synthSectionFM,
		synthSectionEnvelope, synthSectionFilter, synthSectionPost,
	} {
		if !drumIDs[want] {
			t.Errorf("drum-snare synth tab missing standardized section %q", sectionLabel(want))
		}
	}
}

// TestSynthTab_WaveKnobCaptionShowsLabel locks the family generator-type
// selector rendering: every <family>_wave ParamDef is a discrete enum whose
// knob caption shows the waveform name, not a raw number.
func TestSynthTab_WaveKnobCaptionShowsLabel(t *testing.T) {
	for _, recipe := range []string{"drum-kick", "drum-hihat", "fm-bass"} {
		var waveDef *audio.ParamDef
		for _, d := range audio.WiredParamsForRecipe(recipe) {
			if strings.HasSuffix(d.Name, "_wave") {
				dd := d
				waveDef = &dd
				break
			}
		}
		if waveDef == nil {
			t.Fatalf("%s: no *_wave ParamDef", recipe)
		}
		for idx, label := range waveDef.Enum {
			got := synthKnobCaption(*waveDef, float64(idx))
			if !strings.Contains(got, label) {
				t.Errorf("%s %s caption @%d = %q, want it to contain %q", recipe, waveDef.Name, idx, got, label)
			}
		}
	}
}

func TestSynthTab_EnumParamCaptionShowsLabel(t *testing.T) {
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 4, Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM"}}
	for idx, label := range def.Enum {
		got := synthKnobCaption(def, float64(idx))
		if !strings.Contains(got, label) {
			t.Errorf("osc_type caption @%d = %q, want it to contain %q", idx, got, label)
		}
		gotV := synthKnobCaptionValueOnly(def, float64(idx))
		if !strings.Contains(gotV, label) {
			t.Errorf("osc_type value-only caption @%d = %q, want %q", idx, gotV, label)
		}
	}
	// Out-of-range value clamps to a valid label (no panic / index error).
	if got := synthKnobCaption(def, 9); !strings.Contains(got, "FM") {
		t.Errorf("osc_type caption @9 = %q, want clamped to FM", got)
	}
}

// TestSynthTab_ModularEnumKnobWritesSnappedIndex — driving the osc_type
// control (an Enum param rendered as a snapping knob) must write the nearest
// integer selector index through audio.SetInstrumentParam, and that write must
// reach the platform bridge (the WASM seam the browser renders through).
func TestSynthTab_ModularEnumKnobWritesSnappedIndex(t *testing.T) {
	g := setupModularSynthGame(t)
	sliders := g.drum.SynthTabSliders()
	bindings := g.drum.SynthTabBindings()

	idx := -1
	for i, b := range bindings {
		if b.def.Name == "osc_type" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("osc_type control not in modular bindings")
	}
	if len(bindings[idx].def.Enum) == 0 {
		t.Fatal("osc_type binding has no Enum labels")
	}

	var gotPushed = -1.0
	oldCb := audio.SwapPlatformInstrumentParamsChangedForTest(func(id string, p audio.RecipeParams) {
		if id == "modular" {
			if v, ok := p["osc_type"]; ok {
				gotPushed = v
			}
		}
	})
	t.Cleanup(func() { audio.SwapPlatformInstrumentParamsChangedForTest(oldCb) })

	// osc_type range [0,6] (Sine..Noise Pink); 1/6 of the sweep == index 1
	// (Saw). The snap must land it on exactly 1 — no fractional drift.
	sliders[idx].Value = 1.0 / 6.0
	g.drum.propagateSynthSliderValue(idx, "modular")

	got := audio.GetInstrumentParams("modular")["osc_type"]
	if got != 1 {
		t.Errorf("osc_type stored = %v, want exactly 1 (snapped Saw index)", got)
	}
	if gotPushed != 1 {
		t.Errorf("platform bridge received osc_type = %v, want 1 (the browser renders through this seam)", gotPushed)
	}
}

// TestSynthTab_OscPreviewTracksOscType — the OSC preview must be data-driven:
// selecting Saw yields a different previewed waveform than Sine. We assert the
// geometry-determining sample values differ, not pixels.
func TestSynthTab_OscPreviewTracksOscType(t *testing.T) {
	g := setupModularSynthGame(t)
	_ = g

	audio.SetInstrumentParam("modular", "osc_type", 0) // sine
	otSine, _, _ := synthOscParamsFor("modular")
	audio.SetInstrumentParam("modular", "osc_type", 1) // saw
	otSaw, ratio, depth := synthOscParamsFor("modular")
	if otSine != 0 || otSaw != 1 {
		t.Fatalf("synthOscParamsFor did not track osc_type: sine=%d saw=%d", otSine, otSaw)
	}
	// At a quarter-cycle, sine and saw have clearly different amplitudes.
	const phase = 0.25
	sineV := synthOscSample(0, phase, ratio, depth)
	sawV := synthOscSample(1, phase, ratio, depth)
	if sineV == sawV {
		t.Errorf("sine and saw previews identical at phase %.2f (%.3f); preview not shape-driven", phase, sineV)
	}
}

// TestSynthADSRParamsFor — the ADSR preview reads the modular voice's real
// amp_* params (so it tracks every stage), and reports ok=false for non-modular
// recipes (which keep the decay-knob approximation).
func TestSynthADSRParamsFor(t *testing.T) {
	g := setupModularSynthGame(t)
	_ = g

	audio.SetInstrumentParam("modular", "amp_attack", 0.4)
	audio.SetInstrumentParam("modular", "amp_sustain", 0.3)
	audio.SetInstrumentParam("modular", "amp_release", 1.1)
	a, _, s, r, ok := synthADSRParamsFor("modular")
	if !ok {
		t.Fatal("synthADSRParamsFor(modular) ok=false, want true")
	}
	if a != 0.4 || s != 0.3 || r != 1.1 {
		t.Errorf("ADSR params not tracked: attack=%v sustain=%v release=%v", a, s, r)
	}
	// Phase 8B unification: a migrated drum recipe now ALSO exposes the
	// standardized amp ADSR stage (osc/env/filter are user-enable-able on every
	// synth), so the preview pane reads the drum's ADSR too. The stage ships OFF
	// by default, but its params are visible/editable.
	if _, _, _, _, ok := synthADSRParamsFor("snare"); !ok {
		t.Errorf("synthADSRParamsFor(snare) ok=false; the unified ENVELOPE stage must expose ADSR for every synth")
	}
}

// TestSynthFilterPreviewTracksCutoff — the filter preview reads the modular
// voice's own filter params (not the channel EQ), and raising the cutoff lifts
// the response at a fixed high-frequency probe. Geometry, not pixels.
func TestSynthFilterPreviewTracksCutoff(t *testing.T) {
	g := setupModularSynthGame(t)
	_ = g

	ft, _, _, ok := synthFilterParamsFor("modular")
	if !ok {
		t.Fatal("synthFilterParamsFor(modular) ok=false, want true")
	}
	// Phase 8B unification: a migrated drum recipe now exposes the standardized
	// FILTER stage too (enable-able per instrument), so its filter preview reads
	// the synth filter params (the stage ships OFF by default).
	if _, _, _, ok := synthFilterParamsFor("snare"); !ok {
		t.Errorf("synthFilterParamsFor(snare) ok=false; the unified FILTER stage must expose filter params for every synth")
	}

	const probe = 5000.0
	audio.SetInstrumentParam("modular", "filter_cutoff", 800)
	_, c1, q1, _ := synthFilterParamsFor("modular")
	lowGain := filterGainAt(audio.ModularFilterResponse(ft, c1, q1, 48000, 256, 20, 20000), probe)
	audio.SetInstrumentParam("modular", "filter_cutoff", 12000)
	_, c2, q2, _ := synthFilterParamsFor("modular")
	highGain := filterGainAt(audio.ModularFilterResponse(ft, c2, q2, 48000, 256, 20, 20000), probe)
	if !(highGain > lowGain) {
		t.Errorf("filter preview did not track cutoff at %.0fHz: cut=800→%.2f dB cut=12000→%.2f dB", probe, lowGain, highGain)
	}
}

func filterGainAt(pts []audio.FreqResponsePoint, hz float64) float64 {
	best, bestD := 0.0, 1e18
	for _, p := range pts {
		d := p.FreqHz - hz
		if d < 0 {
			d = -d
		}
		if d < bestD {
			bestD, best = d, p.GainDB
		}
	}
	return best
}
