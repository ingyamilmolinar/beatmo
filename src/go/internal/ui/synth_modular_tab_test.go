package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func setupModularSynthGame(t *testing.T) *Game {
	t.Helper()
	g := newSynthTabGame(t)
	g.drum.Rows[0].Instrument = roster.Modular
	_ = audio.InstallInstrumentDocForTest(roster.Modular, roster.Modular)
	t.Cleanup(func() {
		_ = audio.InstallInstrumentDocForTest(roster.Modular, roster.Modular)
		audio.ResetInstrumentParams(roster.Modular)
	})
	// The FM oscillator, so every section is on screen: the FM operators belong to osc.fm and hide on any other.
	audio.SetInstrumentParam(roster.Modular, "osc_type", 4)
	layoutSynthTab(t, g)
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
	// A drum recipe shows the full pipeline too: VOICE for its family knobs plus the enable-able stages, so every
	// instrument shares one section sequence.
	g2 := newSynthTabGame(t) // drum-snare
	g2.drum.eqPanelZone.SetActiveTab(TabSynth)
	g2.drum.eqPanelZone.Layout(g2.drum.eqPanelZone.PanelRect())
	drumIDs := map[synthSectionID]bool{}
	for _, s := range g2.drum.SynthTabSections() {
		drumIDs[s.id] = true
	}
	// FM appears once the FM oscillator is chosen.
	for _, want := range []synthSectionID{
		synthSectionVoice, synthSectionOsc,
		synthSectionEnvelope, synthSectionFilter, synthSectionPost,
	} {
		if !drumIDs[want] {
			t.Errorf("drum-snare synth tab missing standardized section %q", sectionLabel(want))
		}
	}
}

// Every <family>_wave is a discrete enum whose caption shows the waveform name, not a number.
func TestSynthTab_WaveControlCaptionShowsLabel(t *testing.T) {
	for _, id := range []instrument.ID{roster.Kick, roster.Hihat, roster.FmBass} {
		var waveDef *audio.ParamDef
		for _, d := range audio.InstrumentParamDefs(id) {
			if d.Name == "gen1_wave" {
				dd := d
				waveDef = &dd
				break
			}
		}
		if waveDef == nil {
			t.Fatalf("%s: no gen1_wave ParamDef", id)
		}
		for idx, label := range waveDef.Enum {
			got := synthControlCaption(*waveDef, float64(idx))
			if !strings.Contains(got, label) {
				t.Errorf("%s %s caption @%d = %q, want it to contain %q", id, waveDef.Name, idx, got, label)
			}
		}
	}
}

func TestSynthTab_EnumParamCaptionShowsLabel(t *testing.T) {
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 4, Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM"}}
	for idx, label := range def.Enum {
		got := synthControlCaption(def, float64(idx))
		if !strings.Contains(got, label) {
			t.Errorf("osc_type caption @%d = %q, want it to contain %q", idx, got, label)
		}
		gotV := synthControlCaptionValueOnly(def, float64(idx))
		if !strings.Contains(gotV, label) {
			t.Errorf("osc_type value-only caption @%d = %q, want %q", idx, gotV, label)
		}
	}
	// Out-of-range value clamps to a valid label (no panic / index error).
	if got := synthControlCaption(def, 9); !strings.Contains(got, "FM") {
		t.Errorf("osc_type caption @9 = %q, want clamped to FM", got)
	}
}

// The snapping enum writes the nearest integer index, and the write reaches the platform bridge the browser renders
// through.
func TestSynthTab_ModularEnumControlWritesSnappedIndex(t *testing.T) {
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
	oldCb := audio.SwapPlatformInstrumentDocChangedForTest(func(id instrument.ID, d instrument.Doc) {
		if id == roster.Modular {
			gotPushed = audio.FlatFieldMap(d)["osc_type"]
		}
	})
	t.Cleanup(func() { audio.SwapPlatformInstrumentDocChangedForTest(oldCb) })

	// Index 1 (Saw) is 1/(Max-Min) whatever the enum's size, derived from the live binding; the snap lands on exactly 1.
	def := bindings[idx].def
	span := def.Max - def.Min
	if span <= 0 {
		t.Fatalf("osc_type binding has non-positive span: Min=%v Max=%v", def.Min, def.Max)
	}
	sliders[idx].Value = 1.0 / span
	g.drum.propagateSynthSliderValue(idx, roster.Modular)

	got := audio.GetInstrumentParams(roster.Modular)["osc_type"]
	if got != 1 {
		t.Errorf("osc_type stored = %v, want exactly 1 (snapped Saw index)", got)
	}
	if gotPushed != 1 {
		t.Errorf("platform bridge received osc_type = %v, want 1 (the browser renders through this seam)", gotPushed)
	}
}
