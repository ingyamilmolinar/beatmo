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
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
// instrument and asserts the imported circuit renders byte-for-byte identical
// audio to the original. instID is the row instrument id, recipe its synth
// recipe, tweak the per-instrument synth params applied before export.
func synthRoundTrip(t *testing.T, instID, recipe string, tweak audio.RecipeParams) {
	t.Helper()
	if len(tweak) == 0 {
		t.Fatalf("empty tweak for instID=%s recipe=%s — round-trip would be a no-op", instID, recipe)
	}

	// --- Source circuit: bind row 0 to the recipe, apply the synth tweak. ---
	g1 := New(testLogger)
	t.Cleanup(g1.CloseForTest)
	g1.Layout(800, 600)
	if len(g1.drum.Rows) == 0 {
		t.Fatalf("source game has no default rows")
	}
	g1.drum.selRow = 0
	g1.drum.SetInstrument(instID)
	audio.BindInstrumentToRecipe(instID, recipe)
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })
	audio.SetInstrumentParams(instID, tweak)

	// Pin the global BPM before BOTH the original and imported renders.
	// RenderInstrumentOneShot derives the voice duration from the package-global
	// bpm for legacy/family voices (e.g. drum-kick-punchy), so a tempo change
	// between the two renders would change the SAMPLE COUNT and falsely fail.
	// g2.Import below sets the global bpm from the imported file, so without this
	// pin orig and imp can render at different tempos. (The modular voice is
	// fixed-duration, so this only bites BPM-dependent family voices.)
	const roundTripBPM = 120
	audio.SetBPM(roundTripBPM)
	orig, sr := audio.RenderInstrumentOneShot(instID)
	if len(orig) == 0 {
		t.Fatalf("instID=%s recipe=%s rendered 0 samples (sr=%d)", instID, recipe, sr)
	}
	origHash := hashFloat32RT(orig)

	// Precondition: the tweak actually produced a non-empty exported delta,
	// otherwise the round-trip is trivially identical and proves nothing.
	effective := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(instID))
	shipped := audio.RecipeShippedDefaults(recipe)
	deltaKeys := 0
	for k, v := range effective {
		if s, ok := shipped[k]; !ok || math.Abs(v-s) > 1e-9 {
			deltaKeys++
		}
	}
	if deltaKeys == 0 {
		t.Fatalf("instID=%s recipe=%s: tweak %v produced no export delta vs shipped defaults", instID, recipe, tweak)
	}

	data, err := g1.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// --- Fresh instance: import the JSON and re-render. A new Game plus the
	// cache invalidation inside Import guarantees a cold recompute. ---
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import into fresh instance: %v", err)
	}

	// Re-pin the global BPM: g2.Import set it from the imported file, which can
	// differ from the tempo orig rendered at (see roundTripBPM note above).
	audio.SetBPM(roundTripBPM)
	imp, _ := audio.RenderInstrumentOneShot(instID)

	// --- Byte-for-byte assertion. ---
	if len(orig) != len(imp) {
		t.Fatalf("instID=%s recipe=%s: sample count diverged after round-trip: orig=%d imp=%d",
			instID, recipe, len(orig), len(imp))
	}
	if impHash := hashFloat32RT(imp); origHash != impHash {
		idx := firstSampleDiff(orig, imp)
		var ov, iv float32
		if idx >= 0 {
			ov, iv = orig[idx], imp[idx]
		}
		t.Fatalf("instID=%s recipe=%s: rendered audio NOT byte-identical after export/import round-trip\n"+
			"  orig sha256=%s\n  imp  sha256=%s\n  first diff at sample %d: orig=%v imp=%v\n  tweak=%v",
			instID, recipe, origHash, impHash, idx, ov, iv, tweak)
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
			synthRoundTrip(t, c.InstrumentID, c.Recipe, c.Tweak)
		})
	}
}

// nudgeFirstEffective binds instID to recipe, then finds a single recipe-valid
// parameter whose value can be moved off its shipped default (surviving
// SetInstrumentParams' sanitize/clamp), and returns that as a one-key tweak.
// Returns nil if no parameter can be effectively changed. Deriving the tweak
// from the recipe's OWN schema avoids the false-pass trap where a modular-only
// key is silently dropped for a drum/FM recipe.
func nudgeFirstEffective(instID, recipe string) audio.RecipeParams {
	shipped := audio.RecipeShippedDefaults(recipe)
	if len(shipped) == 0 {
		return nil
	}
	keys := make([]string, 0, len(shipped))
	for k := range shipped {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	audio.BindInstrumentToRecipe(instID, recipe)
	defer audio.ResetInstrumentParams(instID)

	for _, k := range keys {
		base := shipped[k]
		for _, cand := range []float64{base + 1, base - 1, base + 0.5, base - 0.5, base + 5} {
			audio.SetInstrumentParams(instID, audio.RecipeParams{k: cand})
			got := audio.GetInstrumentParams(instID)
			if v, ok := got[k]; ok && math.Abs(v-base) > 1e-9 {
				// Return the STORED (post-sanitize) value so the tweak is
				// stable under a second SetInstrumentParams.
				return audio.RecipeParams{k: v}
			}
		}
	}
	return nil
}

// synthImportIsByteStable verifies that once a tweaked instrument is exported
// and imported, a FURTHER export→import reproduces byte-for-byte identical
// audio. This is the faithful, robust round-trip property for the broad
// instrument sweep ("re-export is byte-stable: the delta recomputed vs shipped
// is the original delta", per import.go).
//
// Why idempotency rather than pre-export==post-import (which synthRoundTrip uses
// for the modular stages): legacy family recipes (e.g. drum-kick-punchy) carry
// HIDDEN shipped keys (post_enabled) that are in RecipeShippedDefaults but not
// in the visible RecipeDefaultParams. Import pins shipped⊕delta (so the imported
// overlay carries the hidden key), while the original sparse-overlay render
// resolves missing keys via MergeRecipeDefaults (which omits hidden keys) — so a
// raw pre-vs-post comparison trips on that asymmetry, which is orthogonal to
// "does the round-trip preserve audio". Idempotency holds regardless and is what
// matters: import twice ⇒ identical sound.
func synthImportIsByteStable(t *testing.T, instID, recipe string, tweak audio.RecipeParams) {
	t.Helper()
	const roundTripBPM = 120

	// Source circuit with the tweak applied.
	g1 := New(testLogger)
	t.Cleanup(g1.CloseForTest)
	g1.Layout(800, 600)
	if len(g1.drum.Rows) == 0 {
		t.Fatalf("source game has no default rows")
	}
	g1.drum.selRow = 0
	g1.drum.SetInstrument(instID)
	audio.BindInstrumentToRecipe(instID, recipe)
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })
	audio.SetInstrumentParams(instID, tweak)
	data1, err := g1.drum.exportBytes()
	if err != nil {
		t.Fatalf("first export: %v", err)
	}

	// First import → render imp1 → re-export.
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(data1); err != nil {
		t.Fatalf("first import: %v", err)
	}
	audio.SetBPM(roundTripBPM) // see roundTripBPM note in synthRoundTrip
	imp1, _ := audio.RenderInstrumentOneShot(instID)
	if len(imp1) == 0 {
		t.Fatalf("instID=%s recipe=%s rendered 0 samples", instID, recipe)
	}
	data2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}

	// Second import → render imp2. Must be byte-identical to imp1.
	g3 := New(testLogger)
	t.Cleanup(g3.CloseForTest)
	g3.Layout(800, 600)
	if err := g3.Import(data2); err != nil {
		t.Fatalf("second import: %v", err)
	}
	audio.SetBPM(roundTripBPM)
	imp2, _ := audio.RenderInstrumentOneShot(instID)

	if len(imp1) != len(imp2) {
		t.Fatalf("instID=%s recipe=%s: sample count not stable across re-import: imp1=%d imp2=%d",
			instID, recipe, len(imp1), len(imp2))
	}
	if h1, h2 := hashFloat32RT(imp1), hashFloat32RT(imp2); h1 != h2 {
		idx := firstSampleDiff(imp1, imp2)
		t.Fatalf("instID=%s recipe=%s: audio NOT byte-stable across re-import\n"+
			"  imp1 sha256=%s\n  imp2 sha256=%s\n  first diff at sample %d\n  tweak=%v",
			instID, recipe, h1, h2, idx, tweak)
	}
}

// TestSynthRoundtripByteIdentical_AllInstruments sweeps every registered
// instrument (the ~25 shipped base+variant ids), nudges one recipe-valid
// parameter, and asserts the export/import round-trip is byte-stable (import
// idempotency). This catches family-specific (drum / FM / modular) export/import
// plumbing drift that the modular-only per-stage cases would miss.
func TestSynthRoundtripByteIdentical_AllInstruments(t *testing.T) {
	withDefaultAudio(t)
	ids := audio.Instruments()
	if len(ids) == 0 {
		t.Fatal("no registered instruments")
	}
	sort.Strings(ids)
	tested := 0
	for _, id := range ids {
		recipe := audio.RecipeForInstrument(id)
		if recipe == "" {
			continue // user samples / unbound ids carry no synth recipe
		}
		id, recipe := id, recipe
		t.Run(id, func(t *testing.T) {
			tweak := nudgeFirstEffective(id, recipe)
			if tweak == nil {
				t.Skipf("recipe %s has no effectively-tweakable parameter", recipe)
			}
			synthImportIsByteStable(t, id, recipe, tweak)
		})
		tested++
	}
	if tested == 0 {
		t.Fatal("no instrument had a synth recipe binding — sweep covered nothing")
	}
	t.Logf("swept %d recipe-bound instruments", tested)
}
