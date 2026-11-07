//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// PITCH ENV / LFO / BURST: off by default, an exact bypass while off, audible when on with the UI defaults. PITCH ENV
// acts only on a pitched osc stage or the FM core; LFO and BURST are post-mix and shape every voice.

const modStageSR = 48000
const modStageSamples = modStageSR / 2 // 0.5 s

// The values the NULL fast path uses.
func modIdentityVoice() modularBlock {
	return initBlockForTest(nil)
}

func renderModularFor(t *testing.T, p modularBlock) []float32 {
	t.Helper()
	buf := make([]float32, modStageSamples)
	renderModularBlock(buf, modStageSR, modStageSamples, p)
	return buf
}

func assertSameBytes(t *testing.T, name string, a, b []float32) {
	t.Helper()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("%s: sample %d differs: %v vs %v (must be byte-identical)", name, i, a[i], b[i])
		}
	}
}

func assertDiffers(t *testing.T, name string, a, b []float32) {
	t.Helper()
	for i := range a {
		if a[i] != b[i] {
			return
		}
	}
	t.Fatalf("%s: renders are byte-identical — the enabled stage is a silent no-op", name)
}

// ── PITCH ENV ──────────────────────────────────────────────────────────────

func TestPitchEnvDisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base.clone()
	withKnobs.set("pitchenv_amt", 12)
	withKnobs.set("pitchenv_decay", 0.08)
	// PitchEnvEnabled stays 0 (the identity): knobs set, stage off.
	assertSameBytes(t, "pitchenv knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestPitchEnvEnabledChangesOscRender(t *testing.T) {
	base := modIdentityVoice()
	on := base.clone()
	on.set("pitchenv_enabled", 1)
	on.set("pitchenv_amt", 12)
	on.set("pitchenv_decay", 0.08)
	assertDiffers(t, "pitchenv on (wavetable osc)", renderModularFor(t, base), renderModularFor(t, on))
}

func TestPitchEnvEnabledChangesFMRender(t *testing.T) {
	base := modIdentityVoice()
	base.set("osc_type", 4) // FM core
	base.set("fm_op1_level", 1)
	on := base.clone()
	on.set("pitchenv_enabled", 1)
	on.set("pitchenv_amt", 12)
	on.set("pitchenv_decay", 0.08)
	assertDiffers(t, "pitchenv on (FM core)", renderModularFor(t, base), renderModularFor(t, on))
}

// ── LFO ────────────────────────────────────────────────────────────────────

func TestLFODisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base.clone()
	withKnobs.set("lfo_rate", 8)
	withKnobs.set("lfo_depth", 0.5)
	assertSameBytes(t, "lfo knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestLFOEnabledChangesRender(t *testing.T) {
	base := modIdentityVoice()
	on := base.clone()
	on.set("lfo_enabled", 1)
	on.set("lfo_rate", 8)
	on.set("lfo_depth", 0.5)
	assertDiffers(t, "lfo on", renderModularFor(t, base), renderModularFor(t, on))
}

// On the gen-slot kick voice, not only the osc path.
func TestLFOShapesMigratedFamilyVoice(t *testing.T) {
	base := renderFieldsForTest(t, roster.Kick, modStageSR, modStageSamples, nil)
	on := renderFieldsForTest(t, roster.Kick, modStageSR, modStageSamples, map[string]float64{
		"lfo_enabled": 1, "lfo_rate": 8, "lfo_depth": 0.5,
	})
	assertDiffers(t, "lfo on migrated kick", base, on)
}

// ── BURST ──────────────────────────────────────────────────────────────────

func TestBurstDisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base.clone()
	withKnobs.set("burst_sharp", 40)
	withKnobs.set("burst1_amp", 1)
	withKnobs.set("burst2_off", 0.03)
	withKnobs.set("burst2_amp", 0.6)
	assertSameBytes(t, "burst knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestBurstEnabledChangesRender(t *testing.T) {
	base := modIdentityVoice()
	on := base.clone()
	on.set("burst_enabled", 1)
	on.set("burst_sharp", 40)
	on.set("burst1_amp", 1)
	on.set("burst2_off", 0.03)
	on.set("burst2_amp", 0.6)
	assertDiffers(t, "burst on", renderModularFor(t, base), renderModularFor(t, on))
}

func TestBurstShapesMigratedFamilyVoice(t *testing.T) {
	base := renderFieldsForTest(t, roster.Snare, modStageSR, modStageSamples, nil)
	on := renderFieldsForTest(t, roster.Snare, modStageSR, modStageSamples, map[string]float64{
		"burst_enabled": 1, "burst_sharp": 40, "burst1_amp": 1,
		"burst2_off": 0.03, "burst2_amp": 0.6,
	})
	assertDiffers(t, "burst on migrated snare", base, on)
}

// ── Identity / golden neutrality ───────────────────────────────────────────

// The appended modulator fields must not move the identity render.
func TestModulatorStagesIdentityIsByteNeutral(t *testing.T) {
	viaIdentity := renderModularFor(t, modIdentityVoice())
	null := make([]float32, modStageSamples)
	renderModularBlock(null, modStageSR, modStageSamples, initBlockForTest(nil))
	assertSameBytes(t, "identity vs NULL fast path", viaIdentity, null)
}
