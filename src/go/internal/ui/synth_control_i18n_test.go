//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Knob labels and enum values follow the locale; canonical English is the stored form, and the EN catalogue mirrors
// it.

func withLocaleES(t *testing.T) {
	t.Helper()
	i18n.SetLocale(i18n.LocaleES)
	t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })
}

// The composed families too: the "OpN"/"Hit N" prefix stays, the noun translates.
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

// English reproduces the canonical caption for every curated id.
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

// Display-only: def.Enum keeps the canonical strings, and the universal "FM" stays English.
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
