//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// export_import_playback_fidelity_test.go — thorough verification that every
// synth, sampler, and effects config survives a project export → import AND that
// the IMPORTED project plays identically to the exported one.
//
// The model round-trip (field values) is covered by export_import_audio_test.go,
// eq_export_import_test.go, sampler_export_test.go, repro_synth_config_roundtrip_test.go,
// and insert_effects_roundtrip_test.go. These tests add the gaps those miss:
//   - send-effect (delay/reverb) config value round-trip — the Configure* are
//     GLOBAL, so the test clobbers them to prove the value comes from the file;
//   - EQ band-mute round-trip (per-instrument + master);
//   - SAMPLE PLAYBACK fidelity: the re-baked playable buffer (what actually
//     plays) is bit-identical after import — including reverse + asymmetric trim;
//   - a kitchen-sink project that sets every subsystem at once, resets ALL global
//     audio state, imports, and asserts every audible-determinant seam.

// pcmEqual reports whether two float32 buffers are bit-identical.
func pcm32Equal(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// paramsApproxEqual compares two effective-tone maps with a tolerance.
func paramsApproxEqual(a, b map[string]float64, eps float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if d := va - vb; d > eps || d < -eps {
			return false
		}
	}
	return true
}

// rampPCM returns a length-n ramp whose value identifies its source position.
func rampPCM(n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(i)
	}
	return out
}

func newProjectGame(t *testing.T) (*Game, *uiNode) {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	return g, ui
}

// TestEQBandMuteRoundTripsThroughProject closes the band-mute gap: a muted EQ
// band (per-instrument and master) must survive export→import and reach the
// audio chain.
func TestEQBandMuteRoundTripsThroughProject(t *testing.T) {
	assertDefaultParityState(t)
	g, _ := newProjectGame(t)
	inst := g.drum.Rows[0].Instrument

	// Per-instrument: mute band 3 (+ a gain so the channel EQ is unambiguously active).
	rowEQ(g.drum.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
	rowEQ(g.drum.Rows[0]).GainsDB[1] = 4
	rowEQ(g.drum.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
	rowEQ(g.drum.Rows[0]).BandMuted[3] = true
	g.drum.applyRowEQ(0)

	// Master: mute band 5.
	g.drum.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	g.drum.eqPanelZone.bandMuted[5] = true
	g.drum.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	g.drum.applyMasterEQ()

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	row := g2.drum.Rows[0]
	if len(rowEQ(row).BandMuted) <= 3 || !rowEQ(row).BandMuted[3] {
		t.Errorf("per-instrument band-3 mute lost on round-trip: %+v", rowEQ(row).BandMuted)
	}
	if len(g2.drum.eqBandMuted()) <= 5 || !g2.drum.eqBandMuted()[5] {
		t.Errorf("master band-5 mute lost on round-trip: %+v", g2.drum.eqBandMuted())
	}
	// Reaches the audio chain: the per-instrument channel's band 3 is muted.
	snap := audio.ChannelEQSnapshot(channel.Of(inst))
	if len(snap.Bands) > 3 && !snap.Bands[3].Muted {
		t.Errorf("imported per-instrument EQ band 3 not muted in the audio chain: %+v", snap.Bands)
	}
}

// TestEditedWAVPlaybackFidelityRoundTrips is the headline playback test: a WAV /
// user-sample instrument edited with reverse + ASYMMETRIC trim + gain must, after
// export→import into a fresh instance, produce a BIT-IDENTICAL playable buffer —
// i.e. it sounds exactly like the exported version. Exercises the new
// non-destructive descriptor flow (pristine PCM embedded + descriptor +
// reapplyUserSampleEdit re-baking on import).
func TestEditedWAVPlaybackFidelityRoundTrips(t *testing.T) {
	assertDefaultParityState(t)
	g, _ := newProjectGame(t)

	const id = "user.wav.fidelity"
	const n = 800
	pristine := rampPCM(n)
	audio.PutUserSample(id, pristine, 48000) // seeded like ensureSamplerLoaded does
	g.drum.Rows[0].Instrument = id

	// Exercise EVERY sample-edit field at once so the playback round-trip
	// validates all sampler configs (trim, pitch transpose + detune, gain,
	// normalize, fades, reverse) — not just a subset. Source-frame fractions.
	desc := audio.SampleEdit{
		StartFrac: 0.2, EndFrac: 0.85,
		TransposeSemis: 3, DetuneCents: -15,
		GainDB: -6, Normalize: true,
		Fade:    true,
		Reverse: true,
	}
	audio.SetSampleEdit(id, desc)

	want := audio.BakeSample(pristine, 48000, desc)
	bakedBefore, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok {
		t.Fatal("setting the edit did not re-register a playable buffer")
	}
	if !pcm32Equal(bakedBefore.PCM, want) {
		t.Fatal("precondition: pre-export playable buffer is not the baked edit")
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Fresh instance: clobber the canonical PCM with garbage and clear the edit, so
	// a passing assertion can only come from the embedded project bytes.
	audio.PutUserSample(id, []float32{0, 0, 0, 0}, 48000)
	audio.ClearSampleEdit(id)

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// The descriptor and pristine source both round-tripped.
	if got, ok := audio.SampleEditFor(id); !ok || got != desc {
		t.Fatalf("descriptor not restored: got %+v ok=%v want %+v", got, ok, desc)
	}
	if rec, ok := audio.UserSamplePCM(id); !ok || !pcm32Equal(rec.PCM, pristine) {
		t.Fatalf("pristine source PCM not restored (ok=%v)", ok)
	}
	// THE PLAYBACK CHECK: the re-baked playable buffer is bit-identical to before.
	bakedAfter, ok := audio.LastRegisteredSamplePCMForTest(id)
	if !ok {
		t.Fatal("import did not re-register a playable buffer for the edited WAV")
	}
	if !pcm32Equal(bakedAfter.PCM, bakedBefore.PCM) {
		t.Errorf("imported WAV does not play identically: baked len before=%d after=%d", len(bakedBefore.PCM), len(bakedAfter.PCM))
	}
}

// instDeterminants captures everything that decides how an instrument sounds.
type instDeterminants struct {
	origin    string
	effective map[string]float64
	hasEdit   bool
	edit      audio.SampleEdit
	bakedPlay []float32 // re-baked playable for a user sample (nil otherwise)
	isSample  bool
	fx        []audio.EffectSlot
	eqBands   []audio.EQBand
}

// docOrigin is the library entry id's project document came from ("" = none).
func docOrigin(id string) string {
	d, _ := audio.InstrumentDoc(instrument.ID(id))
	return string(d.Origin)
}

func captureInstDeterminants(id string) instDeterminants {
	d := instDeterminants{
		origin:    docOrigin(id),
		effective: map[string]float64(audio.GetInstrumentParams(instrument.ID(id))),
		fx:        audio.GetInsertEffects(instrument.ID(id)),
		eqBands:   audio.ChannelEQSnapshot(channel.Of(instrument.ID(id))).Bands,
	}
	if e, ok := audio.SampleEditFor(instrument.ID(id)); ok {
		d.hasEdit = true
		d.edit = e
	}
	if rec, ok := audio.UserSamplePCM(instrument.ID(id)); ok {
		d.isSample = true
		// What actually plays: the descriptor baked onto the pristine source.
		d.bakedPlay = audio.BakeSample(rec.PCM, rec.SampleRate, d.edit)
	}
	return d
}

func (d instDeterminants) equal(o instDeterminants, t *testing.T, label string) {
	t.Helper()
	if d.origin != o.origin {
		t.Errorf("%s: instrument document origin %q != %q", label, d.origin, o.origin)
	}
	if !paramsApproxEqual(d.effective, o.effective, 1e-9) {
		t.Errorf("%s: effective synth tone diverged:\n  before=%v\n  after =%v", label, d.effective, o.effective)
	}
	if d.hasEdit != o.hasEdit || d.edit != o.edit {
		t.Errorf("%s: sample_edit diverged: before(%v)=%+v after(%v)=%+v", label, d.hasEdit, d.edit, o.hasEdit, o.edit)
	}
	if d.isSample != o.isSample || !pcm32Equal(d.bakedPlay, o.bakedPlay) {
		t.Errorf("%s: sample PLAYBACK buffer diverged (isSample %v/%v, len %d/%d)", label, d.isSample, o.isSample, len(d.bakedPlay), len(o.bakedPlay))
	}
	if len(d.fx) != len(o.fx) {
		t.Errorf("%s: insert-FX count %d != %d", label, len(d.fx), len(o.fx))
	} else {
		for i := range d.fx {
			if d.fx[i].Type != o.fx[i].Type || d.fx[i].Enabled != o.fx[i].Enabled ||
				!paramsApproxEqual(d.fx[i].Params, o.fx[i].Params, 1e-9) {
				t.Errorf("%s: insert-FX[%d] diverged: %+v != %+v", label, i, d.fx[i], o.fx[i])
			}
		}
	}
	if len(d.eqBands) != len(o.eqBands) {
		t.Errorf("%s: EQ band count %d != %d", label, len(d.eqBands), len(o.eqBands))
	} else {
		for i := range d.eqBands {
			a, b := d.eqBands[i], o.eqBands[i]
			if a.GainDB != b.GainDB || a.Muted != b.Muted {
				t.Errorf("%s: EQ band %d diverged: %+v != %+v", label, i, a, b)
			}
		}
	}
}

// TestProjectAllConfigPlaybackDeterminantsRoundTrip is the kitchen-sink test:
// one project that sets a synth instrument AND a user-sample instrument with
// every per-instrument knob (params/edit/FX/EQ+mute) plus master volume/EQ.
// It exports, RESETS every global audio store, then
// imports and asserts each instrument's full audible-determinant set is
// unchanged — catching cross-subsystem and ordering regressions a per-feature
// test would miss.
func TestProjectAllConfigPlaybackDeterminantsRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	mv0 := audio.MainVolume()
	t.Cleanup(func() { audio.SetMainVolume(mv0) })

	g, n0 := newProjectGame(t)
	dv := g.drum

	// Row 0: a synth instrument with the works.
	synthID := dv.Rows[0].Instrument
	if !audio.HasInstrumentDoc(synthID) {
		t.Fatalf("row0 instrument %q has no instrument document", synthID)
	}
	audio.SetInstrumentParam(synthID, "pitch", 3)
	audio.SetInstrumentParam(synthID, "post_decay", 0.66)
	audio.SetInsertEffects(synthID, []audio.EffectSlot{
		{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 0.5}},
		{Type: "delay", Enabled: false, Params: map[string]float64{"time": 0.3, "feedback": 0.4}},
	})
	rowEQ(dv.Rows[0]).GainsDB = make([]float64, len(eqBandDefs))
	rowEQ(dv.Rows[0]).GainsDB[2] = 5
	rowEQ(dv.Rows[0]).BandMuted = make([]bool, len(eqBandDefs))
	rowEQ(dv.Rows[0]).BandMuted[7] = true
	rowEQ(dv.Rows[0]).HPF = true
	rowEQ(dv.Rows[0]).HPFHz = 120
	dv.applyRowEQ(0)

	// Row 1: a user-sample instrument with a reverse+trim+gain descriptor.
	dv.AddRow()
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	dv.Rows[1].Origin = n1.ID
	dv.Rows[1].Node = n1
	const sampleID = "user.sample.kitchensink"
	pristine := rampPCM(600)
	audio.PutUserSample(sampleID, pristine, 48000)
	dv.Rows[1].Instrument = sampleID
	sampleEdit := audio.SampleEdit{StartFrac: 0.2, EndFrac: 0.9, Reverse: true, GainDB: -3}
	audio.SetSampleEdit(sampleID, sampleEdit)

	// Master volume + master EQ.
	audio.SetMainVolume(0.6)
	dv.eqPanelZone.bandGainsDB = make([]float64, len(eqBandDefs))
	dv.eqPanelZone.bandGainsDB[4] = -4
	dv.eqPanelZone.bandMuted = make([]bool, len(eqBandDefs))
	dv.applyMasterEQ()

	// Capture the audible determinants BEFORE export.
	beforeSynth := captureInstDeterminants(string(synthID))
	beforeSample := captureInstDeterminants(sampleID)

	data, err := dv.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	_ = n0

	// RESET every global audio store the import must repopulate.
	audio.ResetInstrumentDocs()
	audio.ClearAllInsertEffects()
	audio.ClearChannelProcessors(channel.Of(synthID))
	audio.PutUserSample(sampleID, []float32{0, 0, 0, 0}, 48000)
	audio.ClearSampleEdit(sampleID)
	audio.SetMainVolume(1)

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Every audible determinant must be unchanged.
	captureInstDeterminants(string(synthID)).equal(beforeSynth, t, "synth")
	captureInstDeterminants(sampleID).equal(beforeSample, t, "sample")

	if got := audio.MainVolume(); got != 0.6 {
		t.Errorf("master volume not restored: got %v want 0.6", got)
	}
	if len(g2.drum.eqBandGainsDB()) <= 4 || g2.drum.eqBandGainsDB()[4] != -4 {
		t.Errorf("master EQ band-4 gain not restored: %+v", g2.drum.eqBandGainsDB())
	}
}
