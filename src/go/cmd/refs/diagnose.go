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

// Per-axis on purpose, the opposite of synthmatch's Distance(): one weighted scalar lets a 7x decay error and a small
// spectral gain cancel out. Every axis is reported separately and signed (synth - reference), on exactly the clips a
// listener would A/B (computeCompare), through the fingerprint analyzers the game uses.

// The vocabularies live in internal/audio/fingerprint, which owns classification; these aliases keep this file's spellings.
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
	// The stable machine key, so callers never key on the Group/Name wording.
	ID    string   `json:"id,omitempty"`
	Group string   `json:"group"`
	Name  string   `json:"axis"`
	Kind  axisKind `json:"-"`
	Unit  string   `json:"unit,omitempty"`

	// The comparator's own rendering ("7.1x short", "+36%"), stored so formatting and verdict have ONE source.
	Delta string  `json:"delta,omitempty"`
	Ref   float64 `json:"reference"`
	Synth float64 `json:"synth"`
	// Scale-normalised: >= 1 means at or past this axis's mismatch threshold. The only quantity comparable ACROSS axes
	// (what refs validate correlates); never summed into a composite.
	Norm float64 `json:"norm"`
	// The raw signed difference in the descriptor's units, for callers that need the value rather than a ratio.
	Signed float64 `json:"signed"`

	// The UNSIGNED magnitude the comparator weighed, in the units its thresholds are declared in, and whether it is a
	// measurement at all: what refs calibrate-thresholds takes percentiles of.
	Mag   float64 `json:"mag"`
	MagOK bool    `json:"mag_ok"`

	Verdict axisVerdict `json:"verdict"`
	Note    string      `json:"note"`

	// The delta's sign in axis terms ("too short", "too low"), so callers assert direction without re-deriving it.
	Direction string `json:"direction"` // "high" | "low" | "" (ok / undefined)

	// Why a side could not be measured, so the report prints "n/a" rather than a zero, and an ASYMMETRY (a reference that
	// decays where the synth sustains) is reported as the finding it is.
	RefNA   string `json:"ref_na,omitempty"`
	SynthNA string `json:"synth_na,omitempty"`

	// Still measured and printed, tagged "[demoted]", but excluded from every ranking (stage routing, the MISMATCH matrix):
	// a row calibration showed to mislead must not send someone to a knob.
	Demoted bool `json:"demoted,omitempty"`

	// Headline text for a measurement whose Primary() would mislead (an all-peer vector, a track); it REPLACES the
	// numeric column while Ref/Synth keep their raw values.
	RefText   string `json:"ref_text,omitempty"`
	SynthText string `json:"synth_text,omitempty"`
}

// Above KindSeriesDeviation's mismatch, the reference's partials are far from a harmonic series, so NoiseRatio's
// harmonic-bin assumption no longer holds and the noise axes get a caveat.
var seriesDevCaveatTh = func() float64 {
	_, mismatch := fingerprint.KindSeriesDeviation.Thresholds()
	return mismatch
}()

// With BOTH clips ringing above -60 dB at their end the T60s are clamped bounds, so the comparator escalated to the
// post-peak decay RATE; with one side clamped the ratio still carries signal and the note names the bound.
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

// An ASYMMETRY is itself a finding: "the synth sustains where the reference does not" is the defect, so it is stated,
// not flattened into "not measurable".
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

	// The EXACT render this diagnosis describes, so -ablate reproduces it: re-deriving would drift the moment renderPitch
	// or the -dur default changes, ablating a different render than the one reported.
	Pitch  float64 `json:"pitch,omitempty"`
	DurSec float64 `json:"dur_sec,omitempty"`
	SR     int     `json:"sample_rate,omitempty"`

	// The raw partial-ratio lists side by side ("ref 1.00, 4.02, 9.15 / synth 1.00, 2.00, 3.00") show a STRUCTURE
	// difference no scalar can.
	RefPartialRatios   []float64 `json:"ref_partial_ratios,omitempty"`
	SynthPartialRatios []float64 `json:"synth_partial_ratios,omitempty"`

	// Nonzero when the reference was truncated at fetch time (at convert.max_sec), so a decay axis may see less of the
	// instrument than exists.
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

// An explicit refID resolves through the bare catalogue lookup, not computeCompare's ownership gate: the acceptance
// test pins the EXACT reference a listener judged, which can be a shared family reference. "" takes computeCompare.
func diagnoseInstrument(diag io.Writer, inst instrument.ID, refID string) (instrumentDiagnosis, error) {
	return diagnoseInstrumentDur(diag, inst, refID, 0)
}

// durFlag > 0 lengthens only the SYNTH render: the reference is whatever the corpus clip holds, so this shows how the
// synth behaves past it when the reference's ring outlasts its clip.
func diagnoseInstrumentDur(diag io.Writer, inst instrument.ID, refID string, durFlag float64) (instrumentDiagnosis, error) {
	return diagnoseInstrumentOpts(diag, inst, refID, durFlag, nil)
}

// overrides render on top of inst's document, so a session candidate is diagnosed like a shipped instrument. With
// refID "" and overrides set, the instrument's first own candidate reference is used, as computeCompare defaults.
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

	// Each shared artifact (fingerprint, envelope, STFTs) is computed once per report.
	refB := fingerprint.NewBundle(refWave, res.Ref.ID)
	synthB := fingerprint.NewBundle(synthWave, string(inst))
	refFp := refB.Fingerprint()
	synthFp := synthB.Fingerprint()

	out := instrumentDiagnosis{Instrument: inst, RefID: res.Ref.ID, Pitch: res.Pitch, DurSec: res.Dur, SR: wavref.Rate}
	if maxSec := refConvertMaxSec(res.Ref); maxSec > 0 && res.RealM.DurationSec >= maxSec-1e-6 {
		out.RefClipTruncatedSec = maxSec
	}

	// Every axis comes from the descriptor registry; adding one is a registry row. Applicability is judged on the
	// REFERENCE, which defines the instrument: a synth that failed to sustain must not suppress the sustain axes.
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

	// NoiseRatio counts energy off an ASSUMED harmonic series as noise, wrong for a bell whose tonal partials sit
	// elsewhere: tubular bells read noise 0.875 with flatness 0.030, a contradiction that is the tell.
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
	// A rate (Hz/s, dB/s) must not print with its Kind's bare "Hz".
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

// A vector or track prints what it IS ("(vector: 6/10 bands measurable)"); n/a still wins.
func formatValueText(a axisResult, v float64, na, text string) string {
	if na != "" {
		return "n/a"
	}
	if text != "" {
		return text
	}
	return formatValue(a, v, na)
}

// The demotion says "do not RANK on this", not "meaningless", so the real verdict shows with the tag.
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

// Printed beside the table, where a structure difference is legible at a glance.
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

// The matrix has no JSON shape, and a flag that exits 0 having done nothing sends people hunting bugs in the analysis.
func warnAblateIgnoredByJSON(diag io.Writer, ablate bool) {
	if !ablate {
		return
	}
	fmt.Fprintln(diag, "refs diagnose: -ablate is a report-only addendum; ignored with -json")
}

// Only MISMATCH rows (the full matrix is -all -ablate's), and an axis no stage moves is reported, not omitted: the gap
// is then engine-level, a missing model, not a tuning job.
func printAblation(w io.Writer, d instrumentDiagnosis, rep synthdiag.Report) {
	fmt.Fprintln(w, "\n-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tTOP STAGES (\u0394norm above the no-effect floor when the stage is at identity)")
	for _, a := range d.Axes {
		// A routing table must not send anyone to a stage on an axis calibration showed to mislead; it stays tagged above.
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

// -all -ablate asks whether one stage dominates one axis everywhere, which needs every cell.
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

// printSummaryMatrix shows only the MISMATCHes, instrument × axis (columns in first-seen order): the cross-instrument view.
func printSummaryMatrix(w io.Writer, all []instrumentDiagnosis) {
	// A demoted axis gets no column: it misleads on every instrument, and a column of MISMATCHes would read as the
	// strongest pattern on the page.
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

// A failure goes to diag without failing the diagnosis: the stage routing is an addendum.
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

// The CLI door onto the overrides, so a candidate tweak is diagnosed without editing a document; "" = nil = the shipped
// sound.
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

// An unknown key would be silently dropped by the render, measuring the shipped sound while the operator believes it
// is the candidate. Known = the instrument's merged params.
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
