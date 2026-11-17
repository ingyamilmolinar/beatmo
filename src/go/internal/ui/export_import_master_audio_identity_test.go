//go:build !test

package ui

// export → import MASTER-MIX audio identity.
//
// The contract these tests pin: a project file is a COMPLETE description of
// everything that can be heard. Load a circuit, edit it, render N seconds of
// the master bus, export, wipe every global audio store, import, render again —
// the two renders must be byte-identical. Not "close", not "same RMS":
// identical, because every stage from the voice to the limiter is a pure
// function of configuration plus a seeded PRNG.
//
// Why the MASTER bus and not per-instrument voices: a per-voice comparison
// (export_import_playback_fidelity_test.go and friends) cannot see channel
// volume, insert-effect chains, per-channel EQ, master EQ, the master
// compressor or the limiter. Those are where a
// dropped field is loudest and where a per-voice test is blind.
//
// Real build only — see the harness file for why `-tags test` cannot host this.
//
// Harness: export_import_master_audio_harness_test.go.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// ─────────────────────────── audible-state snapshot ──────────────────────────

// audibleState is every engine-held value that can change the master mix for
// one instrument. The export must be able to reconstruct all of it; anything
// added to the engine that affects sound belongs here too.
type audibleState struct {
	Origin      string             // the library entry the document came from
	Effective   map[string]float64 // the instrument document's owned fields
	Effects     []audio.EffectSlot
	ChanVolume  float64
	SampleEdit  map[string]float64
	SamplePCMLn int
	DisplayName string
}

func captureAudibleState(id instrument.ID) audibleState {
	ch := channel.Of(id)
	st := audibleState{
		Effects:     audio.GetInsertEffects(id),
		ChanVolume:  audio.ChannelVolume(ch),
		DisplayName: audio.InstrumentDisplayName(id),
	}
	if d, ok := audio.InstrumentDoc(id); ok {
		st.Origin = string(d.Origin)
	}
	st.Effective = map[string]float64(audio.GetInstrumentParams(id))
	if e, ok := audio.SampleEditFor(id); ok {
		st.SampleEdit = e.Fields()
	}
	if rec, ok := audio.UserSamplePCM(id); ok {
		st.SamplePCMLn = len(rec.PCM)
	}
	return st
}

// diff returns a human-readable list of every field that changed, empty when
// the two snapshots are equal.
func (a audibleState) diff(b audibleState) []string {
	var out []string
	if a.Origin != b.Origin {
		out = append(out, fmt.Sprintf("document origin %q -> %q", a.Origin, b.Origin))
	}
	keys := map[string]bool{}
	for k := range a.Effective {
		keys[k] = true
	}
	for k := range b.Effective {
		keys[k] = true
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		va, oka := a.Effective[k]
		vb, okb := b.Effective[k]
		switch {
		case oka && !okb:
			out = append(out, fmt.Sprintf("param %q LOST (was %v)", k, va))
		case !oka && okb:
			out = append(out, fmt.Sprintf("param %q ADDED (%v)", k, vb))
		case math.Abs(va-vb) > 0:
			out = append(out, fmt.Sprintf("param %q %v -> %v", k, va, vb))
		}
	}
	ja, _ := json.Marshal(a.Effects)
	jb, _ := json.Marshal(b.Effects)
	if string(ja) != string(jb) {
		out = append(out, fmt.Sprintf("insert effects %s -> %s", ja, jb))
	}
	for _, f := range []struct {
		name string
		x, y float64
	}{
		{"channel volume", a.ChanVolume, b.ChanVolume},
	} {
		if f.x != f.y {
			out = append(out, fmt.Sprintf("%s %v -> %v", f.name, f.x, f.y))
		}
	}
	sa, _ := json.Marshal(a.SampleEdit)
	sb, _ := json.Marshal(b.SampleEdit)
	if string(sa) != string(sb) {
		out = append(out, fmt.Sprintf("sample edit %s -> %s", sa, sb))
	}
	if a.SamplePCMLn != b.SamplePCMLn {
		out = append(out, fmt.Sprintf("user sample frames %d -> %d", a.SamplePCMLn, b.SamplePCMLn))
	}
	if a.DisplayName != b.DisplayName {
		out = append(out, fmt.Sprintf("display name %q -> %q", a.DisplayName, b.DisplayName))
	}
	return out
}

// masterState is the global (non per-instrument) half of the audible surface.
type masterState struct {
	MainVolume   float64
	BPM, Subdiv  int
	MasterGains  []float64
	MasterMuted  []bool
	HPFOn, LPFOn bool
	HPFHz, LPFHz float64
}

func captureMasterState(g *Game) masterState {
	return masterState{
		MainVolume:  audio.MainVolume(),
		BPM:         g.drum.BPM(),
		Subdiv:      currentMaxDiv(),
		MasterGains: append([]float64(nil), g.drum.eqBandGainsDB()...),
		MasterMuted: append([]bool(nil), g.drum.eqBandMuted()...),
		HPFOn:       g.drum.hpfEnabled, LPFOn: g.drum.lpfEnabled,
		HPFHz: g.drum.hpfCutoffHz, LPFHz: g.drum.lpfCutoffHz,
	}
}

// ───────────────────────────── global reset ──────────────────────────────────

// wipeGlobalAudioState returns every process-global audio store the import must
// repopulate to a known-wrong state. Anything the file fails to carry shows up
// as an audible difference instead of silently surviving in a global the test
// never cleared — the failure mode that lets a "round-trip" test pass while the
// file is lossy.
func wipeGlobalAudioState(t *testing.T, ids []instrument.ID) {
	t.Helper()
	audio.ClearAllInsertEffectsAndNotify()
	audio.ResetInstrumentDocs()
	for _, id := range ids {
		audio.ClearChannelProcessors(channel.Of(id))
		audio.SetChannelVolume(channel.Of(id), 1)
		audio.ClearSampleEdit(id)
		audio.ClearInstrumentDisplayName(id)
	}
	// Deliberately wrong global values: a field the file omits will be heard.
	audio.SetMainVolume(1)
}

// wipeToCleanMachine is wipeGlobalAudioState plus the state a DIFFERENT machine
// would not have: an empty user library (no Save, no Save As). The file alone
// must then rebuild the tone.
func wipeToCleanMachine(t *testing.T, ids []instrument.ID) {
	t.Helper()
	if err := audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil); err != nil {
		t.Fatal(err)
	}
	wipeGlobalAudioState(t, ids)
}

// ───────────────────────────── the circuit ───────────────────────────────────

// editedProject is the headline circuit: every audible determinant edited
// (editedProjectUnsaved), then the Synth tab's Save and Save As pressed on top
// of those live edits, then MORE live edits nobody saves — the order a user
// actually works in. Returns the game and the instrument ids in row order.
func editedProject(t *testing.T) (*Game, []instrument.ID) {
	t.Helper()
	g, ids := editedProjectUnsaved(t)
	applySynthTabSaves(t, g, ids)
	applyPostSaveEdits(t, g, ids)
	g.updateBeatInfos()
	return g, ids
}

// applySynthTabSaves presses the Synth tab's Save on rows 0 and 1 and Save As
// on row 2, each through the button's own entry point with that row's channel
// selected (the Synth tab edits the selected channel). Save copies the
// instrument's document into the user library; Save As writes a new user
// instrument and repoints the row's document at it. Neither may change a single
// sample of what is heard. The library is the test's private one
// (editedProjectUnsaved). Returns the Save-As instrument id.
func applySynthTabSaves(t *testing.T, g *Game, ids []instrument.ID) string {
	t.Helper()
	for _, id := range ids[:2] {
		g.drum.selectAudioChannel(channel.Of(id))
		if got := saveRecipeForInstrument(id); got != id {
			t.Fatalf("Synth-tab Save on %q saved %q, want %q", id, got, id)
		}
	}
	g.drum.selectAudioChannel(channel.Of(ids[2]))
	userID := g.drum.SaveActiveRecipeAs(string("Tuned " + ids[2]))
	if d, _ := audio.InstrumentDoc(ids[2]); userID == "" || d.Origin != userID {
		t.Fatalf("Synth-tab Save As on %q: got %q, document origin %q", ids[2], userID, d.Origin)
	}
	// Back to Master: the master-EQ edits in editedProjectUnsaved went through
	// the panel's Master view, and the file must see them there.
	g.drum.selectAudioChannel(channel.Master)
	return string(userID)
}

// applyPostSaveEdits makes the edits a user makes AFTER saving and never saves:
// knobs on a Saved instrument and a Sampler edit. The export must carry them
// anyway — exporting is the save that matters.
//
// The knobs are ones the Synth tab actually OFFERS for the instrument (a fixed
// stride through the schema used to land on unused gen slots, i.e. edits no
// user can make and no render reads). The Sampler edit is process-global, so it
// is cleared in Cleanup — left behind, the next test's "saved" render already
// carried it and re-applying it changed nothing.
func applyPostSaveEdits(t *testing.T, g *Game, ids []instrument.ID) {
	t.Helper()
	defs := audio.InstrumentParamDefs(ids[0])
	if len(defs) == 0 {
		t.Fatalf("post-save edits: %q has no instrument document", ids[0])
	}
	fam, famOK := activeFamilyStage(defs)
	merged := audio.MergedInstrumentParamsRO(ids[0])
	edited := 0
	for _, d := range defs {
		if edited == 3 {
			break
		}
		if len(d.Enum) > 0 || d.Max <= d.Min || !synthParamIsGridControlIn(d, fam, famOK, merged) {
			continue
		}
		v := merged[d.Name] + 0.15*(d.Max-d.Min)
		if v > d.Max {
			v = merged[d.Name] - 0.15*(d.Max-d.Min)
		}
		audio.SetInstrumentParam(ids[0], d.Name, v)
		edited++
	}
	if edited == 0 {
		t.Fatalf("post-save edits: the Synth tab offers no numeric knob on %q", ids[0])
	}
	// A Synth-tab MACRO DIAL on another row: a lens whose edits are plain
	// param writes, so they must travel in instrument_docs like any knob edit.
	turnHeadlineDial(t, g, ids[1])
	// The Sampler tab's Save writes a non-destructive edit descriptor.
	audio.SetSampleEdit(ids[3], audio.SampleEditFromFields(map[string]float64{
		"start_frac": 0.01, "end_frac": 0.85, "transpose_semis": -2, "detune_cents": 7,
		"gain_db": -1.5, "fade": 1, "normalize": 1,
	}))
	t.Cleanup(func() { audio.ClearSampleEdit(ids[3]) })
}

// headlineChordInst is the headline row whose second node is a chord node.
const headlineChordInst = roster.OrganChurch

// headlineDial is the dial the headline circuit turns (on ids[1], the snare):
// Length, which turns both ways on a snare (its decay rates up, FINISH Length
// down) — Bright there is down-only since FINISH Crisp left the dial.
const headlineDialCard, headlineDialID = "length", "length"

// turnHeadlineDial opens the Synth tab on inst and turns one macro dial
// through its real input handler (press → drag → release, which applies live
// and commits once). Returns the instrument's params from before the turn, so
// a caller can take the dial edit back out.
func turnHeadlineDial(t *testing.T, g *Game, inst instrument.ID) audio.RecipeParams {
	t.Helper()
	dv := g.drum
	before := audio.GetInstrumentParams(inst)
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.SetActiveChannel(channel.Of(inst))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	resolved := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if resolved != inst {
		t.Fatalf("Synth tab opened on %q, want %q", resolved, inst)
	}
	if _, ok := dv.synthDialByID(headlineDialCard, headlineDialID); !ok {
		t.Fatalf("%q has no %s/%s dial", inst, headlineDialCard, headlineDialID)
	}
	h := &synthDialHitAdapter{dv: dv, instID: inst, card: headlineDialCard, dial: headlineDialID}
	h.OnPress(100, 300)
	for y := 300; y >= 220; y -= 8 {
		h.OnDrag(100, y)
	}
	h.OnRelease(100, 220)
	if paramsEqual(before, audio.GetInstrumentParams(inst)) {
		t.Fatalf("turning %s/%s on %q changed no param", headlineDialCard, headlineDialID, inst)
	}
	return before
}

// editedProjectUnsaved builds a multi-row circuit and then edits EVERY audible
// determinant through the same engine entry points the UI uses. Returns the
// game and the instrument ids in row order.
func editedProjectUnsaved(t *testing.T) (*Game, []instrument.ID) {
	t.Helper()
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	ids := []instrument.ID{roster.Kick, roster.Snare, roster.Hihat, roster.OrganChurch}
	// The instrument documents and the user library are process-global: start
	// from a private, empty library and the shipped documents so a sibling test
	// that edited or saved one of these instruments cannot change what this test
	// is measuring.
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	restoreLibraryCopies(t, ids...)
	audio.ClearAllInsertEffects()
	for _, id := range ids {
		audio.ClearSampleEdit(id) // process-global, like the bindings above
	}

	// A small loop per row so every row fires repeatedly inside the window.
	for i, id := range ids {
		if i >= len(g.drum.Rows) {
			g.drum.AddRow()
		}
		a := g.tryAddNode(i*8, i*2, model.NodeTypeRegular)
		b := g.tryAddNode(i*8+4, i*2, model.NodeTypeRegular)
		g.addEdge(a, b)
		g.addEdge(b, a)
		g.drum.Rows[i].Origin = a.ID
		g.drum.Rows[i].Node = a
		g.drum.selRow = i
		g.drum.SetInstrument(id)

		// Per-node params: pitch, volume and duration all reach the voice.
		na := g.graph.Nodes[a.ID]
		na.Params.Pitch = float64(i) - 1
		na.Params.Volume = 0.8 + 0.05*float64(i)
		na.Params.Duration = 0.9 + 0.1*float64(i)
		g.graph.Nodes[a.ID] = na
		nb := g.graph.Nodes[b.ID]
		nb.Params.Volume = 0.6
		if id == headlineChordInst {
			// A chord node: the whole voice stack must survive the round trip.
			nb.Type = model.NodeTypeChord
			nb.Params.Chord = model.ChordSpec{Quality: "m7", Inversion: 1, Spread: model.ChordSpreadDrop2}
		}
		g.graph.Nodes[b.ID] = nb
	}
	g.drum.SetBPM(132)

	for i, id := range ids {
		row := g.drum.Rows[i]

		// Row volume.
		row.Volume = 0.55 + 0.1*float64(i)

		// Sparse synth-knob edits — the shape the Synth tab actually writes.
		owned := audio.GetInstrumentParams(id)
		keys := make([]string, 0, len(owned))
		for k := range owned {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for n, k := range keys {
			if n%23 != 0 {
				continue
			}
			audio.SetInstrumentParam(id, k, owned[k]+0.25+0.1*float64(i))
		}

		// Insert-effect chain, including a disabled slot (which must survive as
		// disabled, not be dropped) and non-default params.
		audio.SetInsertEffects(id, []audio.EffectSlot{
			{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 1.7 + 0.3*float64(i), "mix": 0.45, "tone": 3200}},
			{Type: "tremolo", Enabled: i%2 == 0, Params: map[string]float64{"depth": 0.6, "rate": 4.5, "mix": 1, "shape": 1.2}},
		})

		// Per-instrument EQ: band gains, a muted band, and both filters.
		rowEQ(row).GainsDB = make([]float64, len(eqBandDefs))
		rowEQ(row).GainsDB[1] = -6 + float64(i)
		rowEQ(row).GainsDB[5] = 4 - float64(i)
		rowEQ(row).BandMuted = make([]bool, len(eqBandDefs))
		rowEQ(row).BandMuted[8] = true
		rowEQ(row).HPF = true
		rowEQ(row).HPFHz = 60 + 20*float64(i)
		rowEQ(row).LPF = i%2 == 1
		rowEQ(row).LPFHz = 9000 - 500*float64(i)
		g.drum.applyRowEQ(i)

		// Display name: an instrument renamed in the UI must come back renamed.
		audio.SetInstrumentDisplayName(id, fmt.Sprintf("Custom %d", i))
	}

	// Master: volume and EQ (gains + mute + filters).
	audio.SetMainVolume(0.72)
	copy(g.drum.eqBandGainsDB(), make([]float64, len(eqBandDefs)))
	g.drum.eqBandGainsDB()[3] = -5
	g.drum.eqBandGainsDB()[7] = 3
	g.drum.eqBandMuted()[0] = true
	g.drum.hpfEnabled = true
	g.drum.hpfCutoffHz = 30
	g.drum.lpfEnabled = true
	g.drum.lpfCutoffHz = 16000
	g.drum.applyMasterEQ()

	g.updateBeatInfos()
	return g, ids
}

// ───────────────────────────────── tests ─────────────────────────────────────

// TestMasterMixRenderIsDeterministic pins the premise everything else rests on:
// the same configuration renders to the same bytes. If this fails, no identity
// assertion downstream means anything — a "difference" could just be noise.
func TestMasterMixRenderIsDeterministic(t *testing.T) {
	g, _ := editedProject(t)
	a := renderLiveMasterMix(t, g, masterRenderSeconds)
	b := renderLiveMasterMix(t, g, masterRenderSeconds)
	assertMasterPCMIdentical(t, a, b, "same config rendered twice")

	var peak float64
	for _, v := range a {
		if d := math.Abs(v); d > peak {
			peak = d
		}
	}
	if peak == 0 {
		t.Fatal("master render is silent — the harness is not exercising the mixer")
	}
	t.Logf("%d samples, peak %.4f", len(a), peak)
}

// TestSynthTabSaveDoesNotChangeTheMasterMix: Save and Save As copy the tone the
// user is ALREADY hearing into the library. If either moves a single sample, the
// user's edit and the saved/exported sound are two different things — and the
// export → import comparison would faithfully reproduce the wrong one.
func TestSynthTabSaveDoesNotChangeTheMasterMix(t *testing.T) {
	g, ids := editedProjectUnsaved(t)
	before := renderLiveMasterMix(t, g, masterRenderSeconds)
	applySynthTabSaves(t, g, ids)
	after := renderLiveMasterMix(t, g, masterRenderSeconds)
	assertMasterPCMIdentical(t, before, after, "Synth-tab Save + Save As")
}

// TestHeadlineEditsAreAudible pins the premise of the identity tests below: an
// identity assertion passes just as happily when an edit is IGNORED on both
// sides, so every post-Save edit must actually move the master mix. This is
// what exposed the pitched-voice cache serving the unedited organ after a
// Sampler edit (the edit never reached a single sample, so export → import
// "matched").
func TestHeadlineEditsAreAudible(t *testing.T) {
	g, ids := editedProjectUnsaved(t)
	applySynthTabSaves(t, g, ids)
	saved := renderLiveMasterMix(t, g, masterRenderSeconds)
	applyPostSaveEdits(t, g, ids)
	edited := renderLiveMasterMix(t, g, masterRenderSeconds)
	if _, _, n := pcmIdentical(saved, edited); n == 0 {
		t.Fatal("the post-Save live edits did not change the master mix")
	}
	audio.ClearSampleEdit(ids[3])
	unedited := renderLiveMasterMix(t, g, masterRenderSeconds)
	if _, _, n := pcmIdentical(edited, unedited); n == 0 {
		t.Errorf("the Sampler edit on %q is inaudible in the master mix", ids[3])
	}
	// The macro dial alone must be audible too: turn it once more on top of
	// everything, then put exactly its pre-turn params back.
	beforeDial := turnHeadlineDial(t, g, ids[1])
	withDial := renderLiveMasterMix(t, g, masterRenderSeconds)
	for k, v := range beforeDial {
		audio.SetInstrumentParam(ids[1], k, v)
	}
	withoutDial := renderLiveMasterMix(t, g, masterRenderSeconds)
	if _, _, n := pcmIdentical(withDial, withoutDial); n == 0 {
		t.Errorf("the macro dial %s/%s on %q is inaudible in the master mix", headlineDialCard, headlineDialID, ids[1])
	}
	// The chord node's quality must be audible (an identity test is only as
	// good as the proof that its edits are audible).
	var chordID = model.InvalidNodeID
	for nid, n := range g.graph.Nodes {
		if n.Type == model.NodeTypeChord {
			chordID = nid
		}
	}
	if chordID == model.InvalidNodeID {
		t.Fatal("the headline circuit has no chord node")
	}
	asM7 := renderLiveMasterMix(t, g, masterRenderSeconds)
	n := g.graph.Nodes[chordID]
	p := n.Params
	p.Chord = model.ChordSpec{Quality: "sus4"}
	g.graph.SetNodeParams(chordID, p)
	asSus4 := renderLiveMasterMix(t, g, masterRenderSeconds)
	if _, _, d := pcmIdentical(asM7, asSus4); d == 0 {
		t.Error("changing the chord node's quality is inaudible in the master mix")
	}
}

// TestExportImportMasterMixIsByteIdentical is the headline contract: edit
// everything, export, wipe every global, import, and hear exactly the same
// four seconds of master bus.
//
// Two import targets: the SAME session (the Synth-tab Saves are still in the
// user library) and a CLEAN machine (an empty library — only the file). They fail differently: the first hides a file that relies on the
// session, the second hides a session that disagrees with the file.
//
// A user's Save trumps the document a circuit carries for the same instrument
// on every open (audio.OpenCircuitInstrumentDocs), so in the same session the
// unsaved post-Save edits would come back as the SAVED tone — by design
// (TestSynthSaveTrumpsTheCircuitOnTheNextOpen). The same-session variant
// therefore Saves those edits too before exporting: the import then rebuilds
// the Saved instruments from the LIBRARY, and must still be byte-identical.
func TestExportImportMasterMixIsByteIdentical(t *testing.T) {
	for _, clean := range []bool{false, true} {
		name := "same-session"
		if clean {
			name = "clean-machine"
		}
		t.Run(name, func(t *testing.T) { exportImportMasterMixIdentity(t, clean) })
	}
}

func exportImportMasterMixIdentity(t *testing.T, cleanMachine bool) {
	g, ids := editedProject(t)
	if !cleanMachine {
		resaveSavedInstruments(t, g, ids)
	}

	beforeInst := make(map[instrument.ID]audibleState, len(ids))
	for _, id := range ids {
		beforeInst[id] = captureAudibleState(id)
	}
	beforeMaster := captureMasterState(g)
	beforeNotes, _ := liveArrangement(t, g, masterRenderSeconds)
	before := renderLiveMasterMix(t, g, masterRenderSeconds)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if cleanMachine {
		wipeToCleanMachine(t, ids)
	} else {
		wipeGlobalAudioState(t, ids)
	}

	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Config completeness first: a named field tells you WHAT was dropped,
	// whereas the PCM assertion only tells you that something was.
	for _, id := range ids {
		if d := beforeInst[id].diff(captureAudibleState(id)); len(d) > 0 {
			t.Errorf("instrument %q config not restored by import:\n  - %v", id, d)
		}
	}
	if got := captureMasterState(g2); !reflect_DeepEqualMaster(beforeMaster, got) {
		t.Errorf("master config not restored by import:\n  want %+v\n  got  %+v", beforeMaster, got)
	}

	// The circuit half: same graph must yield the same note schedule.
	afterNotes, _ := liveArrangement(t, g2, masterRenderSeconds)
	assertNoteSchedulesEqual(t, beforeNotes, afterNotes)

	after := renderLiveMasterMix(t, g2, masterRenderSeconds)
	assertMasterPCMIdentical(t, before, after, "export -> wipe -> import")
}

// resaveSavedInstruments presses Save again on the rows applySynthTabSaves
// Saved, so the user library holds their live (post-Save edited) tone.
func resaveSavedInstruments(t *testing.T, g *Game, ids []instrument.ID) {
	t.Helper()
	for _, id := range ids[:2] {
		g.drum.selectAudioChannel(channel.Of(id))
		if got := saveRecipeForInstrument(id); got != id {
			t.Fatalf("re-Save on %q saved %q", id, got)
		}
	}
	g.drum.selectAudioChannel(channel.Master)
}

// TestExportImportMasterMixIsByteIdenticalAcrossReExport guards the fixed
// point: exporting the imported project must produce a file that renders the
// same again. A field that survives one hop but is re-serialized differently
// (a default re-elided, a map re-ordered into a different clamp) decays across
// repeated save/load cycles — this is what catches that. A property of the
// FILE, so both hops land on a clean machine: with the session's Saves in the
// library the first import would (by design) bring the Saved tones back over
// the file's unsaved edits.
func TestExportImportMasterMixIsByteIdenticalAcrossReExport(t *testing.T) {
	g, ids := editedProject(t)
	first, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	before := renderLiveMasterMix(t, g, masterRenderSeconds)

	wipeToCleanMachine(t, ids)
	g2 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g2.CloseForTest)
	g2.Layout(1280, 720)
	if err := g2.Import(first); err != nil {
		t.Fatalf("import 1: %v", err)
	}
	second, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}

	wipeToCleanMachine(t, ids)
	g3 := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g3.CloseForTest)
	g3.Layout(1280, 720)
	if err := g3.Import(second); err != nil {
		t.Fatalf("import 2: %v", err)
	}
	after := renderLiveMasterMix(t, g3, masterRenderSeconds)
	assertMasterPCMIdentical(t, before, after, "export -> import -> export -> import")

	// The second export must equal the first byte-for-byte: a stable fixed
	// point is what makes repeated save/load lossless rather than merely
	// lossy-by-a-little each time.
	if string(first) != string(second) {
		t.Errorf("re-export is not byte-identical to the first export (%d vs %d bytes)", len(first), len(second))
	}
}

// assertNoteSchedulesEqual compares two arrangements event by event.
func assertNoteSchedulesEqual(t *testing.T, want, got []audio.SongRenderNote) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("note schedule: %d notes -> %d notes", len(want), len(got))
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("note %d differs: %+v -> %+v", i, want[i], got[i])
			return
		}
	}
}

// reflect_DeepEqualMaster compares masterState without pulling in reflect for
// one call site; the slices are short and fixed-length.
func reflect_DeepEqualMaster(a, b masterState) bool {
	if a.MainVolume != b.MainVolume || a.BPM != b.BPM || a.Subdiv != b.Subdiv ||
		a.HPFOn != b.HPFOn || a.LPFOn != b.LPFOn || a.HPFHz != b.HPFHz || a.LPFHz != b.LPFHz ||
		len(a.MasterGains) != len(b.MasterGains) || len(a.MasterMuted) != len(b.MasterMuted) {
		return false
	}
	for i := range a.MasterGains {
		if a.MasterGains[i] != b.MasterGains[i] {
			return false
		}
	}
	for i := range a.MasterMuted {
		if a.MasterMuted[i] != b.MasterMuted[i] {
			return false
		}
	}
	return true
}
