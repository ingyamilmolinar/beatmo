//go:build test

package ui

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Render-level / source-of-truth guards for per-instrument insert effect
// chains across export → import. The pre-existing tests set row.Effects
// directly, which hid two real-flow gaps:
//
//  1. The FX panel's param slider writes ONLY to the audio layer
//     (audio.SetInsertEffectParam — drumview_fx_panel.go propagateFXSliderValue
//     never re-syncs the row), while export read the stale row copy. An
//     edited drive knob exported its pre-edit value.
//  2. Import applied chains only when the file carried them; a project
//     WITHOUT effects imported over a session WITH effects left the old
//     chain processing audio (invisible in the UI — rows are rebuilt empty).
//
// These tests drive the REAL flow (audio-layer APIs, like the FX panel does)
// and assert against audio.GetInsertEffects — the single source of truth the
// mixer builds processor chains from.

// withCleanInsertEffects clears the process-global insert chains before and
// after the test so chains can't leak between tests in this package.
func withCleanInsertEffects(t *testing.T) {
	t.Helper()
	audio.ClearAllInsertEffects()
	t.Cleanup(audio.ClearAllInsertEffects)
}

// TestExportInsertEffects_ParamEditReachesFile reproduces the FX-slider
// export drift: add an effect through the real flow, then edit a param the
// way the slider does (audio layer only, no row re-sync) and export. The
// file must carry the EDITED value.
func TestExportInsertEffects_ParamEditReachesFile(t *testing.T) {
	withDefaultAudio(t)
	withCleanInsertEffects(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	slot := audio.AddInsertEffect(inst, "distortion", nil)
	g.drum.syncFXToRow(0) // FX panel does this after Add

	// Slider drag: audio layer only — propagateFXSliderValue does NOT re-sync
	// the row (drumview_fx_panel.go).
	audio.SetInsertEffectParam(inst, slot, "drive", 15)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	var fx []audio.EffectSlot
	for _, ei := range f.Instruments {
		if ei.ID == inst {
			fx = ei.Effects
		}
	}
	if len(fx) != 1 || fx[0].Type != "distortion" {
		t.Fatalf("exported effects for %q: %+v", inst, fx)
	}
	if math.Abs(fx[0].Params["drive"]-15) > 1e-9 {
		t.Errorf("exported drive = %v, want 15 (the slider edit; export must read the audio layer, not the stale row copy)",
			fx[0].Params["drive"])
	}
}

// TestImportClearsStaleInsertEffects: importing a project WITHOUT effects
// must clear a live chain on the same instrument id — replace-not-merge,
// like synth params and sample edits.
func TestImportClearsStaleInsertEffects(t *testing.T) {
	withDefaultAudio(t)
	withCleanInsertEffects(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	// Project captured BEFORE any effects exist.
	clean, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	audio.AddInsertEffect(inst, "distortion", map[string]float64{"drive": 18, "mix": 1})
	if got := audio.GetInsertEffects(inst); len(got) != 1 {
		t.Fatalf("precondition: chain not installed: %+v", got)
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(clean); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := audio.GetInsertEffects(inst); len(got) != 0 {
		t.Errorf("stale insert chain survived import of an effects-free project: %+v (audio keeps processing FX the UI doesn't show)", got)
	}
}

// TestInsertEffectsRoundTrip_FullCatalogMatrix: every registered effect type,
// each with every declared param at a NON-default in-range value, spread over
// several rows with mixed enabled flags and a meaningful slot order — export →
// fresh import must restore the audio-layer chains EXACTLY (type, order,
// enabled, every param). This is the comprehensive guard the previous tests
// (2 types, 3 params) didn't provide.
func TestInsertEffectsRoundTrip_FullCatalogMatrix(t *testing.T) {
	withDefaultAudio(t)
	withCleanInsertEffects(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	catalog := audio.InsertEffectCatalog()
	order := audio.EffectTypeOrder()
	if len(order) < 10 {
		t.Fatalf("expected a full effect registry, got %d types", len(order))
	}
	// Three rows with distinct builtin instruments so chains are keyed and
	// restored per instrument id.
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}
	g.drum.Rows[1].Instrument = "hihat"
	g.drum.Rows[2].Instrument = "clap"
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if g.drum.Rows[i].Instrument == g.drum.Rows[j].Instrument {
				t.Fatalf("rows %d and %d share instrument %q; matrix needs distinct ids", i, j, g.drum.Rows[i].Instrument)
			}
		}
	}

	// Non-default, in-range value for a param: 73% of the range, nudged if it
	// happens to land on the default.
	valueFor := func(d audio.EffectParamDef) float64 {
		v := d.Min + 0.73*(d.Max-d.Min)
		if math.Abs(v-d.Default) < 1e-12 {
			v = d.Min + 0.37*(d.Max-d.Min)
		}
		return v
	}

	// Distribute the whole catalog across 3 rows; every 4th slot disabled so
	// the Enabled flag round-trips in both states.
	want := map[string][]audio.EffectSlot{}
	for i, typ := range order {
		inst := g.drum.Rows[i%3].Instrument
		params := map[string]float64{}
		for _, d := range catalog[typ] {
			params[d.Name] = valueFor(d)
		}
		want[inst] = append(want[inst], audio.EffectSlot{
			Type:    typ,
			Enabled: i%4 != 3,
			Params:  params,
		})
	}
	for inst, slots := range want {
		audio.SetInsertEffects(inst, slots)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	audio.ClearAllInsertEffects()

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	for inst, slots := range want {
		got := audio.GetInsertEffects(inst)
		if len(got) != len(slots) {
			t.Errorf("%s: chain length %d, want %d", inst, len(got), len(slots))
			continue
		}
		for i := range slots {
			if got[i].Type != slots[i].Type {
				t.Errorf("%s[%d]: type %q, want %q (order must survive)", inst, i, got[i].Type, slots[i].Type)
			}
			if got[i].Enabled != slots[i].Enabled {
				t.Errorf("%s[%d] %s: enabled %v, want %v", inst, i, slots[i].Type, got[i].Enabled, slots[i].Enabled)
			}
			for name, w := range slots[i].Params {
				if v, ok := got[i].Params[name]; !ok || math.Abs(v-w) > 1e-12 {
					t.Errorf("%s[%d] %s.%s: got %v, want %v", inst, i, slots[i].Type, name, v, w)
				}
			}
		}
	}
}

// TestImportClearsChainsForInstrumentsAbsentFromProject: a chain on an
// instrument id that does not appear in the imported project at all must
// not survive either — otherwise re-adding that instrument later in the
// session resurrects a phantom chain.
func TestImportClearsChainsForInstrumentsAbsentFromProject(t *testing.T) {
	withDefaultAudio(t)
	withCleanInsertEffects(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	clean, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// An id outside the default project's instrument set.
	audio.AddInsertEffect("crash", "bitcrusher", map[string]float64{"bits": 2})

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(clean); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := audio.GetInsertEffects("crash"); len(got) != 0 {
		t.Errorf("insert chain on absent instrument survived import: %+v", got)
	}
}
