//go:build !test && !js

// Byte-for-byte export/import synth-audio round-trip suite.
//
// Goal: prove that exporting a circuit to JSON and re-importing it reproduces
// the EXACT same rendered audio (byte-for-byte), even after the user has tweaked
// synth parameters on every modular-synth stage. This is both a regression
// harness and a bug finder: if the export delta math (export.go) or the import
// reconstruction (import.go) drops, clamps, or mis-resolves a synth parameter,
// the rendered PCM diverges and the offending stage's subtest fails.
//
// Why this build tag: byte-for-byte synth audio needs the REAL C DSP renderer,
// which only compiles under `!test && !js`. The `-tags test` fast path links an
// audio stub that produces no real samples. This suite therefore runs on the
// real-Ebiten path: `make test-real` / `xvfb-run -a go test ./internal/ui/...`.
//
// What it actually exercises (true end-to-end, not a copy of the logic):
//
//	set per-instrument synth params
//	  -> (*DrumView).exportBytes()        // real export.go delta-vs-shipped
//	    -> fresh Game.Import(json)         // real import.go shipped⊕delta pin
//	      -> audio.RenderInstrumentOneShot // real render_modular_p (C DSP)
//	compare SHA256(LE float32) of original vs imported render.
//
// Anti-false-pass: Import calls audio.SetInstrumentParams, which UNCONDITIONALLY
// invalidates that instrument's voice cache (synth_recipe.go), so the imported
// render is a genuine recompute and cannot return the original cached buffer.
package ui

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// hashFloat32RT mirrors internal/audio's golden-test hash (SHA256 over
// little-endian float32). Re-declared here because the audio helper is
// package-private. Equivalent to bytes.Equal over the float32 byte image.
func hashFloat32RT(buf []float32) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// firstSampleDiff returns the index of the first differing sample (-1 if equal
// up to the shorter length). Used purely for debuggable failure messages.
func firstSampleDiff(a, b []float32) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return -1
}

// synthRoundTrip performs the full export→import→render round-trip for one
// instrument with one synth tweak and asserts the rendered audio is
// byte-for-byte identical.
func synthRoundTrip(t *testing.T, instID string, tweak audio.RecipeParams) {
	t.Helper()
	if len(tweak) == 0 {
		t.Fatalf("empty tweak for instID=%s — round-trip would be a no-op", instID)
	}

	// --- Source circuit: row 0 plays instID with the synth tweak applied. ---
	g1 := New(testLogger)
	t.Cleanup(g1.CloseForTest)
	g1.Layout(800, 600)
	if len(g1.drum.Rows) == 0 {
		t.Fatalf("source game has no default rows")
	}
	g1.drum.selRow = 0
	g1.drum.SetInstrument(instrument.ID(instID))
	t.Cleanup(func() { audio.ResetInstrumentParams(instrument.ID(instID)) })
	audio.SetInstrumentParams(instrument.ID(instID), tweak)

	// Pin the global BPM before BOTH renders: g2.Import below sets it from the
	// file, and a BPM-sized voice would otherwise change its SAMPLE COUNT.
	const roundTripBPM = 120
	audio.SetBPM(roundTripBPM)
	orig, sr := audio.RenderInstrumentOneShot(instrument.ID(instID))
	if len(orig) == 0 {
		t.Fatalf("instID=%s rendered 0 samples (sr=%d)", instID, sr)
	}
	origHash := hashFloat32RT(orig)

	// Precondition: the tweak actually changed the document, otherwise the
	// round-trip is trivially identical and proves nothing.
	if !audio.InstrumentDocModified(instrument.ID(instID)) {
		t.Fatalf("instID=%s: tweak %v did not change its document", instID, tweak)
	}

	data, err := g1.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// --- Fresh instance: import the JSON and re-render. ---
	audio.ResetInstrumentParams(instrument.ID(instID)) // the file alone must bring the tweak back
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import into fresh instance: %v", err)
	}
	audio.SetBPM(roundTripBPM)
	imp, _ := audio.RenderInstrumentOneShot(instrument.ID(instID))

	// --- Byte-for-byte assertion. ---
	if len(orig) != len(imp) {
		t.Fatalf("instID=%s: sample count diverged after round-trip: orig=%d imp=%d", instID, len(orig), len(imp))
	}
	if impHash := hashFloat32RT(imp); origHash != impHash {
		idx := firstSampleDiff(orig, imp)
		var ov, iv float32
		if idx >= 0 {
			ov, iv = orig[idx], imp[idx]
		}
		t.Fatalf("instID=%s: rendered audio NOT byte-identical after export/import round-trip\n"+
			"  orig sha256=%s\n  imp  sha256=%s\n  first diff at sample %d: orig=%v imp=%v\n  tweak=%v",
			instID, origHash, impHash, idx, ov, iv, tweak)
	}
}

// TestSynthRoundtripByteIdentical_ModularPerStage tweaks one parameter on every
// modular synth stage, exports → imports → re-renders, and asserts the audio is
// byte-for-byte identical. One subtest per stage isolates which stage's
// export/import path drifts if the suite fails.
//
// The stage matrix is the SHARED audio.ParityMatrix() — the same cases the
// cross-platform desktop reference (cmd/export_audio) renders, so the Go
// export/import suite and the browser parity suite exercise identical edits
// (includes the combined "stage-ALL" case).
func TestSynthRoundtripByteIdentical_ModularPerStage(t *testing.T) {
	withDefaultAudio(t)
	for _, c := range audio.ParityMatrix() {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			synthRoundTrip(t, string(c.InstrumentID), c.Tweak)
		})
	}
}

// nudgeFirstEffective finds a single value of instID's document that can be
// moved (surviving SetInstrumentParams' sanitize/clamp) and returns it as a
// one-key tweak, or nil. Deriving the tweak from the instrument's OWN document
// avoids the false-pass trap where a key the instrument has no module for is
// silently skipped.
func nudgeFirstEffective(instID string) audio.RecipeParams {
	owned := audio.GetInstrumentParams(instrument.ID(instID))
	keys := make([]string, 0, len(owned))
	for k := range owned {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	defer audio.ResetInstrumentParams(instrument.ID(instID))
	for _, k := range keys {
		base := owned[k]
		for _, cand := range []float64{base + 1, base - 1, base + 0.5, base - 0.5, base + 5} {
			audio.SetInstrumentParams(instrument.ID(instID), audio.RecipeParams{k: cand})
			if v, ok := audio.GetInstrumentParams(instrument.ID(instID))[k]; ok && math.Abs(v-base) > 1e-9 {
				// Return the STORED (post-sanitize) value so the tweak is
				// stable under a second SetInstrumentParams.
				return audio.RecipeParams{k: v}
			}
		}
	}
	return nil
}

// TestSynthRoundtripByteIdentical_AllInstruments sweeps every registered
// instrument with a document, nudges one of its values, and asserts the
// export/import round-trip is byte-identical. This catches instrument-specific
// export/import plumbing drift the modular-only per-stage cases would miss.
func TestSynthRoundtripByteIdentical_AllInstruments(t *testing.T) {
	withDefaultAudio(t)
	ids := audio.Instruments()
	if len(ids) == 0 {
		t.Fatal("no registered instruments")
	}
	slices.Sort(ids)
	tested := 0
	for _, id := range ids {
		if !audio.HasInstrumentDoc(id) {
			continue // user samples carry no instrument document
		}
		id := id
		t.Run(string(id), func(t *testing.T) {
			tweak := nudgeFirstEffective(string(id))
			if tweak == nil {
				t.Skipf("%s has no effectively-tweakable value", id)
			}
			synthRoundTrip(t, string(id), tweak)
		})
		tested++
	}
	if tested == 0 {
		t.Fatal("no instrument had a document — sweep covered nothing")
	}
	t.Logf("swept %d instruments", tested)
}
