//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestCloneInstrument_ConfigFirst clones a built-in instrument purely by config
// (no code): the clone renders, an override changes its sound, and DeleteInstrument
// removes it without disturbing the source or its recipe.
func TestCloneInstrument_ConfigFirst(t *testing.T) {
	t.Cleanup(ResetInstruments)

	const src = "zgump-kick"
	if RecipeForInstrument(src) == "" {
		t.Fatalf("precondition: %q must be recipe-bound", src)
	}

	// Exact clone (no overrides) renders identically to the source.
	if _, err := CloneInstrument(src, "clone-exact", "Exact Clone", nil); err != nil {
		t.Fatalf("CloneInstrument exact: %v", err)
	}
	if RecipeForInstrument("clone-exact") == "" || !instanceAlreadyRegistered("clone-exact") {
		t.Fatal("exact clone not registered/bound")
	}
	srcBuf, _ := RenderInstrumentOneShotRaw(src)
	exactBuf, _ := RenderInstrumentOneShotRaw("clone-exact")
	if !buffersClose(srcBuf, exactBuf) {
		t.Fatal("exact clone should render identically to the source")
	}

	// Clone with an override changes the sound (lower fundamental → lower dominant).
	if _, err := CloneInstrument(src, "clone-low", "Low Clone", RecipeParams{"voice_freq_hz": 45}); err != nil {
		t.Fatalf("CloneInstrument override: %v", err)
	}
	lowBuf, sr := RenderInstrumentOneShotRaw("clone-low")
	if buffersClose(srcBuf, lowBuf) {
		t.Fatal("override clone should differ from the source")
	}
	grid := []float64{40, 45, 55, 65, 78, 90}
	if dom := argmaxMagAt(f32toF64(lowBuf), grid, sr); dom > 65 {
		t.Fatalf("override (voice_freq 45) should lower the fundamental, got dominant=%.0f Hz", dom)
	}

	// Duplicate id is rejected; unknown source is rejected.
	if _, err := CloneInstrument(src, "clone-exact", "", nil); err == nil {
		t.Fatal("cloning onto an existing id must error")
	}
	if _, err := CloneInstrument("no-such-instrument", "clone-x", "", nil); err == nil {
		t.Fatal("cloning an unbound source must error")
	}

	// Delete removes the clone (voice + binding) but leaves the source intact.
	if err := DeleteInstrument("clone-low"); err != nil {
		t.Fatalf("DeleteInstrument: %v", err)
	}
	if instanceAlreadyRegistered("clone-low") || RecipeForInstrument("clone-low") != "" {
		t.Fatal("deleted clone still registered/bound")
	}
	if RecipeForInstrument(src) == "" {
		t.Fatal("source recipe binding must survive deleting a clone")
	}
	if b, _ := RenderInstrumentOneShotRaw(src); len(b) == 0 {
		t.Fatal("source must still render after a clone was deleted")
	}
	if err := DeleteInstrument("no-such-instrument"); err == nil {
		t.Fatal("deleting an unknown instrument must error")
	}
}

func buffersClose(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-6 {
			return false
		}
	}
	return true
}
