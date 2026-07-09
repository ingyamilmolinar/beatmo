//go:build test

package audio

import "testing"

// The override form must (a) equal the base form when override is nil and
// (b) change the render when an override pins a param the preview reads.
func TestRenderInstrumentPreviewWithOverrides_NilMatchesBase(t *testing.T) {
	const inst = "preview-override-test"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	base := RenderInstrumentPreview(inst, 120)
	viaNil := RenderInstrumentPreviewWithOverrides(inst, nil, 120)
	if previewHash(base) != previewHash(viaNil) {
		t.Fatalf("nil-override render differs from RenderInstrumentPreview")
	}
	if len(base) == 0 {
		t.Fatalf("empty preview render")
	}
}

func TestRenderInstrumentPreviewWithOverrides_OverrideChangesOutput(t *testing.T) {
	const inst = "preview-override-test-2"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	base := RenderInstrumentPreview(inst, 120)
	shifted := RenderInstrumentPreviewWithOverrides(inst, map[string]float64{"fundamental": 550}, 120)
	if previewHash(base) == previewHash(shifted) {
		t.Fatalf("fundamental override did not change the preview render")
	}
	// The override must NOT leak into instrument state.
	after := RenderInstrumentPreview(inst, 120)
	if previewHash(base) != previewHash(after) {
		t.Fatalf("override mutated persistent instrument params")
	}
}
