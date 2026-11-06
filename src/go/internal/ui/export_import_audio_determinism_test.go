//go:build !test

package ui

// Determinism and completeness guards for the export → import path.
//
// These sit alongside export_import_master_audio_identity_test.go and cover the
// three ways a project file stopped being a faithful description of its sound:
//
//  1. Stateful DSP that survived a discontinuity, so the same circuit played
//     differently depending on what ran before it (audio.ResetDSPState).
//  2. A Save-As instrument the file NAMED but did not DEFINE, so the tone existed
//     only on the machine that created it (now: projectFile.InstrumentDocs).
//  3. A wall-clock-derived user id, so the same project exported twice was two
//     different documents (now content addressed: audio.NewUserInstrumentID,
//     guarded by TestNewUserInstrumentIDIsContentAddressed).
//
// Real build only — the `-tags test` synth and EQ are stubs.

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// oneRowGame builds a minimal two-node loop on row 0 so a render has repeated
// hits to work with, and applies `setup` before the paths are computed.
func oneRowGame(t *testing.T, id instrument.ID, setup func(g *Game)) *Game {
	t.Helper()
	withPrivateInstrumentLibrary(t)
	restoreLibraryCopies(t, id)
	audio.ClearAllInsertEffects()
	audio.ResetEQSettings() // instrument EQ settings are process-global too
	audio.ClearChannelProcessors(channel.Of(id))

	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(4, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.drum.selRow = 0
	g.drum.SetInstrument(id)
	// Master volume and master EQ are process-global and are what Import
	// rewrites from the file. Normalize them to the same values an EQ-less
	// project imports to, so a sibling test that
	// boosted the master cannot make "before" and "after" differ for reasons
	// that have nothing to do with what is under test.
	audio.SetMainVolume(0.5)
	for i := range g.drum.eqBandGainsDB() {
		g.drum.eqBandGainsDB()[i] = 0
	}
	for i := range g.drum.eqBandMuted() {
		g.drum.eqBandMuted()[i] = false
	}
	g.drum.hpfEnabled, g.drum.hpfCutoffHz = false, 20
	g.drum.lpfEnabled, g.drum.lpfCutoffHz = false, 20000
	g.drum.applyMasterEQ()
	if setup != nil {
		setup(g)
	}
	g.updateBeatInfos()
	return g
}

// TestEveryDSPStageRendersIdenticallyOnReplay walks the mix chain one stateful
// stage at a time and asserts each renders the same bytes twice.
//
// Every one of these failed before audio.ResetDSPState existed, each for its own
// reason: EQ and the distortion's tone filter carry a biquad delay line, the
// tremolo carries LFO PHASE (which decides the gain of sample 0, so 100% of the
// output moved), the reverb send carries a tail, and a loud master leaves the
// compressor envelope part-way down. A per-stage matrix rather than one
// composite case because the composite tells you only that something is
// non-deterministic, never which stage to go fix.
func TestEveryDSPStageRendersIdenticallyOnReplay(t *testing.T) {
	const id = roster.Kick
	cases := []struct {
		name  string
		setup func(g *Game)
	}{
		{"no processing", nil},
		{"per-instrument EQ", func(g *Game) {
			rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
			rowEQ(g.drum.Rows[0]).GainsDB[1] = -6
			rowEQ(g.drum.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
			g.drum.applyRowEQ(0)
		}},
		{"master EQ", func(g *Game) {
			g.drum.eqBandGainsDB()[3] = -5
			g.drum.applyMasterEQ()
		}},
		{"insert FX: distortion", func(g *Game) {
			audio.SetInsertEffects(id, []audio.EffectSlot{
				{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 1.7, "mix": 0.45, "tone": 3200}}})
		}},
		{"insert FX: tremolo (LFO phase)", func(g *Game) {
			audio.SetInsertEffects(id, []audio.EffectSlot{
				{Type: "tremolo", Enabled: true, Params: map[string]float64{"depth": 0.6, "rate": 4.5, "mix": 1, "shape": 1.2}}})
		}},
		{"insert FX: reverb (tail)", func(g *Game) {
			audio.SetInsertEffects(id, []audio.EffectSlot{
				{Type: "reverb", Enabled: true, Params: map[string]float64{"room": 0.7, "damping": 0.4, "mix": 0.4}}})
		}},
		{"insert FX: delay (tail)", func(g *Game) {
			audio.SetInsertEffects(id, []audio.EffectSlot{
				{Type: "delay", Enabled: true, Params: map[string]float64{"time": 300, "feedback": 0.4, "mix": 0.4}}})
		}},
		{"master compressor envelope", func(g *Game) {
			audio.SetMainVolume(1)
			g.drum.Rows[0].Volume = 4
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := oneRowGame(t, id, tc.setup)
			first := renderLiveMasterMix(t, g, 2.0)
			second := renderLiveMasterMix(t, g, 2.0)
			assertMasterPCMIdentical(t, first, second, "replayed render")
		})
	}
}

// TestImportResetsDSPStateFromThePreviousProject pins the reason ResetDSPState
// is called from Import and not only from the render harness: loading a project
// is a discontinuity, and the incoming circuit must not inherit the outgoing
// one's filter memory, LFO phase, compressor envelope or reverb tail.
func TestImportResetsDSPStateFromThePreviousProject(t *testing.T) {
	const id = roster.Kick
	g := oneRowGame(t, id, func(g *Game) {
		audio.SetInsertEffects(id, []audio.EffectSlot{
			{Type: "tremolo", Enabled: true, Params: map[string]float64{"depth": 0.9, "rate": 3.3, "mix": 1, "shape": 1}},
			{Type: "reverb", Enabled: true, Params: map[string]float64{"room": 0.7, "damping": 0.4, "mix": 0.5}}})
	})
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	pristine := renderLiveMasterMix(t, g, 2.0)

	// Dirty every stateful stage by running audio through the live chain, then
	// import. A clean import must land on the pristine render regardless.
	_ = renderLiveMasterMix(t, g, 2.0)
	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	assertMasterPCMIdentical(t, pristine, renderLiveMasterMix(t, g2, 2.0), "import after a dirtied chain")
}

// TestSaveAsInstrumentSurvivesExportImportOnAMachineThatNeverSawIt is the
// portability guard for Save As.
//
// A Save As writes a NEW user library instrument and repoints the row's
// document at it. Before instrument documents the file merely NAMED that user
// recipe and carried not one synth param; on the machine that made it that
// still sounded right — the recipe was registered — which is exactly why the
// loss was invisible. An empty library here is what a different machine,
// another browser profile, or cleared storage looks like.
func TestSaveAsInstrumentSurvivesExportImportOnAMachineThatNeverSawIt(t *testing.T) {
	const id = roster.Kick
	g := oneRowGame(t, id, func(g *Game) {
		audio.SetInstrumentParam(id, "gen1_kick_sat", 3.5)
		audio.SetInstrumentParam(id, "pitchenv_amt", 18)
	})
	newID := g.drum.SaveActiveRecipeAs("Fat Kick")
	if newID == "" {
		t.Fatal("SaveActiveRecipeAs returned no id")
	}
	if d, _ := audio.InstrumentDoc(id); d.Origin != newID {
		t.Fatalf("the row's document is not repointed at the saved instrument: origin %q", d.Origin)
	}
	before := renderLiveMasterMix(t, g, 2.0)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d, ok := f.InstrumentDocs[id]; !ok || d.Origin != newID {
		t.Fatalf("export does not carry the Save-As document for %q (origin %q)", id, d.Origin)
	}

	// The recipient has never seen this instrument.
	if err := audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs()
	audio.ClearAllInsertEffects()

	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	assertMasterPCMIdentical(t, before, renderLiveMasterMix(t, g2, 2.0), "Save-As tone on a fresh machine")
}

// TestImportNeverOverwritesTheLibrary: a project file is attacker- and
// accident-controlled input. Its documents are the PROJECT's copies; importing
// one must never re-tune the instrument for every other project (the library).
func TestImportNeverOverwritesTheLibrary(t *testing.T) {
	g := oneRowGame(t, roster.Kick, nil)
	before, _ := audio.LibraryDoc(roster.Kick)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	hostile, err := instrument.SetField(f.InstrumentDocs[roster.Kick].Doc, audio.BuiltinModuleRegistry(), "gen1_kick_sat", 4)
	if err != nil {
		t.Fatal(err)
	}
	f.InstrumentDocs[roster.Kick] = projectDoc{Doc: hostile}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(raw); err != nil {
		t.Fatalf("import: %v", err)
	}
	after, _ := audio.LibraryDoc(roster.Kick)
	if d := instrument.Diff(before, after); len(d) != 0 {
		t.Fatalf("import rewrote the library's kick: %v", d)
	}
	if v := audio.GetInstrumentParams(roster.Kick)["gen1_kick_sat"]; math.Abs(v-4) > 0 {
		t.Errorf("the project copy did not take the file's value: %v", v)
	}
}

// TestImportClearsThePreviousProjectsInstrumentEQ: import is replace-not-merge,
// and that has to reach the audio layer. The exporter omits a flat `eq` block,
// and the importer used to apply EQ only when one was present — so an
// instrument that is flat in the incoming project kept the OUTGOING project's
// curve on its channel, audible while the rebuilt row read flat in the UI.
// (The master-EQ branch already handled this; rows did not.)
func TestImportClearsThePreviousProjectsInstrumentEQ(t *testing.T) {
	const id = roster.Snare
	flat := oneRowGame(t, id, nil)
	flatData, err := flat.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(flatData, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if f.Instruments[0].EQ != nil {
		t.Fatal("precondition: a flat instrument must export no eq block")
	}
	want := renderLiveMasterMix(t, flat, 2.0)

	// A previous project left a loud curve on that instrument's channel.
	loud := oneRowGame(t, id, func(g *Game) {
		rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
		rowEQ(g.drum.Rows[0]).GainsDB[2] = 18
		rowEQ(g.drum.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
		g.drum.applyRowEQ(0)
	})
	if dirty := renderLiveMasterMix(t, loud, 2.0); len(dirty) == len(want) {
		if _, _, n := pcmIdentical(want, dirty); n == 0 {
			t.Fatal("precondition: the loud EQ must actually change the render")
		}
	}

	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(flatData); err != nil {
		t.Fatalf("import: %v", err)
	}
	assertMasterPCMIdentical(t, want, renderLiveMasterMix(t, g2, 2.0), "EQ-less project imported over a boosted one")
}

// TestMasterEQSurvivesExportImportWithAnInstrumentChannelSelected pins the
// channel the master EQ is read from and written to.
//
// `dv.eqBandGainsDB()` is `eqPanelZone.bandGainsDB` — the audio panel's WORKING
// COPY of whichever channel is on screen, not the master's store. Export, import
// and applyMasterEQ all read it as if it were the master, so a single click on
// the channel pill (selecting any instrument) silently changed three things:
// the file exported that instrument's curve as the master EQ — or, for a flat
// instrument, dropped the `eq` block entirely and lost the master EQ for good;
// importing wrote the file's master curve into the instrument's display buffer
// and left the master bus untouched; and applyMasterEQ pushed the instrument's
// curve onto the master bus.
//
// Caught by the cross-engine browser test (xplat_browser_export_import), which
// selects an instrument channel to edit its EQ — the natural way to drive it —
// and then found the re-export missing a block on every engine. It is not a
// browser bug at all: it reproduces natively, which is what this test pins.
func TestMasterEQSurvivesExportImportWithAnInstrumentChannelSelected(t *testing.T) {
	const id = roster.Kick
	g := oneRowGame(t, id, nil)
	dv := g.drum

	// Author a master EQ the normal way: Master selected, edit the bands.
	dv.setEQActiveChannel(channel.Master)
	gains := dv.eqBandGainsDB()
	for i := range gains {
		gains[i] = 0
	}
	gains[2], gains[6] = -7.5, 4.25
	dv.applyMasterEQ()

	authored, err := dv.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var want projectFile
	if err := json.Unmarshal(authored, &want); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if want.EQ == nil {
		t.Fatal("precondition: authoring a master EQ must export an eq block")
	}
	masterEQAudio := renderLiveMasterMix(t, g, 2.0)

	// The user clicks the channel pill and picks an instrument. Nothing about
	// the master EQ has changed — so neither may the file, nor the master bus.
	dv.setEQActiveChannel(channel.Of(id))
	withInstrument, err := dv.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var got projectFile
	if err := json.Unmarshal(withInstrument, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a, b := mustJSON(t, want.EQ), mustJSON(t, got.EQ); a != b {
		t.Errorf("selecting an instrument channel changed the exported MASTER eq:\n  want %s\n  got  %s", a, b)
	}
	dv.applyMasterEQ()
	assertMasterPCMIdentical(t, masterEQAudio, renderLiveMasterMix(t, g, 2.0),
		"master bus after selecting an instrument channel")

	// And the round trip, imported while an instrument channel is selected.
	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	g2.drum.setEQActiveChannel(channel.Of(id))
	if err := g2.Import(authored); err != nil {
		t.Fatalf("import: %v", err)
	}
	reexport, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	var back projectFile
	if err := json.Unmarshal(reexport, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a, b := mustJSON(t, want.EQ), mustJSON(t, back.EQ); a != b {
		t.Errorf("master eq not restored by an import made while an instrument channel was selected:\n  want %s\n  got  %s", a, b)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
