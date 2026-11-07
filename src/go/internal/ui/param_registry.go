package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// paramFacet is the one presentation of a document parameter, shared by undo/redo notifications and the
// UI readouts, so a notification's "from → to" reads exactly like the control did.
type paramFacet struct {
	labelKey i18n.Key // localized label; "" ⇒ use literal
	literal  string   // label when labelKey == ""
	format   func(float64) string
}

// "@<key>" makes display() localize it on render; a literal is kept verbatim.
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

// Through the shared formatParamValue, so notification values and captions are byte-identical.
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

// The table samplerKnobCaption renders from, so the readout and the undo notification cannot drift.
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
	case "fade":
		return keyFacet(i18n.KeyFade, fmtOnOff), true
	case "reverse":
		return keyFacet(i18n.KeyReverse, fmtOnOff), true
	case "normalize":
		return keyFacet(i18n.KeyNormalize, fmtOnOff), true
	}
	return paramFacet{}, false
}

var modularDefByName = func() map[string]audio.ParamDef {
	m := map[string]audio.ParamDef{}
	for _, d := range audio.ModularSynthParamDefs() {
		m[d.Name] = d
	}
	return m
}()

// Same display name and formatter as the Synth tab; enums render their option label, not an index.
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

// Effect/param names are already specific jargon keys, so they stay literal.
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

// bands may be nil; then the 1-based band index labels it.
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

func titleParamWord(s string) string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == ' ' })
	for i, w := range fields {
		fields[i] = titleWord(w)
	}
	return strings.Join(fields, " ")
}
