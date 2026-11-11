//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthdiag"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// diagnose.go — `refs diagnose`: a PER-AXIS synth-vs-reference diagnostic.
//
// This is deliberately the opposite shape of internal/audio/synthmatch's
// Distance(): that collapses every metric into one weighted scalar, which is
// exactly how a 7x decay-time error and a small spectral improvement can
// cancel out (a real regression: winds work scored +17..+42% while sounding
// worse -- see docs/superpowers/specs/2026-08-22-verdict-to-metric-mapping.md).
// Every axis here is reported SEPARATELY, signed (delta = synth - reference),
// with its own verdict -- never folded into a combined score.
//
// It reuses cmd/refs/compare.go's computeCompare (renders the synth at the
// reference's own measured pitch, level-matches both, writes both WAVs) so
// the diagnostic analyzes EXACTLY the clips a listener would A/B -- then runs
// the shared internal/audio/fingerprint analyzers (used by both this CLI and
// the game's own analyzer) on each.

// The verdict and kind vocabularies now live in internal/audio/fingerprint,
// which owns the descriptor registry and therefore owns classification. These
// aliases keep this file's (and the acceptance test's) existing spellings
// working unchanged while the single source of truth moves.
type (
	axisVerdict = fingerprint.Verdict
	axisKind    = fingerprint.Kind
)

const (
	verdictOK       = fingerprint.VerdictOK
	verdictDrift    = fingerprint.VerdictDrift
	verdictMismatch = fingerprint.VerdictMismatch
)

const (
	axisFraction        = fingerprint.KindFraction
	axisSeconds         = fingerprint.KindSeconds
	axisHz              = fingerprint.KindHz
	axisTinyFraction    = fingerprint.KindTinyFraction
	axisSeriesDeviation = fingerprint.KindSeriesDeviation
)

// axisResult is one (instrument, axis) row of the diagnostic.
type axisResult struct {
	// ID is the descriptor's stable machine key (fingerprint.Descriptor.ID),
	// carried through so a caller can key on it instead of the human-facing
	// Group/Name pair, which is free to change wording.
	ID    string   `json:"id,omitempty"`
	Group string   `json:"group"`
	Name  string   `json:"axis"`
	Kind  axisKind `json:"-"`
	Unit  string   `json:"unit,omitempty"`

	// Delta is the comparator's own rendering ("7.1x short", "+36%",
	// "+0.341"). Stored rather than recomputed so there is ONE formatting
	// source of truth: the pre-registry code formatted deltas here while the
	// classifiers decided verdicts elsewhere, and the two could disagree.
	Delta string  `json:"delta,omitempty"`
	Ref   float64 `json:"reference"`
	Synth float64 `json:"synth"`
	// Norm is the descriptor's scale-normalized magnitude: >=1 means at or
	// past this axis's mismatch threshold, whatever its Kind or unit. It is
	// the only quantity comparable ACROSS axes, so it is what `refs validate`
	// correlates against the listener's verdicts. Never summed into a
	// composite -- see fingerprint.Delta's own doc for why.
	Norm float64 `json:"norm"`
	// Signed is the descriptor's own raw signed difference (synth - ref, in
	// the descriptor's units), carried through from fingerprint.Delta.Signed
	// unnormalized -- Norm is the cross-axis-comparable magnitude, Signed
	// keeps the sign + real units for callers that need the actual value
	// rather than a mismatch ratio.
	Signed float64 `json:"signed"`

	// Mag / MagOK carry fingerprint.Delta.Mag: the UNSIGNED magnitude the
	// comparator actually weighed against this axis's drift and mismatch, in
	// the units those thresholds are DECLARED in, and whether it is a
	// measurement at all. It is what `refs calibrate-thresholds` takes
	// percentiles of; nothing else in this struct is in the right units for
	// that (Signed is the raw difference, which is only the compared
	// quantity for the absolute-delta kinds).
	Mag   float64 `json:"mag"`
	MagOK bool    `json:"mag_ok"`

	Verdict axisVerdict `json:"verdict"`
	Note    string      `json:"note"`

	// TooLow/TooHigh describe the SIGN of the delta in axis-appropriate terms
	// (e.g. "too short" for a seconds axis, "too low" for a Hz/fraction axis).
	// Exposed so callers (including the acceptance-gate test) can assert
	// direction without re-deriving it from Ref/Synth.
	Direction string `json:"direction"` // "high" | "low" | "" (ok / undefined)

	// RefNA/SynthNA carry the reason a side could not be measured, empty when
	// it was. They exist so the report can print "n/a" instead of a zero that
	// reads as a measurement -- and, more importantly, so an ASYMMETRY (one
	// side measurable, the other not) is reported as the finding it is rather
	// than buried under a generic "not measurable". A reference that decays
	// where the synth sustains is exactly the kind of defect this tool exists
	// to catch.
	RefNA   string `json:"ref_na,omitempty"`
	SynthNA string `json:"synth_na,omitempty"`

	// Demoted mirrors fingerprint.Descriptor.Demoted: the axis is still
	// measured and still printed (a demoted row can be the most informative
	// thing on the page when you already know why it was demoted), but it is
	// excluded from every ranking this report drives -- the stage-sensitivity
	// routing list and the cross-instrument MISMATCH matrix -- because a row
	// calibration showed to mislead must not be what sends someone to a knob.
	// Rendered as a "[demoted]" suffix on the VERDICT cell so a reader of the
	// table never has to remember which axes are on the list.
	Demoted bool `json:"demoted,omitempty"`

	// RefText/SynthText are Descriptor.Headline's rendering for a measurement
	// whose Primary() would mislead -- an all-peer vector or a track, where
	// component 0 is just the first band/mode and carries no more authority
	// than any other. When set they REPLACE the numeric column; Ref/Synth
	// keep their raw Primary() so nothing downstream changes shape.
	RefText   string `json:"ref_text,omitempty"`
	SynthText string `json:"synth_text,omitempty"`
}

// Per-axis thresholds and the four classifiers moved to
// internal/audio/fingerprint (kind.go + classify.go) when the descriptor
// registry took ownership of classification. They were moved VERBATIM;
// diagnose_classifier_replay_test.go replays every measured (reference,
// synth) pair in the frozen baseline through the new comparators and asserts
// the verdicts and directions come out identical.

// seriesDevCaveatTh is the reference-side partial-series-deviation above which
// the noise-ish axes get an explicit unreliability caveat. It matches
// fingerprint.KindSeriesDeviation's mismatch threshold: if the reference's own
// partials diverge from a consecutive harmonic series by that much, the
// harmonic-bin assumption underneath NoiseRatio no longer holds for it.
var seriesDevCaveatTh = func() float64 {
	_, mismatch := fingerprint.KindSeriesDeviation.Thresholds()
	return mismatch
}()

// ringT60Note renders the ring-T60 row's bespoke note. When BOTH clips still
// ring above -60dB at their end, the T60 values are equally-CLAMPED lower
// bounds and comparing them is vacuous, so the descriptor's comparator
// escalated to the post-peak decay RATE -- say so plainly rather than
// printing a ratio of two bounds. When only one side is clamped the ordinary
// ratio still carries signal, but the note names which value is a bound.
func ringT60Note(refM, synthM fingerprint.Measurement, delta fingerprint.Delta) string {
	if !delta.OK {
		return "NOT MEASURABLE: " + delta.NA
	}
	const (
		tSec    = 0
		tExtrap = 1
		tRate   = 2
	)
	refExtrap := refM.Values[tExtrap] >= 0.5
	synthExtrap := synthM.Values[tExtrap] >= 0.5

	switch {
	case refExtrap && synthExtrap:
		note := fmt.Sprintf(
			"T60 NOT OBSERVABLE: both clips still ring above -60dB at clip end (ref clamped at %.2fs, synth at %.2fs) -- "+
				"comparing overall decay RATE instead (ref %.1f dB/s, synth %.1f dB/s)",
			refM.Values[tSec], synthM.Values[tSec], refM.Values[tRate], synthM.Values[tRate])
		if delta.Direction != "" {
			note += "; " + axisNote("ring T60", delta.Direction)
		}
		return note
	case refExtrap:
		return strings.TrimSpace(axisNote("ring T60", delta.Direction) +
			" (NOTE: reference clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)")
	case synthExtrap:
		return strings.TrimSpace(axisNote("ring T60", delta.Direction) +
			" (NOTE: synth clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)")
	}
	return axisNote("ring T60", delta.Direction)
}

// asymmetryNote explains why an axis could not be compared.
//
// The interesting case is ASYMMETRY: when one side measures fine and the other
// does not, the reason the other failed is itself a finding about the signal,
// not merely a gap in the tooling. A reference snare that decays cleanly while
// the synth establishes a sustain plateau cannot be compared on body-decay --
// but "the synth sustains where the reference does not" is precisely the
// defect worth reporting, so it is stated rather than flattened into a shared
// "not measurable".
func asymmetryNote(refM, synthM fingerprint.Measurement, delta fingerprint.Delta) string {
	switch {
	case refM.OK && !synthM.OK:
		return "NOT COMPARABLE -- measurable on the reference but NOT on the synth: " + synthM.NA +
			". That asymmetry is itself the finding: the synth's signal character differs from the reference's."
	case !refM.OK && synthM.OK:
		return "NOT COMPARABLE -- measurable on the synth but NOT on the reference: " + refM.NA +
			". That asymmetry is itself the finding: the reference's signal character differs from the synth's."
	default:
		return "NOT MEASURABLE on either side: " + delta.NA
	}
}

// axisNote maps (axis name, direction) to the plain-language callout used in
// the report. dir=="" (ok) produces no note.
func axisNote(name, dir string) string {
	if dir == "" {
		return ""
	}
	switch name {
	case "noise fraction", "spectral flatness":
		if dir == "high" {
			return "synth is far too breathy/noisy"
		}
		return "synth is too pure/tonal (missing noise character)"
	case "inharmonicity":
		if dir == "high" {
			return "synth partials are too inharmonic"
		}
		return "synth partials are too harmonic (needs inharmonic bar/tube modes)"
	case "partial series deviation":
		if dir == "high" {
			return "synth partials skip ranks worse than the reference (less like a consecutive harmonic series)"
		}
		return "synth partials follow a cleaner consecutive harmonic series than the reference (needs non-consecutive bar/tube-style overtones, e.g. ~1,4,9x)"
	case "attack":
		if dir == "low" {
			return "synth attack is far too fast"
		}
		return "synth attack is far too slow"
	case "ring T60", "decay":
		if dir == "low" {
			return "synth decays far too fast"
		}
		return "synth decay/ring is far too sustained"
	case "release":
		if dir == "low" {
			return "synth release is far too fast"
		}
		return "synth release is far too slow"
	case "centroid":
		if dir == "high" {
			return "synth is brighter"
		}
		return "synth is darker/duller"
	case "rolloff":
		if dir == "high" {
			return "synth has too much high-frequency energy"
		}
		return "synth is missing high-frequency energy"
	case "sustain modulation":
		if dir == "low" {
			return "synth sustain is missing amplitude modulation (flat/organ-like)"
		}
		return "synth sustain wobbles more than the reference"
	default:
		if dir == "high" {
			return "synth is too high on this axis"
		}
		return "synth is too low on this axis"
	}
}

// instrumentDiagnosis is the full per-axis report for one instrument.
type instrumentDiagnosis struct {
	Instrument instrument.ID `json:"instrument"`
	RefID      string        `json:"reference_id"`
	Axes       []axisResult  `json:"axes"`

	// Pitch/DurSec/SR record the EXACT render this diagnosis describes: the
	// semitone offset from A3 the synth was rendered at (the reference's own
	// measured pitch, not its stored label), the render duration, and the
	// sample rate. They are carried so a follow-up analysis -- `-ablate`,
	// which re-renders the same instrument once per synthesis stage -- can
	// reproduce this render exactly. Re-deriving them would silently drift the
	// moment renderPitch's f0-reliability rule or the -dur default changes,
	// and an ablation of a DIFFERENT render than the one reported is a
	// plausible-looking answer to the wrong question.
	Pitch  float64 `json:"pitch,omitempty"`
	DurSec float64 `json:"dur_sec,omitempty"`
	SR     int     `json:"sample_rate,omitempty"`

	// RefPartialRatios/SynthPartialRatios are the raw partial-to-f0 ratio
	// lists (fingerprint.PartialRatios.Ratios) printed side by side so a
	// reader can see the STRUCTURE difference directly -- e.g.
	// "ref 1.00, 4.02, 9.15 / synth 1.00, 2.00, 3.00" makes "reference is a
	// bar, synth is a harmonic series" legible in a way no single scalar
	// can. Not rendered as a normal axis row since they are lists, not a
	// single ref/synth pair.
	RefPartialRatios   []float64 `json:"ref_partial_ratios,omitempty"`
	SynthPartialRatios []float64 `json:"synth_partial_ratios,omitempty"`

	// RefClipTruncatedSec is nonzero when the reference's measured duration
	// is at (or past) the corpus's own convert.max_sec fetch cap -- i.e.
	// this reference WAV was truncated at fetch time, so any decay/ring
	// axis measured against it may be looking at less of the real
	// instrument than actually exists. Surfaced explicitly rather than
	// silently comparing against a truncated tail (see ringT60Axis).
	RefClipTruncatedSec float64 `json:"ref_clip_truncated_sec,omitempty"`
}

// refConvertMaxSec returns ref.Convert.MaxSec, or 0 when Convert is unset.
func refConvertMaxSec(ref wavref.Ref) float64 {
	if ref.Convert == nil {
		return 0
	}
	return ref.Convert.MaxSec
}

// Axis looks up one axis by exact name (used by the acceptance-gate test).
func (d instrumentDiagnosis) Axis(name string) (axisResult, bool) {
	for _, a := range d.Axes {
		if a.Name == name {
			return a, true
		}
	}
	return axisResult{}, false
}

// toWave converts decoded PCM samples into a wave.Wave for fingerprint analysis.
func toWave(samples []float32, sr int, label string) wave.Wave {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = float64(s)
	}
	return wave.Wave{Samples: out, SampleRate: sr, Label: label}
}

// diagnoseInstrumentDur is diagnoseInstrument's -dur-aware core. durFlag>0
// overrides the SYNTH's render duration only (renderCompare's durFlag
// parameter never affects the reference side, which is always whatever's
// decoded from the corpus WAV on disk). This matters because the reference
// clip length is fixed by the corpus's own `convert.max_sec` fetch config
// (typically 2-3s) -- there is no way to get MORE reference audio without
// re-fetching from the network, which is out of scope for a diagnostic
// read. What -dur DOES let you do is render the SYNTH for longer to see how
// it behaves past the reference's own clip length -- useful when the
// reference's ring genuinely outlasts its clip (see refClipTruncatedSec
// below and ringT60Axis's escalation for the case where BOTH sides clamp).
func diagnoseInstrumentDur(diag io.Writer, inst instrument.ID, refID string, durFlag float64) (instrumentDiagnosis, error) {
	return diagnoseInstrumentOpts(diag, inst, refID, durFlag, nil)
}

// diagnoseInstrumentOpts is diagnoseInstrumentDur's param-override-aware
// core: overrides, when non-nil, renders inst on top of its shipped defaults
// via renderCompareOverrides (see synthmatch.RenderWithParams) instead of
// inst's plain shipped sound -- this is how a session candidate's proposed
// knob tweak gets diagnosed against the reference exactly like a shipped
// instrument would.
//
// When refID == "" and overrides is set, there is no computeCompare
// equivalent to fall back on (computeCompare always renders the instrument's
// OWN default candidate reference with no override support), so the first of
// the instrument's own candidate references (compareCandidates) is used
// directly, mirroring computeCompare's own "no -ref given" default pick.
func diagnoseInstrumentOpts(diag io.Writer, inst instrument.ID, refID string, durFlag float64, overrides map[string]float64) (instrumentDiagnosis, error) {
	var res compareResult
	switch {
	case refID != "":
		c, err := loadCatalogue()
		if err != nil {
			return instrumentDiagnosis{}, err
		}
		ref, ok := c.Ref(refID)
		if !ok {
			return instrumentDiagnosis{}, fmt.Errorf("refs diagnose: unknown reference id %q", refID)
		}
		res, err = renderCompareOverrides(diag, inst, overrides, ref, durFlag, matchRMS)
		if err != nil {
			return instrumentDiagnosis{}, err
		}
	case overrides != nil:
		c, err := loadCatalogue()
		if err != nil {
			return instrumentDiagnosis{}, err
		}
		candidates := compareCandidates(c, inst)
		if len(candidates) == 0 {
			return instrumentDiagnosis{}, fmt.Errorf("refs diagnose: no reference audio for instrument %q", inst)
		}
		res, err = renderCompareOverrides(diag, inst, overrides, candidates[0], durFlag, matchRMS)
		if err != nil {
			return instrumentDiagnosis{}, err
		}
	default:
		var err error
		res, err = computeCompare(diag, inst, "", durFlag, matchRMS)
		if err != nil {
			return instrumentDiagnosis{}, err
		}
	}

	realSamples, realRate, err := audio.DecodeWAVAtRate(res.RealPath, wavref.Rate)
	if err != nil {
		return instrumentDiagnosis{}, fmt.Errorf("refs diagnose: decode %s: %w", res.RealPath, err)
	}
	synthSamples, synthRate, err := audio.DecodeWAVAtRate(res.SynthPath, wavref.Rate)
	if err != nil {
		return instrumentDiagnosis{}, fmt.Errorf("refs diagnose: decode %s: %w", res.SynthPath, err)
	}

	refWave := toWave(realSamples, realRate, res.Ref.ID)
	synthWave := toWave(synthSamples, synthRate, string(inst))

	// Bundles memoize every shared artifact (fingerprint, envelope segmentation,
	// STFTs) so a 150-descriptor report computes each one exactly once.
	refB := fingerprint.NewBundle(refWave, res.Ref.ID)
	synthB := fingerprint.NewBundle(synthWave, string(inst))
	refFp := refB.Fingerprint()
	synthFp := synthB.Fingerprint()

	out := instrumentDiagnosis{Instrument: inst, RefID: res.Ref.ID, Pitch: res.Pitch, DurSec: res.Dur, SR: wavref.Rate}
	if maxSec := refConvertMaxSec(res.Ref); maxSec > 0 && res.RealM.DurationSec >= maxSec-1e-6 {
		out.RefClipTruncatedSec = maxSec
	}

	// ---- Registry-driven axes -------------------------------------------
	//
	// Every axis comes from internal/audio/fingerprint's descriptor registry.
	// This loop is the WHOLE axis-construction path: adding a measurement is a
	// registry row, never an edit here. Applicability is evaluated against the
	// REFERENCE bundle, because the reference defines what kind of instrument
	// this is -- letting a synth that failed to sustain suppress the sustain
	// axes would hide exactly the defect being looked for.
	for _, d := range fingerprint.DefaultRegistry().Applicable(refB) {
		refM := d.Extract(refB)
		synthM := d.Extract(synthB)
		delta := d.CompareWith(refM, synthM)

		ax := axisResult{
			ID:        d.ID,
			Group:     d.Group,
			Name:      d.Name,
			Kind:      d.Kind,
			Unit:      d.Unit,
			Delta:     delta.Display,
			Ref:       refM.Primary(),
			Synth:     synthM.Primary(),
			Norm:      delta.Norm,
			Signed:    delta.Signed,
			Mag:       delta.Mag,
			MagOK:     delta.MagOK,
			Verdict:   delta.Verdict,
			Direction: delta.Direction,
			Note:      axisNote(d.Name, delta.Direction),
			Demoted:   d.Demoted,
		}
		if !refM.OK {
			ax.RefNA = refM.NA
		}
		if !synthM.OK {
			ax.SynthNA = synthM.NA
		}
		if txt, ok := d.Headline(refM); ok {
			ax.RefText = txt
		}
		if txt, ok := d.Headline(synthM); ok {
			ax.SynthText = txt
		}
		if !delta.OK {
			// An honest not-applicable: say why, and assert no verdict.
			ax.Verdict = ""
			ax.Direction = ""
			ax.Note = asymmetryNote(refM, synthM, delta)
		}
		if d.ID == fingerprint.IDEnvRingT60 {
			ax.Note = ringT60Note(refM, synthM, delta)
		}
		out.Axes = append(out.Axes, ax)
	}

	// The noise-ish axes carry an explicit caveat when the REFERENCE itself is
	// strongly non-harmonic: NoiseRatio counts any energy outside +/-1 bin of an
	// ASSUMED harmonic series as noise, which is the wrong model for a bell or
	// bar whose real partials are tonal but not where that assumption expects
	// them. Tubular-bells is the worked case -- ref noise fraction reads 0.875,
	// implausibly high for a bell, while ref spectral flatness reads 0.030
	// (very pure). That contradiction is the tell.
	refSeriesDev := refFp.PartialRatioProfile.SeriesDeviation
	if refSeriesDev >= seriesDevCaveatTh {
		caveat := fmt.Sprintf(" (CAVEAT: reference partial series deviation=%.2f -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)", refSeriesDev)
		for i := range out.Axes {
			if out.Axes[i].Name == "noise fraction" || out.Axes[i].Name == "spectral flatness" {
				out.Axes[i].Note = strings.TrimSpace(out.Axes[i].Note + caveat)
			}
		}
	}

	out.RefPartialRatios = refFp.PartialRatioProfile.Ratios
	out.SynthPartialRatios = synthFp.PartialRatioProfile.Ratios

	return out, nil
}

// --- Reporting ---------------------------------------------------------------

func formatDelta(a axisResult) string {
	if a.RefNA != "" || a.SynthNA != "" {
		return "n/a"
	}
	if a.Delta != "" {
		return a.Delta
	}
	switch a.Kind {
	case axisSeconds:
		if a.Ref <= 0 || a.Synth <= 0 {
			return "n/a"
		}
		ratio := a.Synth / a.Ref
		if ratio < 1 {
			return fmt.Sprintf("%.1fx short", 1/ratio)
		}
		return fmt.Sprintf("%.1fx long", ratio)
	case axisHz:
		if a.Ref <= 0 {
			return "n/a"
		}
		pct := (a.Synth - a.Ref) / a.Ref * 100
		return fmt.Sprintf("%+.0f%%", pct)
	default:
		return fmt.Sprintf("%+.3f", a.Synth-a.Ref)
	}
}

// formatValue renders one side's value, or "n/a" when that side could not be
// measured -- never a zero standing in for an absent measurement.
func formatValue(a axisResult, v float64, na string) string {
	if na != "" {
		return "n/a"
	}
	// An explicit Unit wins: a rate (Hz/s, dB/s, 1/s^2) must not print with
	// the bare "Hz" its Kind's default would supply.
	if a.Unit != "" {
		return fmt.Sprintf("%.4g %s", v, a.Unit)
	}
	switch a.Kind {
	case axisSeconds:
		return fmt.Sprintf("%.3f s", v)
	case axisHz:
		return fmt.Sprintf("%.0f Hz", v)
	default:
		return fmt.Sprintf("%.3f", v)
	}
}

// formatValueText is formatValue with the descriptor's Headline override
// applied: an all-peer vector or track prints what it IS ("(vector: 6/10
// bands measurable)") instead of offering component 0 as if it were the
// reading. The n/a case still wins -- a side that could not be measured has
// no headline either.
func formatValueText(a axisResult, v float64, na, text string) string {
	if na != "" {
		return "n/a"
	}
	if text != "" {
		return text
	}
	return formatValue(a, v, na)
}

// formatVerdict renders the VERDICT cell, tagging demoted axes so the reader
// of the table does not have to carry the demotion list in their head. A
// demoted row is still shown with its real verdict -- the demotion says "do
// not RANK on this", not "this measurement is meaningless" -- but the tag
// travels with it everywhere it is printed.
func formatVerdict(a axisResult) string {
	v := string(a.Verdict)
	if !a.Demoted {
		return v
	}
	if v == "" {
		return "[demoted]"
	}
	return v + " [demoted]"
}

// printDiagnosis writes the human-readable per-axis table for one instrument,
// grouped exactly as the mapping doc describes. Axes are never collapsed into
// a single score -- each group and each row is printed independently.
func printDiagnosis(w io.Writer, d instrumentDiagnosis) {
	fmt.Fprintf(w, "=== %s (reference: %s) ===\n\n", d.Instrument, d.RefID)
	if d.RefClipTruncatedSec > 0 {
		fmt.Fprintf(w, "NOTE: the reference clip is TRUNCATED to %.2fs by the corpus's fetch config (convert.max_sec) --\n", d.RefClipTruncatedSec)
		fmt.Fprintln(w, "the real instrument recording may ring/decay further than this reference can show. Pass")
		fmt.Fprintln(w, "-dur <seconds> to render the SYNTH for longer (the reference itself cannot be extended without")
		fmt.Fprintln(w, "re-fetching from the network).")
		fmt.Fprintln(w)
	}

	var groups []string
	seen := map[string]bool{}
	for _, a := range d.Axes {
		if !seen[a.Group] {
			seen[a.Group] = true
			groups = append(groups, a.Group)
		}
	}

	for _, g := range groups {
		fmt.Fprintf(w, "-- %s --\n", g)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "AXIS\tREFERENCE\tSYNTH\tDELTA\tVERDICT\tNOTE")
		for _, a := range d.Axes {
			if a.Group != g {
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				a.Name, formatValueText(a, a.Ref, a.RefNA, a.RefText),
				formatValueText(a, a.Synth, a.SynthNA, a.SynthText),
				formatDelta(a), formatVerdict(a), a.Note)
		}
		tw.Flush()
		if g == "partial structure" && (len(d.RefPartialRatios) > 0 || len(d.SynthPartialRatios) > 0) {
			fmt.Fprintf(w, "  ref partials (x f0):   %s\n", formatRatios(d.RefPartialRatios))
			fmt.Fprintf(w, "  synth partials (x f0): %s\n", formatRatios(d.SynthPartialRatios))
		}
		fmt.Fprintln(w)
	}
}

// formatRatios renders a partial-ratio list as "1.00, 4.02, 9.15" (or
// "(none)" when empty) -- printed directly beside the axis table so the
// STRUCTURE difference between two partial sequences is legible at a
// glance, which no single scalar delta can convey (see instrumentDiagnosis's
// RefPartialRatios/SynthPartialRatios doc comment).
func formatRatios(ratios []float64) string {
	if len(ratios) == 0 {
		return "(none)"
	}
	parts := make([]string, len(ratios))
	for i, r := range ratios {
		parts[i] = fmt.Sprintf("%.2f", r)
	}
	return strings.Join(parts, ", ")
}

// warnAblateIgnoredByJSON says out loud that -ablate did nothing on the -json
// path. The matrix is a report shape, not an axis row, so it has no JSON
// rendering -- and a flag that exits 0 having silently done nothing is exactly
// what sends someone looking for a bug in the analysis instead of in their
// command line.
func warnAblateIgnoredByJSON(diag io.Writer, ablate bool) {
	if !ablate {
		return
	}
	fmt.Fprintln(diag, "refs diagnose: -ablate is a report-only addendum; ignored with -json")
}

// printAblation prints, for the axes the report called MISMATCH, which
// synthesis stages actually move them -- the bridge from "what is wrong" to
// "which knob group to reach for".
//
// Only MISMATCH rows are listed: the whole point is to route the axes that
// need fixing, and a full stage x axis matrix (which -all -ablate does print)
// buries them. An axis no stage moves is reported as such rather than omitted:
// "nothing in the parameter set touches this" is a finding -- it means the gap
// is engine-level (a missing model), not a tuning job.
func printAblation(w io.Writer, d instrumentDiagnosis, rep synthdiag.Report) {
	fmt.Fprintln(w, "\n-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tTOP STAGES (\u0394norm above the no-effect floor when the stage is at identity)")
	for _, a := range d.Axes {
		// Demoted axes are excluded: this list is a ROUTING table ("which
		// knob group should I reach for"), and routing someone to a stage on
		// the strength of an axis calibration showed to mislead is the exact
		// failure the demotion exists to prevent. The row is still in the
		// table above, tagged, for anyone who wants to look at it.
		if a.Verdict != verdictMismatch || a.ID == "" || a.Demoted {
			continue
		}
		top := rep.TopStages(a.ID, 3)
		if len(top) == 0 {
			fmt.Fprintf(tw, "%s\t(no stage moves it \u2014 engine-level)\n", a.Name)
			continue
		}
		parts := make([]string, len(top))
		for i, s := range top {
			parts[i] = fmt.Sprintf("%s %.2f", s.Stage, s.Norm)
		}
		fmt.Fprintf(tw, "%s\t%s\n", a.Name, strings.Join(parts, "  "))
	}
	tw.Flush()
}

// printAblationMatrix prints the FULL stage x axis sensitivity matrix. Used by
// -all -ablate, where the cross-instrument question is "does the same stage
// dominate the same axis everywhere", which needs every cell, not just the
// MISMATCH rows.
func printAblationMatrix(w io.Writer, rep synthdiag.Report) {
	fmt.Fprintf(w, "\n-- stage x axis sensitivity matrix: %s (\u0394norm above the no-effect floor; 0 = no effect, '-' = not comparable) --\n", rep.Instrument)
	if len(rep.Stages) == 0 {
		fmt.Fprintln(w, "(no active stages to ablate)")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprint(tw, "AXIS")
	for _, st := range rep.Stages {
		fmt.Fprintf(tw, "\t%s", st)
	}
	fmt.Fprintln(tw)
	for ai, name := range rep.Names {
		fmt.Fprint(tw, name)
		for si := range rep.Stages {
			v := rep.Sens[si][ai]
			if math.IsNaN(v) {
				fmt.Fprint(tw, "\t-")
				continue
			}
			fmt.Fprintf(tw, "\t%.2f", v)
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
}

// printSummaryMatrix prints, for -all, a compact instrument x axis matrix
// showing ONLY the MISMATCHes -- the cross-instrument-pattern view. Rows are
// sorted by instrument; columns are the union of axis names encountered, in
// first-seen order.
func printSummaryMatrix(w io.Writer, all []instrumentDiagnosis) {
	// Demoted axes get no column at all: this matrix exists to expose
	// CROSS-INSTRUMENT patterns, and an axis that misleads on one instrument
	// misleads on all of them -- a demoted row would paint a column of
	// MISMATCHes that reads as the strongest pattern on the page.
	var axisOrder []string
	seenAxis := map[string]bool{}
	for _, d := range all {
		for _, a := range d.Axes {
			if a.Demoted {
				continue
			}
			if !seenAxis[a.Name] {
				seenAxis[a.Name] = true
				axisOrder = append(axisOrder, a.Name)
			}
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprint(tw, "INSTRUMENT")
	for _, name := range axisOrder {
		fmt.Fprintf(tw, "\t%s", name)
	}
	fmt.Fprintln(tw)

	any := false
	for _, d := range all {
		hasMismatch := false
		cells := make([]string, len(axisOrder))
		for i, name := range axisOrder {
			a, ok := d.Axis(name)
			if !ok || a.Demoted {
				cells[i] = "-"
				continue
			}
			if a.Verdict == verdictMismatch {
				hasMismatch = true
				dir := a.Direction
				if dir == "" {
					dir = "?"
				}
				cells[i] = "MISMATCH(" + dir + ")"
			} else {
				cells[i] = "-"
			}
		}
		if !hasMismatch {
			continue
		}
		any = true
		fmt.Fprint(tw, d.Instrument)
		for _, c := range cells {
			fmt.Fprintf(tw, "\t%s", c)
		}
		fmt.Fprintln(tw)
	}
	tw.Flush()
	if !any {
		fmt.Fprintln(w, "(no MISMATCHes across any comparable instrument)")
	}
}

func runDiagnose(w, diag io.Writer, inst instrument.ID, all bool, asJSON bool, durFlag float64, ablate bool) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}

	var insts []instrument.ID
	if all {
		insts = comparableInstruments(c)
	} else {
		if inst == "" {
			return fmt.Errorf("refs diagnose: -instrument or -all is required")
		}
		insts = []instrument.ID{inst}
	}

	var results []instrumentDiagnosis
	for _, id := range insts {
		d, err := diagnoseInstrumentDur(diag, id, "", durFlag)
		if err != nil {
			fmt.Fprintf(diag, "refs diagnose: %s: %v\n", id, err)
			continue
		}
		results = append(results, d)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Instrument < results[j].Instrument })

	if asJSON {
		warnAblateIgnoredByJSON(diag, ablate)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	if all {
		for _, d := range results {
			printDiagnosis(w, d)
			if ablate {
				emitAblation(w, diag, d, true, nil)
			}
		}
		fmt.Fprintln(w, "=== summary (MISMATCHes only) ===")
		printSummaryMatrix(w, results)
		return nil
	}
	if len(results) == 0 {
		return fmt.Errorf("refs diagnose: no result for %q", inst)
	}
	printDiagnosis(w, results[0])
	if ablate {
		emitAblation(w, diag, results[0], false, nil)
	}
	return nil
}

// emitAblation runs the ablation for d's OWN render (same pitch, duration,
// sample rate and param overrides) and prints it. A failure is reported to
// diag and does not fail the diagnosis: the per-axis report is the command's
// answer, and the stage routing is an addendum to it.
func emitAblation(w, diag io.Writer, d instrumentDiagnosis, full bool, overrides map[string]float64) {
	rep, err := synthdiag.Ablate(d.Instrument, d.Pitch, d.SR, d.DurSec, overrides)
	if err != nil {
		fmt.Fprintf(diag, "refs diagnose: ablate %s: %v\n", d.Instrument, err)
		return
	}
	if full {
		printAblationMatrix(w, rep)
		return
	}
	printAblation(w, d, rep)
}

// parseOverrideList parses a `-override` value: a comma-separated list of
// `key=value` pairs, each value a float. It is the CLI door onto
// diagnoseInstrumentOpts's overrides argument, so a candidate's proposed knob
// tweak can be diagnosed WITHOUT editing a seed — which is exactly what
// authoring audition_candidates.json needs (the candidates are overrides, and
// seeds are off-limits to that authoring loop).
//
// An empty string yields a nil map (meaning "no overrides", the shipped
// sound), so callers can pass the flag's zero value straight through.
func parseOverrideList(s string) (map[string]float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := map[string]float64{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" {
			return nil, fmt.Errorf("refs diagnose: -override %q: want key=value", part)
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("refs diagnose: -override %q: %w", part, err)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("refs diagnose: -override: duplicate key %q", k)
		}
		out[k] = f
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// validateOverrideKeys rejects an override key that is not a parameter of
// inst's recipe. Without this an unknown key is silently DROPPED by the render
// path, so the operator measures the shipped sound while believing they
// measured a candidate — the same failure mode
// TestShippedCandidates_AreLive guards for the shipped candidates file, moved
// to the CLI door. The known set is the recipe defaults merged with the
// instrument's own overrides, which is exactly what that test uses.
func validateOverrideKeys(inst instrument.ID, ov map[string]float64) error {
	if len(ov) == 0 {
		return nil
	}
	known := audio.MergedInstrumentParamsRO(inst)
	var bad []string
	for k := range ov {
		if _, ok := known[k]; !ok {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("-override: %s is not a value of instrument %q's document; an unknown key is dropped by the renderer, "+
		"so this would have measured the SHIPPED sound", strings.Join(bad, ", "), inst)
}

func cmdDiagnose(args []string) error {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to diagnose")
	ref := fs.String("ref", "", "reference id to diagnose against (default: first candidate; ignored with -all)")
	all := fs.Bool("all", false, "diagnose every comparable instrument and print a MISMATCH summary matrix")
	asJSON := fs.Bool("json", false, "emit raw per-axis numbers as JSON instead of the human-readable report")
	dur := fs.Float64("dur", 0, "override the SYNTH's render duration in seconds (default: match the reference's own measured duration). "+
		"Does NOT extend the reference clip -- that is fixed by the corpus's own fetch config (convert.max_sec) and cannot be lengthened "+
		"without re-fetching from the network. Useful for seeing further into a synth's own decay/ring when the reference is clip-truncated "+
		"(see the 'reference clip is TRUNCATED' note printed in that case).")
	ablate := fs.Bool("ablate", false, "after the report, render each synthesis stage at identity and print which stages move every MISMATCH axis")
	override := fs.String("override", "", "comma-separated key=value recipe-parameter overrides to render the synth with "+
		"(e.g. -override gen4_gain=0.12,lfo_enabled=0) -- diagnoses a PROPOSED sound without editing its seed; ignored with -all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *all {
		if *override != "" {
			return fmt.Errorf("refs diagnose: -override applies to a single -instrument, not -all")
		}
		return runDiagnose(os.Stdout, os.Stderr, "", true, *asJSON, *dur, *ablate)
	}
	if *inst == "" {
		return fmt.Errorf("refs diagnose: -instrument or -all is required")
	}
	ov, err := parseOverrideList(*override)
	if err != nil {
		return err
	}
	if err := validateOverrideKeys(instrument.ID(*inst), ov); err != nil {
		return err
	}
	d, err := diagnoseInstrumentOpts(os.Stderr, instrument.ID(*inst), *ref, *dur, ov)
	if err != nil {
		return err
	}
	if *asJSON {
		warnAblateIgnoredByJSON(os.Stderr, *ablate)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode([]instrumentDiagnosis{d})
	}
	printDiagnosis(os.Stdout, d)
	if *ablate {
		emitAblation(os.Stdout, os.Stderr, d, false, ov)
	}
	return nil
}
