//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// synth_knob_i18n_test.go proves the Synth-tab knob LABELS and value ENUMS
// follow the active locale (the gap the user reported: "Triangle", "Pitch", …
// stayed English). Design mirrors the synth-stage pass: canonical English backs
// logic/round-trips, a localized display layer reads i18n.T, and the EN catalog
// mirrors canonical so every existing EN-locale test stays green.

func withLocaleES(t *testing.T) {
	t.Helper()
	i18n.SetLocale(i18n.LocaleES)
	t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })
}

// TestSynthParamDisplayName_LocalizedES asserts curated knob labels translate
// under the Spanish locale, including the composed FM-operator and burst-hit
// families (the "OpN"/"Hit N" prefix stays English; the noun translates).
func TestSynthParamDisplayName_LocalizedES(t *testing.T) {
	withLocaleES(t)
	cases := map[string]string{
		"fundamental":      "Tono",
		"voice_freq_hz":    "Tono",
		"pitch":            "Tono",
		"osc_type":         "Onda",
		"fm_wave":          "Onda",
		"bass_wave":        "Onda",
		"osc_octave":       "Octava",
		"osc_detune":       "Desafine",
		"amp_attack":       "Ataque",
		"amp_decay":        "Caída",
		"amp_sustain":      "Sostenido",
		"amp_release":      "Liberación",
		"amp_curve":        "Curva",
		"filter_type":      "Tipo",
		"filter_cutoff":    "Corte",
		"filter_resonance": "Resonancia",
		"drive":            "Saturación",
		"body":             "Cuerpo",
		"brightness":       "Brillo",
		"gain":             "Ganancia",
		"tone":             "Timbre",
		"lfo_rate":         "Velocidad",
		"lfo_depth":        "Profundidad",
		"burst_sharp":      "Nitidez",
		"noise_seed":       "Semilla",
		// Composed FM-operator family — prefix English, noun Spanish.
		"fm_op1_ratio": "Op1 Relación",
		"fm_op2_level": "Op2 Nivel",
		"fm_op3_depth": "Op3 Profundidad",
		"fm_op1_decay": "Op1 Caída",
		// Composed burst-hit family.
		"burst1_amp": "Golpe 1 Nivel",
		"burst3_off": "Golpe 3 Tiempo",
	}
	for id, want := range cases {
		if got := synthParamDisplayName(id); got != want {
			t.Errorf("ES synthParamDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestSynthParamDisplayName_ENMatchesCanonical pins zero behavior change in the
// English locale: the localized layer must reproduce the canonical caption for
// every curated id (including the composed families).
func TestSynthParamDisplayName_ENMatchesCanonical(t *testing.T) {
	i18n.SetLocale(i18n.LocaleEN)
	cases := map[string]string{
		"fundamental":      "Pitch",
		"pitch":            "Tune",
		"osc_type":         "Wave",
		"osc_octave":       "Octave",
		"amp_attack":       "Attack",
		"amp_decay":        "Decay",
		"filter_cutoff":    "Cutoff",
		"filter_resonance": "Resonance",
		"decay":            "Decay ×",
		"fm_op1_ratio":     "Op1 Ratio",
		"fm_op4_depth":     "Op4 Depth",
		"burst2_amp":       "Hit 2 Level",
		"burst2_off":       "Hit 2 Time",
	}
	for id, want := range cases {
		if got := synthParamDisplayName(id); got != want {
			t.Errorf("EN synthParamDisplayName(%q) = %q, want %q", id, got, want)
		}
	}
}

// TestEnumLabelFor_LocalizedES asserts the knob VALUE enums localize — the
// "Triangle" the user called out by name — while keeping the universal "FM"
// member English. Canonical strings stay the storage form (def.Enum), so this
// is a display-only transform.
func TestEnumLabelFor_LocalizedES(t *testing.T) {
	withLocaleES(t)
	wave := audio.ParamDef{Name: "osc_type", Min: 0, Max: 6,
		Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM", "Noise White", "Noise Pink"}}
	waveCases := map[int]string{
		0: "Seno", 1: "Sierra", 2: "Cuadrada", 3: "Triángulo",
		4: "FM", 5: "Ruido Blanco", 6: "Ruido Rosa",
	}
	for idx, want := range waveCases {
		if got := enumLabelFor(wave, float64(idx)); got != want {
			t.Errorf("ES enumLabelFor(osc_type,%d) = %q, want %q", idx, got, want)
		}
	}

	filter := audio.ParamDef{Name: "filter_type", Min: 0, Max: 2,
		Enum: []string{"Low-pass", "High-pass", "Band-pass"}}
	filterCases := map[int]string{0: "Paso bajo", 1: "Paso alto", 2: "Paso banda"}
	for idx, want := range filterCases {
		if got := enumLabelFor(filter, float64(idx)); got != want {
			t.Errorf("ES enumLabelFor(filter_type,%d) = %q, want %q", idx, got, want)
		}
	}

	curve := audio.ParamDef{Name: "amp_curve", Min: 0, Max: 1, Enum: []string{"Linear", "Exponential"}}
	if got := enumLabelFor(curve, 1); got != "Exponencial" {
		t.Errorf("ES enumLabelFor(amp_curve,1) = %q, want %q", got, "Exponencial")
	}

	algo := audio.ParamDef{Name: "fm_algorithm", Min: 0, Max: 3,
		Enum: []string{"2-op", "Parallel", "3-op chain", "4-op stack"}}
	algoCases := map[int]string{0: "2-op", 1: "Paralelo", 2: "Cadena 3-op", 3: "Pila 4-op"}
	for idx, want := range algoCases {
		if got := enumLabelFor(algo, float64(idx)); got != want {
			t.Errorf("ES enumLabelFor(fm_algorithm,%d) = %q, want %q", idx, got, want)
		}
	}
}

// TestEnumLabelFor_ENMatchesCanonical pins that the English locale returns the
// canonical enum strings verbatim (no behavior change for existing tests).
func TestEnumLabelFor_ENMatchesCanonical(t *testing.T) {
	i18n.SetLocale(i18n.LocaleEN)
	wave := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3,
		Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	for idx, want := range map[int]string{0: "Sine", 1: "Saw", 2: "Square", 3: "Triangle"} {
		if got := enumLabelFor(wave, float64(idx)); got != want {
			t.Errorf("EN enumLabelFor(osc_type,%d) = %q, want %q", idx, got, want)
		}
	}
}

// TestParseParamEntry_AcceptsLocalizedEnum proves the numeric editor round-trips
// the localized label a user sees: typing "Triángulo" under the Spanish locale
// resolves to the canonical index, and the canonical English form still works.
func TestParseParamEntry_AcceptsLocalizedEnum(t *testing.T) {
	withLocaleES(t)
	def := audio.ParamDef{Name: "osc_type", Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Triangle"}}
	if v, ok := parseParamEntry(def, "Triángulo"); !ok || v != 3 {
		t.Errorf("parseParamEntry(localized) = (%v,%v), want (3,true)", v, ok)
	}
	// Canonical English still parses regardless of locale.
	if v, ok := parseParamEntry(def, "square"); !ok || v != 2 {
		t.Errorf("parseParamEntry(canonical) = (%v,%v), want (2,true)", v, ok)
	}
}
