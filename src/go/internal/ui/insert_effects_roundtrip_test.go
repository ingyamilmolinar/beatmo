//go:build test

package ui

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The slider writes only the audio layer, so export must read the chain from there; and importing a file with no
// effects must clear the live chain. Real-flow APIs, asserted against audio.GetInsertEffects, which the
// mixer builds from.

// The chains are process-global.
func withCleanInsertEffects(t *testing.T) {
	t.Helper()
	audio.ClearAllInsertEffects()
	t.Cleanup(audio.ClearAllInsertEffects)
}

// An edit made like the slider's (audio layer only) reaches the exported file.
func TestExportInsertEffects_ParamEditReachesFile(t *testing.T) {
	withDefaultAudio(t)
	withCleanInsertEffects(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	slot := audio.AddInsertEffect(inst, "distortion", nil)

	// The slider does not re-sync the row.
	audio.SetInsertEffectParam(inst, slot, "drive", 15)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
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

// Replace-not-merge, like synth params and sample edits.
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

// Every effect type, every param at a non-default value, over several rows with mixed enabled flags and a
// meaningful order: export → fresh import restores the chains exactly.
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
	// Distinct instruments, since chains are keyed by instrument id.
	for len(g.drum.Rows) < 3 {
		g.drum.AddRow()
	}
	g.drum.Rows[1].Instrument = roster.Hihat
	g.drum.Rows[2].Instrument = roster.Clap
	for i := 0; i < 3; i++ {
		for j := i + 1; j < 3; j++ {
			if g.drum.Rows[i].Instrument == g.drum.Rows[j].Instrument {
				t.Fatalf("rows %d and %d share instrument %q; matrix needs distinct ids", i, j, g.drum.Rows[i].Instrument)
			}
		}
	}

	// 73% of the range, nudged off the default.
	valueFor := func(d audio.EffectParamDef) float64 {
		v := d.Min + 0.73*(d.Max-d.Min)
		if math.Abs(v-d.Default) < 1e-12 {
			v = d.Min + 0.37*(d.Max-d.Min)
		}
		return v
	}

	// Every 4th slot disabled, so Enabled round-trips both ways.
	want := map[string][]audio.EffectSlot{}
	for i, typ := range order {
		inst := g.drum.Rows[i%3].Instrument
		params := map[string]float64{}
		for _, d := range catalog[typ] {
			params[d.Name] = valueFor(d)
		}
		want[string(inst)] = append(want[string(inst)], audio.EffectSlot{
			Type:    typ,
			Enabled: i%4 != 3,
			Params:  params,
		})
	}
	for inst, slots := range want {
		audio.SetInsertEffects(instrument.ID(inst), slots)
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
		got := audio.GetInsertEffects(instrument.ID(inst))
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

// Or re-adding that instrument later resurrects a phantom chain.
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
	audio.AddInsertEffect(roster.Crash, "bitcrusher", map[string]float64{"bits": 2})

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(clean); err != nil {
		t.Fatalf("import: %v", err)
	}

	if got := audio.GetInsertEffects(roster.Crash); len(got) != 0 {
		t.Errorf("insert chain on absent instrument survived import: %+v", got)
	}
}
