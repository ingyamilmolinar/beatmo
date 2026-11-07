//go:build !test

package ui

// A project file describes everything audible: render, export, wipe every global, import, render again —
// byte-identical, since every stage is a pure function of config plus a seeded PRNG. The master bus, because a
// per-voice test cannot see channel volume, inserts, EQ or the master soft clip.

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

// Anything added to the engine that affects sound belongs here too.
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

// To a known-wrong state, so anything the file fails to carry is an audible difference, not a survivor.
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

// Plus an empty user library, so the file alone must rebuild the tone.
func wipeToCleanMachine(t *testing.T, ids []instrument.ID) {
	t.Helper()
	if err := audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil); err != nil {
		t.Fatal(err)
	}
	wipeGlobalAudioState(t, ids)
}

// ───────────────────────────── the circuit ───────────────────────────────────

// Everything edited, then Save and Save As, then more unsaved edits, in the order a user works.
func editedProject(t *testing.T) (*Game, []instrument.ID) {
	t.Helper()
	g, ids := editedProjectUnsaved(t)
	applySynthTabSaves(t, g, ids)
	applyPostSaveEdits(t, g, ids)
	g.updateBeatInfos()
	return g, ids
}

// Through the buttons' entry points with each row's channel selected; neither may change a sample. Returns the
// Save-As id.
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
	// The master-EQ edits went through the Master view.
	g.drum.selectAudioChannel(channel.Master)
	return string(userID)
}

// Never saved, yet the export must carry them. Knobs the Synth tab really offers; the Sampler edit is
// process-global, so it is cleared on Cleanup.
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
	// A dial's edits are plain param writes and travel like any knob.
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

// Length turns both ways on a snare.
const headlineDialCard, headlineDialID = "length", "length"

// Through its real input handler; returns the pre-turn params so a caller can undo it.
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

// Through the UI's engine entry points.
func editedProjectUnsaved(t *testing.T) (*Game, []instrument.ID) {
	t.Helper()
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	ids := []instrument.ID{roster.Kick, roster.Snare, roster.Hihat, roster.OrganChurch}
	// A private empty library and the shipped documents, so sibling tests cannot change what is measured.
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

		// A disabled slot must survive as disabled.
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

// Without it, any difference downstream could be noise.
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

// Save copies what the user already hears; a moved sample would make the saved sound differ from the edit.
func TestSynthTabSaveDoesNotChangeTheMasterMix(t *testing.T) {
	g, ids := editedProjectUnsaved(t)
	before := renderLiveMasterMix(t, g, masterRenderSeconds)
	applySynthTabSaves(t, g, ids)
	after := renderLiveMasterMix(t, g, masterRenderSeconds)
	assertMasterPCMIdentical(t, before, after, "Synth-tab Save + Save As")
}

// An identity passes just as well when an edit is ignored on both sides, so every post-Save edit must move the
// mix.
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
	// Then exactly its pre-turn params back.
	beforeDial := turnHeadlineDial(t, g, ids[1])
	withDial := renderLiveMasterMix(t, g, masterRenderSeconds)
	for k, v := range beforeDial {
		audio.SetInstrumentParam(ids[1], k, v)
	}
	withoutDial := renderLiveMasterMix(t, g, masterRenderSeconds)
	if _, _, n := pcmIdentical(withDial, withoutDial); n == 0 {
		t.Errorf("the macro dial %s/%s on %q is inaudible in the master mix", headlineDialCard, headlineDialID, ids[1])
	}
	// The chord quality must be audible too.
	var chordID model.NodeID = model.InvalidNodeID
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

// Into the same session and a clean machine, which fail differently. A Save trumps a circuit's document on open,
// so the same-session variant re-saves its post-Save edits first and must still be byte-identical.
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

	// A named field says WHAT was dropped; the PCM only says that something was.
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

// So the library holds their live tone.
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

// Exporting the import renders the same again: a field re-serialised differently decays over repeated save/load.
// Both hops on a clean machine, since session Saves would bring the saved tones back over unsaved edits.
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

	// A stable fixed point makes repeated save/load lossless.
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

// Short fixed-length slices, no reflect needed.
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
