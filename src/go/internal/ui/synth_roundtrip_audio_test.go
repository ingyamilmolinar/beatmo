//go:build !test && !js

// Export → fresh import → render reproduces every tweaked stage's audio byte for byte. Real build only: the -tags
// test audio stub produces no samples. Import invalidates the voice cache, so the second render is a genuine
// recompute.
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

// The audio package's golden hash is private; equivalent to bytes.Equal over the float32 image.
func hashFloat32RT(buf []float32) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// For readable failure messages.
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

	// Import sets the BPM from the file; pinned so it cannot change a sample count.
	const roundTripBPM = 120
	audio.SetBPM(roundTripBPM)
	orig, sr := audio.RenderInstrumentOneShot(instrument.ID(instID))
	if len(orig) == 0 {
		t.Fatalf("instID=%s rendered 0 samples (sr=%d)", instID, sr)
	}
	origHash := hashFloat32RT(orig)

	// Otherwise the round trip is trivially identical.
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

// One subtest per stage, from the shared audio.ParityMatrix the cross-platform reference renders too.
func TestSynthRoundtripByteIdentical_ModularPerStage(t *testing.T) {
	withDefaultAudio(t)
	for _, c := range audio.ParityMatrix() {
		c := c
		t.Run(c.Name, func(t *testing.T) {
			synthRoundTrip(t, string(c.InstrumentID), c.Tweak)
		})
	}
}

// Derived from the instrument's own document, so a key it has no module for is never silently skipped.
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
				// The stored, post-sanitize value, stable under a second write.
				return audio.RecipeParams{k: v}
			}
		}
	}
	return nil
}

// Catches per-instrument export/import drift the modular cases would miss.
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
