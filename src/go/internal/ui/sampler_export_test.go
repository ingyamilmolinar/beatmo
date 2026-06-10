//go:build test

package ui

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestExportImportSampleEditRoundTrip — a recipe-bound instrument with a
// non-destructive sample-edit descriptor exports a `sample_edit` field
// (effective-delta style, like synth_params) and importing the project
// restores the descriptor, so the saved chop travels inside beatmo.json
// while the synth stays the source of truth.
func TestExportImportSampleEditRoundTrip(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("test game has no rows")
	}
	inst := dv.Rows[0].Instrument
	origEdit, hadEdit := audio.SampleEditFor(inst)
	t.Cleanup(func() {
		if hadEdit {
			audio.SetSampleEdit(inst, origEdit)
		} else {
			audio.ClearSampleEdit(inst)
		}
	})
	want := audio.SampleEdit{StartFrac: 0.25, EndFrac: 0.75, GainDB: -6, Reverse: true}
	audio.SetSampleEdit(inst, want)

	data, err := dv.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	var found *exportInstrument
	for i := range f.Instruments {
		if f.Instruments[i].ID == inst {
			found = &f.Instruments[i]
		}
	}
	if found == nil {
		t.Fatalf("instrument %q missing from export", inst)
	}
	if len(found.SampleEdit) == 0 {
		t.Fatal("export omitted sample_edit for a descriptor-bearing instrument")
	}
	if got := audio.SampleEditFromFields(found.SampleEdit); got != want {
		t.Fatalf("exported sample_edit decodes to %+v, want %+v", got, want)
	}

	// Import restores the descriptor (cleared in between to prove it).
	audio.ClearSampleEdit(inst)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, ok := audio.SampleEditFor(inst)
	if !ok || got != want {
		t.Fatalf("after import descriptor = %+v (ok=%v), want %+v", got, ok, want)
	}
}

// TestImportClearsStaleSampleEdit — importing a project WITHOUT a sample_edit
// for an instrument clears any leftover descriptor from the previous project
// (parity with the SynthParams bulk-replace semantics).
func TestImportClearsStaleSampleEdit(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum
	inst := dv.Rows[0].Instrument
	t.Cleanup(func() { audio.ClearSampleEdit(inst) })

	data, err := dv.exportBytes() // exported with NO descriptor
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	audio.SetSampleEdit(inst, audio.SampleEdit{StartFrac: 0.4, EndFrac: 1})
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if audio.HasSampleEdit(inst) {
		t.Error("import must clear a stale descriptor the project does not carry")
	}
}

func TestSamplePCMCodecRoundTripsBitExact(t *testing.T) {
	rec := audio.SampleRecord{PCM: []float32{0, 0.25, -0.5, 0.123456, 1.0, -1.0}, SampleRate: 48000}
	pcm, sr, ok := decodeSamplePCM(encodeSamplePCM(rec))
	if !ok {
		t.Fatal("decode failed")
	}
	if sr != 48000 || len(pcm) != len(rec.PCM) {
		t.Fatalf("decoded sr=%d len=%d, want 48000 / %d", sr, len(pcm), len(rec.PCM))
	}
	for i := range rec.PCM {
		if pcm[i] != rec.PCM[i] {
			t.Errorf("sample %d: got %v want %v (must be bit-exact)", i, pcm[i], rec.PCM[i])
		}
	}
}

func TestEncodeSamplePCMIsGzipped(t *testing.T) {
	// A zero-filled buffer compresses dramatically; the payload must carry the
	// gzip magic header (0x1f 0x8b) and be smaller than the raw LE bytes so
	// embedded samples don't bloat beatmo.json (plan risk #5).
	rec := audio.SampleRecord{PCM: make([]float32, 4096), SampleRate: 48000}
	enc := encodeSamplePCM(rec)
	raw, err := base64.StdEncoding.DecodeString(enc.DataB64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		t.Fatalf("payload not gzipped (first bytes %v)", raw[:min(2, len(raw))])
	}
	if len(raw) >= len(rec.PCM)*4 {
		t.Errorf("gzip did not shrink compressible buffer: %d >= %d", len(raw), len(rec.PCM)*4)
	}
}

func TestDecodeSamplePCMAcceptsLegacyUncompressed(t *testing.T) {
	// Defensive: a pre-gzip payload (raw LE float32 bytes, no gzip wrapper)
	// must still decode, so a hand-written or older payload isn't rejected.
	want := []float32{0.1, -0.2, 0.3}
	buf := make([]byte, len(want)*4)
	for i, v := range want {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	legacy := &exportSamplePCM{SampleRate: 44100, Frames: len(want), DataB64: base64.StdEncoding.EncodeToString(buf)}
	pcm, sr, ok := decodeSamplePCM(legacy)
	if !ok || sr != 44100 || len(pcm) != len(want) {
		t.Fatalf("legacy decode: ok=%v sr=%d len=%d", ok, sr, len(pcm))
	}
	for i := range want {
		if pcm[i] != want[i] {
			t.Errorf("legacy sample %d = %v, want %v", i, pcm[i], want[i])
		}
	}
}

func TestDecodeSamplePCMRejectsGarbage(t *testing.T) {
	if _, _, ok := decodeSamplePCM(nil); ok {
		t.Error("nil PCM should decode ok=false")
	}
	if _, _, ok := decodeSamplePCM(&exportSamplePCM{Frames: 0, DataB64: ""}); ok {
		t.Error("empty PCM should decode ok=false")
	}
	if _, _, ok := decodeSamplePCM(&exportSamplePCM{Frames: 100, DataB64: "not-base64!!"}); ok {
		t.Error("malformed base64 should decode ok=false")
	}
}

func TestExportEmbedsUserSamplePCM(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	id := "user.sample.export"
	want := []float32{0.1, -0.2, 0.3, -0.4}
	audio.PutUserSample(id, want, 44100)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.drum.Rows[0].Instrument = id

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if f.Version != 1 {
		t.Errorf("export version = %d, want 1 (embedded PCM must stay additive)", f.Version)
	}
	var found *exportInstrument
	for i := range f.Instruments {
		if f.Instruments[i].ID == id {
			found = &f.Instruments[i]
		}
	}
	if found == nil {
		t.Fatalf("instrument %q not in export", id)
	}
	if found.Kind != "sample" {
		t.Errorf("kind = %q, want sample", found.Kind)
	}
	if found.Path != "" {
		t.Errorf("embedded sample must not carry a path, got %q", found.Path)
	}
	if found.PCM == nil {
		t.Fatal("embedded PCM missing from export")
	}
	pcm, sr, ok := decodeSamplePCM(found.PCM)
	if !ok || sr != 44100 || len(pcm) != len(want) {
		t.Fatalf("exported PCM decode: ok=%v sr=%d len=%d", ok, sr, len(pcm))
	}
	for i := range want {
		if pcm[i] != want[i] {
			t.Errorf("exported sample %d = %v, want %v", i, pcm[i], want[i])
		}
	}
}

func TestImportRegistersEmbeddedSamplePCM(t *testing.T) {
	assertDefaultParityState(t)
	id := "user.sample.import.unique"
	want := []float32{0.9, -0.8, 0.7}
	file := exportFile{
		Version: 1, Subdiv: 32, BPM: 120,
		Instruments: []exportInstrument{{
			Name: "Chop", ID: id, Kind: "sample", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			PCM: encodeSamplePCM(audio.SampleRecord{PCM: want, SampleRate: 44100}),
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !audio.IsUserSample(id) {
		t.Fatalf("import did not register embedded sample %q", id)
	}
	rec, ok := audio.UserSamplePCM(id)
	if !ok || len(rec.PCM) != len(want) || rec.SampleRate != 44100 {
		t.Fatalf("imported record wrong: ok=%v len=%d sr=%d", ok, len(rec.PCM), rec.SampleRate)
	}
	for i := range want {
		if rec.PCM[i] != want[i] {
			t.Errorf("imported sample %d = %v, want %v", i, rec.PCM[i], want[i])
		}
	}
}
