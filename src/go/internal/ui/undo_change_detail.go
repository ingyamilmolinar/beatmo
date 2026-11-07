package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Diffed from the before/after snapshots, so a notification names WHICH parameter changed, formatted by the
// shared facets to read exactly like the control.
type changeDetail struct {
	param string
	old   string
	new   string
}

func keyVolumeLabel() i18n.Key   { return i18n.KeyNodeSecVolume }
func keyPitchLabel() i18n.Key    { return i18n.KeyCapPitch }
func keyDurationLabel() i18n.Key { return i18n.KeyNodeSecDuration }

func volPctStr(v float64) string { return strconv.Itoa(int(math.Round(v*100))) + "%" }

// The export omits volume and duration at 1.0, which then unmarshal as 0.
func orDefaultOne(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// !ok for structural changes or no scalar difference; the caller uses the generic label.
func describeUndoChange(before, after []byte) (changeDetail, bool) {
	var b, a projectFile
	if json.Unmarshal(before, &b) != nil || json.Unmarshal(after, &a) != nil {
		return changeDetail{}, false
	}
	if len(b.Nodes) != len(a.Nodes) || len(b.Instruments) != len(a.Instruments) {
		return changeDetail{}, false
	}
	if b.BPM != a.BPM {
		return changeDetail{"BPM", strconv.Itoa(b.BPM), strconv.Itoa(a.BPM)}, true
	}
	if b.Subdiv != a.Subdiv {
		return changeDetail{"Subdivision", strconv.Itoa(b.Subdiv), strconv.Itoa(a.Subdiv)}, true
	}
	// nil means absent, i.e. 1.0: compare values, not pointers.
	bmv, amv := 1.0, 1.0
	if b.MasterVolume != nil {
		bmv = *b.MasterVolume
	}
	if a.MasterVolume != nil {
		amv = *a.MasterVolume
	}
	if bmv != amv {
		f := keyFacet(keyVolumeLabel(), func(v float64) string { return volPctStr(orDefaultOne(v)) })
		return f.detail(bmv, amv), true
	}
	if d, ok := diffTheme(b.Theme, a.Theme); ok {
		return d, true
	}
	bn := make(map[int]exportNode, len(b.Nodes))
	for _, n := range b.Nodes {
		bn[n.ID] = n
	}
	for _, an := range a.Nodes {
		if bv, ok := bn[an.ID]; ok {
			if d, ok := diffNode(bv, an); ok {
				return d, true
			}
		}
	}
	bi := make(map[instrument.ID]exportInstrument, len(b.Instruments))
	for _, in := range b.Instruments {
		bi[in.ID] = in
	}
	for _, ai := range a.Instruments {
		if bv, ok := bi[ai.ID]; ok {
			if d, ok := diffInstrument(bv, ai, b.InstrumentDocs[ai.ID].Doc, a.InstrumentDocs[ai.ID].Doc); ok {
				return d, true
			}
		}
	}
	if d, ok := diffEQ(b.EQ, a.EQ, ""); ok {
		return d, true
	}
	return changeDetail{}, false
}

func diffNode(b, a exportNode) (changeDetail, bool) {
	if orDefaultOne(b.Volume) != orDefaultOne(a.Volume) {
		f := keyFacet(keyVolumeLabel(), func(v float64) string { return volPctStr(orDefaultOne(v)) })
		return f.detail(b.Volume, a.Volume), true
	}
	if b.Pitch != a.Pitch {
		f := keyFacet(keyPitchLabel(), func(v float64) string { return fmt.Sprintf("%+d", int(v)) })
		return f.detail(b.Pitch, a.Pitch), true
	}
	if orDefaultOne(b.Duration) != orDefaultOne(a.Duration) {
		f := keyFacet(keyDurationLabel(), func(v float64) string { return fmt.Sprintf("%gx", orDefaultOne(v)) })
		return f.detail(b.Duration, a.Duration), true
	}
	if b.LogicN != a.LogicN {
		return litFacet("N", func(v float64) string { return strconv.Itoa(int(v)) }).detail(float64(b.LogicN), float64(a.LogicN)), true
	}
	if b.LogicP != a.LogicP {
		return litFacet("P", func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }).detail(b.LogicP, a.LogicP), true
	}
	if b.GroovePct != a.GroovePct {
		return litFacet("Groove", func(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }).detail(b.GroovePct, a.GroovePct), true
	}
	if b.Type != a.Type {
		return changeDetail{"Type", b.Type.String(), a.Type.String()}, true
	}
	if b.LogicKind != a.LogicKind {
		return changeDetail{"Logic", strOrNone(b.LogicKind.String()), strOrNone(a.LogicKind.String())}, true
	}
	if b.GrooveKind != a.GrooveKind {
		return changeDetail{"Groove", strOrNone(b.GrooveKind.String()), strOrNone(a.GrooveKind.String())}, true
	}
	return changeDetail{}, false
}

func diffInstrument(b, a exportInstrument, bDoc, aDoc instrument.Doc) (changeDetail, bool) {
	if b.Volume != a.Volume {
		return keyFacet(keyVolumeLabel(), volPctStr).detail(b.Volume, a.Volume), true
	}
	// Through the flat field view, so the notification names the knob.
	reg := audio.BuiltinModuleRegistry()
	if d, ok := diffFloatMap(instrument.OwnedFields(bDoc, reg), instrument.OwnedFields(aDoc, reg), func(k string) (paramFacet, bool) {
		return synthParamFacet(k), true
	}); ok {
		return d, true
	}
	if d, ok := diffSampleEdit(b.SampleEdit, a.SampleEdit); ok {
		return d, true
	}
	if d, ok := diffEQ(b.EQ, a.EQ, b.Name); ok {
		return d, true
	}
	if d, ok := diffEffects(b.Effects, a.Effects); ok {
		return d, true
	}
	return changeDetail{}, false
}

// Both sides are identity-filled first (end_frac 1, the rest 0): a FIRST edit would otherwise show end_frac
// changing too and mislabel a gain tweak as "End".
func diffSampleEdit(b, a map[string]float64) (changeDetail, bool) {
	if len(b) == 0 && len(a) == 0 {
		return changeDetail{}, false
	}
	return diffFloatMap(sampleEditIdentityFilled(b), sampleEditIdentityFilled(a), sampleEditFacet)
}

func sampleEditIdentityFilled(m map[string]float64) map[string]float64 {
	out := map[string]float64{
		"start_frac": 0, "end_frac": 1, "transpose_semis": 0, "detune_cents": 0,
		"gain_db": 0, "fade": 0, "reverse": 0, "normalize": 0,
	}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// Sorted keys for determinism; a missing side reads as 0 so set/cleared keys still show from → to.
func diffFloatMap(b, a map[string]float64, facet func(string) (paramFacet, bool)) (changeDetail, bool) {
	if len(b) == 0 && len(a) == 0 {
		return changeDetail{}, false
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(b)+len(a))
	for k := range b {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range a {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if b[k] == a[k] {
			continue
		}
		f, ok := facet(k)
		if !ok {
			f = litFacet(k, func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) })
		}
		return f.detail(b[k], a[k]), true
	}
	return changeDetail{}, false
}

// prefix separates a per-instrument EQ from the master EQ.
func diffEQ(b, a *exportEQ, prefix string) (changeDetail, bool) {
	if b == nil && a == nil {
		return changeDetail{}, false
	}
	var bb, aa exportEQ
	if b != nil {
		bb = *b
	}
	if a != nil {
		aa = *a
	}
	bands := aa.BandsHz
	if len(bands) == 0 {
		bands = bb.BandsHz
	}
	n := len(bb.GainsDB)
	if len(aa.GainsDB) > n {
		n = len(aa.GainsDB)
	}
	for k := 0; k < n; k++ {
		if gainAt(bb.GainsDB, k) != gainAt(aa.GainsDB, k) {
			return eqBandFacet(prefix, bands, k).detail(gainAt(bb.GainsDB, k), gainAt(aa.GainsDB, k)), true
		}
	}
	hpf := eqFilterLabel(prefix, "HPF")
	if bb.HPFCutoffHz != aa.HPFCutoffHz {
		return litFacet(hpf, fmtUnit("Hz")).detail(bb.HPFCutoffHz, aa.HPFCutoffHz), true
	}
	if bb.HPFEnabled != aa.HPFEnabled {
		return litFacet(hpf, fmtOnOff).detail(b2f(bb.HPFEnabled), b2f(aa.HPFEnabled)), true
	}
	lpf := eqFilterLabel(prefix, "LPF")
	if bb.LPFCutoffHz != aa.LPFCutoffHz {
		return litFacet(lpf, fmtUnit("Hz")).detail(bb.LPFCutoffHz, aa.LPFCutoffHz), true
	}
	if bb.LPFEnabled != aa.LPFEnabled {
		return litFacet(lpf, fmtOnOff).detail(b2f(bb.LPFEnabled), b2f(aa.LPFEnabled)), true
	}
	m := len(bb.BandMuted)
	if len(aa.BandMuted) > m {
		m = len(aa.BandMuted)
	}
	for k := 0; k < m; k++ {
		if mutedAt(bb.BandMuted, k) != mutedAt(aa.BandMuted, k) {
			f := eqBandFacet(prefix, bands, k)
			f.literal += " mute"
			return f.detail(b2f(mutedAt(bb.BandMuted, k)), b2f(mutedAt(aa.BandMuted, k))), true
		}
	}
	return changeDetail{}, false
}

func eqFilterLabel(prefix, which string) string {
	if prefix == "" {
		return which
	}
	return prefix + " " + which
}

func gainAt(s []float64, k int) float64 {
	if k >= 0 && k < len(s) {
		return s[k]
	}
	return 0
}

func mutedAt(s []bool, k int) bool {
	if k >= 0 && k < len(s) {
		return s[k]
	}
	return false
}

// Matched by chain index; a different chain length is structural.
func diffEffects(b, a []audio.EffectSlot) (changeDetail, bool) {
	if len(b) != len(a) {
		return changeDetail{}, false
	}
	for i := range b {
		if b[i].Type != a[i].Type {
			return changeDetail{titleParamWord(string(a[i].Type)), titleParamWord(string(b[i].Type)), titleParamWord(string(a[i].Type))}, true
		}
		if b[i].Enabled != a[i].Enabled {
			return litFacet(titleParamWord(string(b[i].Type)), fmtOnOff).detail(b2f(b[i].Enabled), b2f(a[i].Enabled)), true
		}
		if d, ok := diffFloatMap(b[i].Params, a[i].Params, func(k string) (paramFacet, bool) {
			return effectParamFacet(b[i].Type, k), true
		}); ok {
			return d, true
		}
	}
	return changeDetail{}, false
}
func strOrNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// "(default)" when cleared.
func diffTheme(b, a *theme.Doc) (changeDetail, bool) {
	get := func(d *theme.Doc, k string) string {
		if d == nil {
			return ""
		}
		return d.Categories[k]
	}
	for _, c := range themeCategories {
		bv, av := get(b, c.ID), get(a, c.ID)
		if bv == av {
			continue
		}
		fmtv := func(v string) string {
			if v == "" {
				return c.ID + ": (default)"
			}
			return c.ID + ": " + v
		}
		return changeDetail{"Theme", fmtv(bv), fmtv(av)}, true
	}
	return changeDetail{}, false
}
