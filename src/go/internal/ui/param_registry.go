package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// paramFacet is the single source of truth for presenting ONE document
// parameter: a label (localizable via labelKey, else a literal) plus a value
// formatter that renders the value in the parameter's natural unit. It is shared
// by undo/redo notifications (undo_change_detail.go) AND by UI rendering (the
// Sampler captions route through sampleEditFacet) so a notification's "from → to"
// always reads exactly like the readout the user saw on the control. Centralising
// label+format here means a new knob is described specifically everywhere from a
// single edit, instead of a generic "edit synth" / "edit sample".
type paramFacet struct {
	labelKey i18n.Key // localized label; "" ⇒ use literal
	literal  string   // label when labelKey == ""
	format   func(float64) string
}

// notifLabel is the label form stored in a notification arg: "@<key>" so
// display() localizes it on render, or the literal verbatim.
func (f paramFacet) notifLabel() string {
	if f.labelKey != "" {
		return "@" + string(f.labelKey)
	}
	return f.literal
}

// renderLabel is the localized label for live UI rendering (Sampler captions).
func (f paramFacet) renderLabel() string {
	if f.labelKey != "" {
		return i18n.T(f.labelKey)
	}
	return f.literal
}

// detail builds the from → to changeDetail a notification shows for a scalar change.
func (f paramFacet) detail(old, nw float64) changeDetail {
	return changeDetail{param: f.notifLabel(), old: f.format(old), new: f.format(nw)}
}

func keyFacet(k i18n.Key, format func(float64) string) paramFacet {
	return paramFacet{labelKey: k, format: format}
}
func litFacet(s string, format func(float64) string) paramFacet {
	return paramFacet{literal: s, format: format}
}

// fmtUnit renders a value through the shared formatParamValue for a given unit,
// so notification values and UI captions are byte-identical.
func fmtUnit(unit string) func(float64) string {
	return func(v float64) string { return formatParamValue(v, unit) }
}

// fmtPct100 renders a 0–1 fraction as an integer percent ("0.3" → "30%").
func fmtPct100(v float64) string { return formatParamValue(v*100, "%") }

// fmtOnOff renders a 0/1 boolean param as Off/On.
func fmtOnOff(v float64) string {
	if v >= 0.5 {
		return "On"
	}
	return "Off"
}

// fmtPan renders a -1..+1 pan: 0 → "C", positive → "R<pct>", negative → "L<pct>".
func fmtPan(v float64) string {
	switch {
	case v > 0:
		return "R" + strconv.Itoa(int(math.Round(v*100)))
	case v < 0:
		return "L" + strconv.Itoa(int(math.Round(-v*100)))
	default:
		return "C"
	}
}

// sampleEditFacet maps a sample_edit map key to its facet, reusing the exact
// i18n labels and unit formatters the Sampler tab captions use. ok=false for an
// unknown key. This is the table samplerKnobCaption renders from, so the Sampler
// readout and the undo notification can never drift.
func sampleEditFacet(key string) (paramFacet, bool) {
	switch key {
	case "start_frac":
		return keyFacet(i18n.KeySamplerKnobStart, fmtPct100), true
	case "end_frac":
		return keyFacet(i18n.KeySamplerKnobEnd, fmtPct100), true
	case "transpose_semis":
		return keyFacet(i18n.KeySamplerKnobPitch, fmtUnit("st")), true
	case "detune_cents":
		return keyFacet(i18n.KeySamplerKnobFine, fmtUnit("cents")), true
	case "gain_db":
		return keyFacet(i18n.KeySamplerKnobGain, fmtUnit("dB")), true
	case "reverse":
		return keyFacet(i18n.KeyReverse, fmtOnOff), true
	case "normalize":
		return keyFacet(i18n.KeyNormalize, fmtOnOff), true
	}
	return paramFacet{}, false
}

// modularDefByName indexes the modular synth schema by param name so synth_params
// diffs resolve the same Label/Unit the Synth tab knobs render with.
var modularDefByName = func() map[string]audio.ParamDef {
	m := map[string]audio.ParamDef{}
	for _, d := range audio.ModularSynthParamDefs() {
		m[d.Name] = d
	}
	return m
}()

// synthParamFacet resolves a synth_params key to a facet using the SAME display
// name (synthParamDisplayName) and value formatter (formatSynthParamValue) the
// Synth tab uses. Enum params render their selected option label, not an index.
func synthParamFacet(key string) paramFacet {
	label := synthParamDisplayName(key)
	def, ok := modularDefByName[key]
	if !ok {
		return litFacet(label, func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) })
	}
	if len(def.Enum) > 0 {
		return litFacet(label, func(v float64) string {
			i := int(math.Round(v))
			if i >= 0 && i < len(def.Enum) {
				return localizeEnumLabel(def.Enum[i])
			}
			return strconv.Itoa(i)
		})
	}
	return litFacet(label, func(v float64) string { return formatSynthParamValue(def, v) })
}

// insertEffectCatalog caches the effect→param-defs map (immutable registry).
var insertEffectCatalog = audio.InsertEffectCatalog()

// effectParamFacet resolves one insert-effect param to a facet labelled
// "<Effect> · <Param>" with the registry's unit. Effect/param names are the
// canonical jargon keys — already specific, so they stay literal (not localized).
func effectParamFacet(t audio.EffectType, name string) paramFacet {
	unit := ""
	for _, d := range insertEffectCatalog[t] {
		if d.Name == name {
			unit = d.Unit
			break
		}
	}
	label := titleParamWord(string(t)) + " · " + titleParamWord(name)
	return litFacet(label, fmtUnit(unit))
}

// eqBandFacet labels EQ band k by its centre frequency ("EQ 8.0 kHz") and formats
// the gain in dB, matching the EQ tab readouts (formatHzShort + formatDB). bands
// may be nil (legacy snapshot) — then the 1-based band index is used.
func eqBandFacet(prefix string, bands [][2]float64, k int) paramFacet {
	freq := ""
	if k >= 0 && k < len(bands) {
		lo, hi := bands[k][0], bands[k][1]
		if lo > 0 && hi > 0 {
			freq = formatHzShort(math.Sqrt(lo * hi))
		}
	}
	label := strings.TrimSpace(prefix + " EQ")
	if freq != "" {
		label += " " + freq
	} else {
		label += " #" + strconv.Itoa(k+1)
	}
	return litFacet(label, formatDB)
}

// titleWordSlice / titleParamWord build a human label from a snake/space token.

// titleParamWord upper-cases the first rune of each underscore/space token, so
// "ring_mod" → "Ring Mod" and "feedback" → "Feedback".
func titleParamWord(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == ' ' })
	for i, w := range fields {
		fields[i] = titleWord(w)
	}
	return strings.Join(fields, " ")
}
