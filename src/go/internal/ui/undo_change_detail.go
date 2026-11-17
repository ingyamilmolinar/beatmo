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

// changeDetail is a concrete "param: old → new" description derived by diffing
// the before/after undo snapshots, so an undo/redo notification can say WHICH
// parameter changed and from/to what value (instead of a generic "edit synth").
// param is either a literal label (e.g. "BPM", "Cutoff") or "@<i18n key>" which
// display() localizes. old/new are locale-neutral formatted strings (universal
// units: %, st, dB, Hz, x) produced by the shared paramFacet formatters, so the
// notification reads exactly like the control's on-screen readout.
type changeDetail struct {
	param string
	old   string
	new   string
}

func keyVolumeLabel() i18n.Key   { return i18n.KeyNodeSecVolume }
func keyPitchLabel() i18n.Key    { return i18n.KeyCapPitch }
func keyDurationLabel() i18n.Key { return i18n.KeyNodeSecDuration }

func volPctStr(v float64) string { return strconv.Itoa(int(math.Round(v*100))) + "%" }

// orDefaultOne maps a value the export omits at its 1.0 default (volume,
// duration) — which unmarshals to 0.0 when absent — back to 1.0.
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

// describeUndoChange diffs two export snapshots and returns the single scalar
// parameter that changed, formatted via the unified paramFacet registry. ok=false
// for structural changes (node/instrument added or removed) or when nothing
// scalar differs — the caller then falls back to the generic action label.
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
	// MasterVolume is *float64 (nil = legacy absent field = the historical
	// 1.0 default) — compare effective values, not pointer identities.
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
	// The instrument documents, compared through their flat field view so the
	// notification names the knob ("Cutoff: 800 Hz → 1.2 kHz").
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

// diffSampleEdit diffs two sample_edit maps after filling each with the sample
// IDENTITY (end_frac defaults to 1, everything else 0). Without this, a FIRST
// edit — where one side is absent — would materialise both end_frac=1 AND the
// changed knob, and the first-differing-key scan would mislabel a gain tweak as
// an "End" change. Identity-filling means only the genuinely-changed knob differs.
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

// diffFloatMap returns the first key (in sorted order, for determinism) whose
// value differs between two param maps, formatted through its facet. A nil/zero
// side reads as 0 so a newly-set or cleared key still produces a from → to.
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

// diffEQ reports the first differing EQ field (band gain, HPF/LPF cutoff or
// enable, band mute). prefix disambiguates a per-instrument EQ ("Kick EQ …")
// from the master EQ ("EQ …").
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

// diffEffects reports the first differing insert-effect param or enable toggle.
// Effects are matched by chain index; a different chain length is structural.
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

// diffTheme reports the first category whose swatch changed between two
// circuit theme blocks ("accent: orchid-300"), "(default)" when cleared.
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
