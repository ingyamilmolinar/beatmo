package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestFXPanelEmptyChainHasNoParamRows(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument

	// Chain is empty (newTestDV clears all insert effects).
	if len(audio.GetInsertEffects(instID)) != 0 {
		t.Fatalf("expected empty chain, got %d effects", len(audio.GetInsertEffects(instID)))
	}

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders for empty chain, got %d", len(dv.fxPanelSliders))
	}
	if len(dv.fxSliderBindings) != 0 {
		t.Errorf("expected 0 slider bindings for empty chain, got %d", len(dv.fxSliderBindings))
	}
}

func TestFXPanelParamRowsClearedAfterRemoval(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument

	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("expected sliders for the distortion effect on desktop")
	}

	// Remove via the audio API + rebuild (mirrors the remove button's OnClick).
	audio.RemoveInsertEffect(instID, 0)
	dv.buildFXPanel()

	if got := len(audio.GetInsertEffects(instID)); got != 0 {
		t.Fatalf("expected empty chain after removal, got %d", got)
	}
	if len(dv.fxPanelSliders) != 0 {
		t.Errorf("expected 0 sliders after removing the only effect, got %d", len(dv.fxPanelSliders))
	}
	if len(dv.fxSliderBindings) != 0 {
		t.Errorf("expected 0 slider bindings after removal, got %d", len(dv.fxSliderBindings))
	}
}

// No binding may come from a previous (distortion) effect's params.
func TestFXPanelReverbOnlyDerivesFromLiveChain(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument

	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	effects := audio.GetInsertEffects(instID)
	if len(effects) != 1 || effects[0].Type != audio.EffectReverb {
		t.Fatalf("expected single reverb effect, got %+v", effects)
	}

	// The display name must be the reverb registration's name.
	if got := effectTypeName(audio.EffectReverb); got != "Reverb" {
		t.Errorf("expected effect name 'Reverb', got %q", got)
	}

	reverbParams := map[string]bool{"room": true, "damping": true, "mix": true}
	if len(dv.fxSliderBindings) == 0 {
		t.Fatal("expected reverb param sliders")
	}
	for i, b := range dv.fxSliderBindings {
		if b.slotIndex != 0 {
			t.Errorf("binding %d: expected slot 0, got %d", i, b.slotIndex)
		}
		if !reverbParams[b.paramName] {
			t.Errorf("binding %d: param %q is not a reverb param — stale/foreign row", i, b.paramName)
		}
		// The slider's normalized value must match the live chain value.
		live := effects[0].Params[b.paramName]
		norm := (live - b.def.Min) / (b.def.Max - b.def.Min)
		if got := dv.fxPanelSliders[i].Value; abs64(got-norm) > 1e-6 {
			t.Errorf("binding %d (%s): slider value %f != live-derived %f", i, b.paramName, got, norm)
		}
	}
}

func TestFXPanelSwapEffectTypeUpdatesRows(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument

	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.toggleFXPanel(0)

	// Swap distortion -> delay by removing then adding.
	audio.RemoveInsertEffect(instID, 0)
	audio.AddInsertEffect(instID, audio.EffectDelay, nil)
	dv.buildFXPanel()

	delayParams := map[string]bool{}
	for _, def := range audio.InsertEffectCatalog()[audio.EffectDelay] {
		delayParams[def.Name] = true
	}
	distortionParams := map[string]bool{}
	for _, def := range audio.InsertEffectCatalog()[audio.EffectDistortion] {
		distortionParams[def.Name] = true
	}

	if len(dv.fxSliderBindings) == 0 {
		t.Fatal("expected delay param sliders after swap")
	}
	for i, b := range dv.fxSliderBindings {
		if !delayParams[b.paramName] {
			t.Errorf("binding %d: param %q is not a delay param after swap", i, b.paramName)
		}
		// Guard against a distortion-only param leaking (e.g. "drive").
		if distortionParams[b.paramName] && !delayParams[b.paramName] {
			t.Errorf("binding %d: stale distortion param %q survived the swap", i, b.paramName)
		}
	}
}

func TestFXParamSummaryDerivesFromLiveChain(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	instID := dv.Rows[0].Instrument

	audio.AddInsertEffect(instID, audio.EffectReverb, nil)
	// Mutate a param to a recognizable value.
	audio.SetInsertEffectParam(instID, 0, "room", 0.9)

	effects := audio.GetInsertEffects(instID)
	summary := fxParamSummary(effects[0])
	if summary == "" {
		t.Fatal("expected non-empty summary for reverb")
	}
	if !strings.Contains(summary, "·") {
		t.Errorf("expected '·' separator in multi-param summary, got %q", summary)
	}
	// The mutated room value (0.9) must appear in the summary.
	if !strings.Contains(summary, "0.9") {
		t.Errorf("summary %q does not reflect live room=0.9", summary)
	}
	// A foreign (distortion) param must not appear.
	if strings.Contains(strings.ToLower(summary), "drive") {
		t.Errorf("summary %q leaked a foreign distortion param", summary)
	}
}

// No stray separator for an effect without catalog params.
func TestFXParamSummaryEmptyForUnknownType(t *testing.T) {
	got := fxParamSummary(audio.EffectSlot{Type: audio.EffectType("nonexistent")})
	if got != "" {
		t.Errorf("expected empty summary for unknown effect type, got %q", got)
	}
}

// The shared green default stays for the synth pills, so the FX toggle and chrome accent must stay distinct
// from it.
func TestFXToggleColorIsAccentNotGreen(t *testing.T) {
	if colAccent == colPlayGreen {
		t.Skip("accent and play-green share a hex — color assertion is vacuous")
	}
	if colAccent.R == colPlayGreen.R && colAccent.G == colPlayGreen.G && colAccent.B == colPlayGreen.B {
		t.Error("FX toggle accent must differ from the iOS-green default")
	}
}
