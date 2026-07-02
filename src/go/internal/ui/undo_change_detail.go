package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
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
	var b, a importFile
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
	if b.MasterVolume != a.MasterVolume {
		f := keyFacet(keyVolumeLabel(), func(v float64) string { return volPctStr(orDefaultOne(v)) })
		return f.detail(b.MasterVolume, a.MasterVolume), true
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
	bi := make(map[string]exportInstrument, len(b.Instruments))
	for _, in := range b.Instruments {
		bi[in.ID] = in
	}
	for _, ai := range a.Instruments {
		if bv, ok := bi[ai.ID]; ok {
			if d, ok := diffInstrument(bv, ai); ok {
				return d, true
			}
		}
	}
	if d, ok := diffEQ(b.EQ, a.EQ, ""); ok {
		return d, true
	}
	if d, ok := diffSendEffects(b.SendEffects, a.SendEffects); ok {
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
		return changeDetail{"Type", b.Type, a.Type}, true
	}
	if b.LogicKind != a.LogicKind {
		return changeDetail{"Logic", strOrNone(b.LogicKind), strOrNone(a.LogicKind)}, true
	}
	if b.GrooveKind != a.GrooveKind {
		return changeDetail{"Groove", strOrNone(b.GrooveKind), strOrNone(a.GrooveKind)}, true
	}
	return changeDetail{}, false
}

func diffInstrument(b, a exportInstrument) (changeDetail, bool) {
	if b.Volume != a.Volume {
		return keyFacet(keyVolumeLabel(), volPctStr).detail(b.Volume, a.Volume), true
	}
	if b.Pan != a.Pan {
		return keyFacet(i18n.KeyParamPan, fmtPan).detail(b.Pan, a.Pan), true
	}
	if b.DelaySend != a.DelaySend {
		return keyFacet(i18n.KeyParamDelaySend, fmtPct100).detail(b.DelaySend, a.DelaySend), true
	}
	if b.ReverbSend != a.ReverbSend {
		return keyFacet(i18n.KeyParamReverbSend, fmtPct100).detail(b.ReverbSend, a.ReverbSend), true
	}
	if b.Recipe != a.Recipe {
		return changeDetail{"Recipe", b.Recipe, a.Recipe}, true
	}
	if d, ok := diffFloatMap(b.SynthParams, a.SynthParams, func(k string) (paramFacet, bool) {
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
		"gain_db": 0, "fade_in_ms": 0, "fade_out_ms": 0, "reverse": 0, "normalize": 0,
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

// diffSendEffects reports the first differing global send (delay/reverb) param.
func diffSendEffects(b, a *SendEffectsConfig) (changeDetail, bool) {
	if b == nil && a == nil {
		return changeDetail{}, false
	}
	var bb, aa SendEffectsConfig
	if b != nil {
		bb = *b
	}
	if a != nil {
		aa = *a
	}
	bd, ad := sendDelay(bb.Delay), sendDelay(aa.Delay)
	if bd.TimeMs != ad.TimeMs {
		return litFacet("Delay · Time", fmtUnit("ms")).detail(bd.TimeMs, ad.TimeMs), true
	}
	if bd.Feedback != ad.Feedback {
		return litFacet("Delay · Feedback", fmtUnit("")).detail(bd.Feedback, ad.Feedback), true
	}
	if bd.DampingHz != ad.DampingHz {
		return litFacet("Delay · Damping", fmtUnit("Hz")).detail(bd.DampingHz, ad.DampingHz), true
	}
	br, ar := sendReverb(bb.Reverb), sendReverb(aa.Reverb)
	if br.Room != ar.Room {
		return litFacet("Reverb · Room", fmtPct100).detail(br.Room, ar.Room), true
	}
	if br.Damping != ar.Damping {
		return litFacet("Reverb · Damping", fmtPct100).detail(br.Damping, ar.Damping), true
	}
	if br.Wet != ar.Wet {
		return litFacet("Reverb · Wet", fmtPct100).detail(br.Wet, ar.Wet), true
	}
	return changeDetail{}, false
}

func sendDelay(d *SendDelayConfig) SendDelayConfig {
	if d == nil {
		return SendDelayConfig{}
	}
	return *d
}
func sendReverb(r *SendReverbConfig) SendReverbConfig {
	if r == nil {
		return SendReverbConfig{}
	}
	return *r
}

func strOrNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
