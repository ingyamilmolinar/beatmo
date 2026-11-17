//go:build !test

package ui

// Breadth coverage for the export → import master-mix contract.
//
// export_import_master_audio_identity_test.go proves the contract on ONE
// hand-built circuit that exercises every subsystem at once. These two go wide
// instead: every shipped template (real projects, real instrument
// combinations), and every builtin instrument in the roster (so a family whose
// recipe serializes badly cannot hide behind the four instruments the headline
// test happens to use).
//
// Real build only — the `-tags test` synth and EQ are stubs.

import (
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// templateRenderSeconds is shorter than the headline test's window: these
// sweeps trade depth for breadth, and every template loops well inside 2s.
const templateRenderSeconds = 2.0

// TestEveryBuiltinTemplateRoundTripsByteIdentical is the LOAD → EDIT → EXPORT →
// IMPORT cycle a user actually performs: open a shipped circuit, change its
// sound, save it, reopen it.
//
// Each template is loaded through the real Import path, edited through the same
// engine entry points the Synth/FX/EQ panels use, rendered, exported, and then
// re-imported after every global audio store has been wiped. The second render
// must match the first byte for byte.
func TestEveryBuiltinTemplateRoundTripsByteIdentical(t *testing.T) {
	tps := assets.Templates()
	if len(tps) == 0 {
		t.Skip("no built-in templates embedded")
	}
	for _, tp := range tps {
		t.Run(tp.Genre, func(t *testing.T) {
			g := New(game_log.New(nil, game_log.LevelError))
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			if err := g.Import(tp.Bytes); err != nil {
				t.Fatalf("load template: %v", err)
			}
			ids := rowInstrumentIDs(g)
			if len(ids) == 0 {
				t.Skip("template has no rows")
			}

			// Edit the loaded circuit the way a user would: a few synth knobs,
			// an insert chain and an EQ curve on the first row.
			editFirstRow(t, g)

			before := renderLiveMasterMix(t, g, templateRenderSeconds)
			data, err := g.drum.exportBytes()
			if err != nil {
				t.Fatalf("export: %v", err)
			}

			wipeGlobalAudioState(t, ids)

			g2 := New(game_log.New(nil, game_log.LevelError))
			t.Cleanup(g2.CloseForTest)
			g2.Layout(1280, 720)
			if err := g2.Import(data); err != nil {
				t.Fatalf("re-import: %v", err)
			}
			assertMasterPCMIdentical(t, before, renderLiveMasterMix(t, g2, templateRenderSeconds),
				"template "+tp.Genre)
		})
	}
}

// TestEveryBuiltinInstrumentMasterMixRoundTrips sweeps the whole instrument
// roster (roster.All, not the catalogue — see CLAUDE.md on why
// those differ) so an instrument whose document serializes badly is caught even
// when no template or headline test uses it.
//
// The overlay applied here is SPARSE, which is the shape the Synth tab writes:
// the exporter turns it into a delta against shipped defaults and the importer
// pins shipped ⊕ delta, a much larger map. Those two must resolve to the same
// sound, and that equivalence is the whole reason the pinning trick is safe.
func TestEveryBuiltinInstrumentMasterMixRoundTrips(t *testing.T) {
	if testing.Short() {
		t.Skip("roster sweep: -short")
	}
	for _, id := range roster.All {
		if _, ok := audio.LibraryDoc(id); !ok {
			continue
		}
		t.Run(string(id), func(t *testing.T) {
			g := oneRowGame(t, id, func(g *Game) {
				applySparseKnobEdits(string(id), 6)
				audio.SetInsertEffects(id, []audio.EffectSlot{
					{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 1.4, "mix": 0.35, "tone": 4000}},
				})
				rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
				rowEQ(g.drum.Rows[0]).GainsDB[4] = 3
				rowEQ(g.drum.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
				rowEQ(g.drum.Rows[0]).HPF = true
				rowEQ(g.drum.Rows[0]).HPFHz = 45
				g.drum.applyRowEQ(0)
			})
			before := renderLiveMasterMix(t, g, templateRenderSeconds)
			data, err := g.drum.exportBytes()
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			beforeState := captureAudibleState(id)

			wipeGlobalAudioState(t, []instrument.ID{id})

			g2 := New(game_log.New(nil, game_log.LevelError))
			t.Cleanup(g2.CloseForTest)
			g2.Layout(1280, 720)
			if err := g2.Import(data); err != nil {
				t.Fatalf("import: %v", err)
			}
			if d := beforeState.diff(captureAudibleState(id)); len(d) > 0 {
				t.Errorf("config not restored:\n  - %v", d)
			}
			assertMasterPCMIdentical(t, before, renderLiveMasterMix(t, g2, templateRenderSeconds), string(id))
		})
	}
}

// rowInstrumentIDs returns the instrument id of every row, deduplicated.
func rowInstrumentIDs(g *Game) []instrument.ID {
	seen := map[instrument.ID]bool{}
	var out []instrument.ID
	for _, r := range g.drum.Rows {
		if r.Instrument == "" || seen[r.Instrument] {
			continue
		}
		seen[r.Instrument] = true
		out = append(out, r.Instrument)
	}
	return out
}

// editFirstRow applies a representative user edit — synth knobs, an insert
// chain and an EQ curve — to row 0.
func editFirstRow(t *testing.T, g *Game) {
	t.Helper()
	if len(g.drum.Rows) == 0 {
		return
	}
	id := g.drum.Rows[0].Instrument
	if audio.HasInstrumentDoc(id) {
		applySparseKnobEdits(string(id), 4)
	}
	audio.SetInsertEffects(id, []audio.EffectSlot{
		{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 1.6, "mix": 0.4, "tone": 3500}},
		{Type: "tremolo", Enabled: true, Params: map[string]float64{"depth": 0.45, "rate": 5, "mix": 1, "shape": 1}},
	})
	rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
	rowEQ(g.drum.Rows[0]).GainsDB[6] = -4
	rowEQ(g.drum.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
	rowEQ(g.drum.Rows[0]).LPF = true
	rowEQ(g.drum.Rows[0]).LPFHz = 11000
	g.drum.applyRowEQ(0)
	audio.SetMainVolume(0.66)
	g.updateBeatInfos()
}

// applySparseKnobEdits moves up to `n` of the instrument's declared params off
// their current value, one call to SetInstrumentParam at a time — the
// sparse-overlay shape the Synth tab produces. Declared fields are walked in
// sorted order with a fixed stride so the selection is deterministic across
// runs and across machines (an undeclared field is not a knob and would be
// rejected).
func applySparseKnobEdits(id string, n int) {
	cur := audio.MergedInstrumentParamsRO(instrument.ID(id))
	keys := make([]string, 0, len(cur))
	for k := range cur {
		if audio.InstrumentDeclaresField(instrument.ID(id), k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	vals := make(map[string]float64, len(keys))
	for _, k := range keys {
		vals[k] = cur[k]
	}
	stride := 1
	if n > 0 && len(keys) > n {
		stride = len(keys) / n
	}
	applied := 0
	for i := 0; i < len(keys) && applied < n; i += stride {
		audio.SetInstrumentParam(instrument.ID(id), keys[i], vals[keys[i]]+0.25)
		applied++
	}
}
