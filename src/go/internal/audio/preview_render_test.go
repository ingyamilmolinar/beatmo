//go:build !test && !js

package audio

import "testing"

func TestRenderInstrumentPreview_NonEmptyAndDeterministic(t *testing.T) {
	const inst = "kick"
	a := RenderInstrumentPreview(inst, 200)
	if len(a) == 0 {
		t.Fatalf("expected non-empty PCM")
	}
	_, peak := argmaxAbs(a)
	if peak <= 0 {
		t.Fatalf("expected audible peak, got %v", peak)
	}
	b := RenderInstrumentPreview(inst, 200)
	if hashFloat64PCM(a) != hashFloat64PCM(b) {
		t.Fatalf("render must be deterministic for identical params")
	}
}

func TestRenderInstrumentPreview_ReactsToParamChange(t *testing.T) {
	const inst = "kick"
	base := RenderInstrumentPreview(inst, 200)
	old := GetInstrumentParams(inst)["fundamental"]
	SetInstrumentParam(inst, "fundamental", 120)
	t.Cleanup(func() { SetInstrumentParam(inst, "fundamental", old) })
	changed := RenderInstrumentPreview(inst, 200)
	if hashFloat64PCM(base) == hashFloat64PCM(changed) {
		t.Fatalf("changing a param must change the rendered preview")
	}
}
