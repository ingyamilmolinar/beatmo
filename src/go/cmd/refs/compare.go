//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// -match modes.
const (
	matchRMS  = "rms"
	matchPeak = "peak"
	matchNone = "none"
)

// clipCeiling is how close to full scale a level-matched clip may land.
// Comfortably below 1.0 so a level-matching gain can never itself introduce
// clipping.
const clipCeiling = 0.99

// silenceRMSThreshold marks a render as "no audio", not merely "quiet
// audio" -- worth a loud warning rather than a silently-written, baffling
// WAV file the user plays and can't explain.
const silenceRMSThreshold = 1e-6

// compareDir is the review directory for one instrument's REAL-vs-SYNTH
// comparison audio. Lives under $BEATMO_WAV_REFS, mirroring candidateDir,
// so comparison audio never lands in the tree.
func compareDir(inst instrument.ID) (string, error) {
	base, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for comparison review audio", wavref.EnvVar)
	}
	d := filepath.Join(base, "compare", string(inst))
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// pickReference chooses which of an instrument's candidate references
// (already status-filtered and ID-sorted by the caller) to compare against.
// wantID == "" picks the first -- the caller is responsible for telling the
// user which one that was and that -ref can pick another.
func pickReference(candidates []wavref.Ref, wantID string) (wavref.Ref, error) {
	if wantID == "" {
		return candidates[0], nil
	}
	for _, r := range candidates {
		if r.ID == wantID {
			return r, nil
		}
	}
	return wavref.Ref{}, fmt.Errorf("refs compare: %q is not a reference for this instrument", wantID)
}

// computeGains returns the pair of gains that bring levels a and b to their
// common average: a*realGain == b*synthGain == target. That's what
// "equalised" means here. A zero level means there is no ratio to compute
// against silence, so unity gain (leave it alone) is the only sane answer.
func computeGains(a, b float64) (float64, float64) {
	if a <= 0 || b <= 0 {
		return 1, 1
	}
	target := (a + b) / 2
	return target / a, target / b
}

// clampGainsForPeak scales BOTH gains down by the same factor whenever
// either resulting peak would exceed ceiling, so the two clips' relative
// loudness -- the whole point of level-matching -- survives the clamp.
func clampGainsForPeak(realGain, synthGain, realPeak, synthPeak, ceiling float64) (float64, float64) {
	worst := math.Max(realPeak*realGain, synthPeak*synthGain)
	if worst <= ceiling || worst <= 0 {
		return realGain, synthGain
	}
	scale := ceiling / worst
	return realGain * scale, synthGain * scale
}

// levelMatchGains is the -match dispatcher. rms/peak equalise the named
// metric across the two clips (subject to the clip guard); none leaves both
// clips exactly as recorded/rendered.
func levelMatchGains(mode string, realRMS, realPeak, synthRMS, synthPeak float64) (realGain, synthGain float64, err error) {
	switch mode {
	case matchNone:
		return 1, 1, nil
	case matchRMS:
		realGain, synthGain = computeGains(realRMS, synthRMS)
	case matchPeak:
		realGain, synthGain = computeGains(realPeak, synthPeak)
	default:
		return 0, 0, fmt.Errorf("refs compare: unknown -match mode %q (want rms|peak|none)", mode)
	}
	realGain, synthGain = clampGainsForPeak(realGain, synthGain, realPeak, synthPeak, clipCeiling)
	return realGain, synthGain, nil
}

// computeGainsN generalises computeGains from one pair to a reference plus N
// synth options: every clip is brought to their COMMON average, so all N+1 sit
// at the same level.
//
// levels[0] is the reference; the rest are the options. For N==1 this reduces
// EXACTLY to computeGains -- target = (a+b)/2 -- which is what keeps
// `refs compare` and `refs diagnose` byte-identical while `refs audition`
// gains a family-wide match. A zero level has no ratio to compute against
// silence, so it keeps unity gain and is excluded from the target.
func computeGainsN(levels []float64) []float64 {
	gains := make([]float64, len(levels))
	var sum float64
	var n int
	for _, l := range levels {
		if l > 0 {
			sum += l
			n++
		}
	}
	if n == 0 {
		for i := range gains {
			gains[i] = 1
		}
		return gains
	}
	target := sum / float64(n)
	for i, l := range levels {
		if l <= 0 {
			gains[i] = 1
			continue
		}
		gains[i] = target / l
	}
	return gains
}

// clampFamilyGains is clampGainsForPeak over N+1 clips: when ANY resulting
// peak would exceed ceiling, EVERY gain is scaled down by the same factor, so
// the equal-loudness relationship the matching just established survives the
// clamp. Scaling only the offending clip would reintroduce exactly the level
// difference this whole path exists to remove.
func clampFamilyGains(gains, peaks []float64, ceiling float64) []float64 {
	var worst float64
	for i := range gains {
		if i >= len(peaks) {
			break
		}
		if p := peaks[i] * gains[i]; p > worst {
			worst = p
		}
	}
	if worst <= ceiling || worst <= 0 {
		return gains
	}
	scale := ceiling / worst
	out := make([]float64, len(gains))
	for i, g := range gains {
		out[i] = g * scale
	}
	return out
}

// familyLevelGains is levelMatchGains for a whole family: one reference plus N
// synth options, returned as N+1 gains with rms[0]/peaks[0] being the
// reference's.
func familyLevelGains(mode string, rms, peaks []float64) ([]float64, error) {
	switch mode {
	case matchNone:
		out := make([]float64, len(rms))
		for i := range out {
			out[i] = 1
		}
		return out, nil
	case matchRMS:
		return clampFamilyGains(computeGainsN(rms), peaks, clipCeiling), nil
	case matchPeak:
		return clampFamilyGains(computeGainsN(peaks), peaks, clipCeiling), nil
	default:
		return nil, fmt.Errorf("refs compare: unknown -match mode %q (want rms|peak|none)", mode)
	}
}

func scaleSamples(samples []float64, gain float64) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = s * gain
	}
	return out
}

// isNearSilent reports whether samples carry essentially no signal -- an
// all-zero or near-zero render, not merely a quiet one.
func isNearSilent(samples []float64) bool {
	if len(samples) == 0 {
		return true
	}
	var sumsq float64
	for _, s := range samples {
		sumsq += s * s
	}
	rms := math.Sqrt(sumsq / float64(len(samples)))
	return rms < silenceRMSThreshold
}

// writeCompareWAV encodes samples via the app's own WAV encoder (the same
// one internal/wavgen wraps for tests/tools) so the output is a real,
// spec-conformant WAV usable everywhere -- including under `-tags test`,
// where internal/audio's desktop-only ExportCaptureToWAV is unavailable.
func writeCompareWAV(path string, samples []float64, sr int) error {
	f32 := make([]float32, len(samples))
	for i, s := range samples {
		f32[i] = float32(s)
	}
	b, err := wavgen.EncodeWAV(f32, sr)
	if err != nil {
		return fmt.Errorf("refs compare: encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("refs compare: write %s: %w", path, err)
	}
	return nil
}

// compareCandidates returns inst's non-status (real-audio) reference
// candidates, sorted by id -- the exact set and ordering computeCompare and
// `-list` both pick the default reference from.
func compareCandidates(c wavref.Cat, inst instrument.ID) []wavref.Ref {
	var out []wavref.Ref
	for _, r := range c.ForInstrument(inst) {
		if r.Status == "" {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// comparableInstruments returns every distinct instrument id with at least
// one non-status (real-audio) reference candidate -- i.e. everything
// `refs compare -instrument` can legally be pointed at. This is the single
// source of truth for "what can I pass to -instrument"; don't reimplement
// this enumeration elsewhere (including in the Makefile).
func comparableInstruments(c wavref.Cat) []instrument.ID {
	set := map[instrument.ID]bool{}
	for _, r := range c.Refs {
		if r.Status != "" {
			continue
		}
		for _, inst := range r.Instruments {
			set[inst] = true
		}
	}
	out := make([]instrument.ID, 0, len(set))
	for inst := range set {
		out = append(out, inst)
	}
	slices.Sort(out)
	return out
}

// compareResult carries the full pipeline output that both the
// human-readable report (runCompare) and the machine-readable `-sh` output
// (runCompareSh) are printed from -- one computation path, two renderings.
type compareResult struct {
	Match string
	Ref   wavref.Ref
	Dur   float64
	// Pitch is the semitone offset from A3 the synth was actually rendered at.
	// Reported because it is no longer simply the reference's stored integer:
	// a reliable f0 renders at its exact fractional value, and the listener
	// should be able to see which of the two happened.
	Pitch      float64
	PitchExact bool
	RealPath   string
	SynthPath  string
	RealM      wavref.Measured
	SynthM     wavref.Measured
	RealGain   float64
	SynthGain  float64
}

// computeCompare renders the synth at a reference's own measured pitch --
// upstream pitch labels have been wrong repeatedly, so the reference's own
// `measured.semis_from_a3` is the only trustworthy render target -- then
// level-matches the two clips and writes both to disk for an A/B listen.
// Level-matching matters more than it looks: in any A/B test the louder
// source is reliably judged fuller/better, so without it the listener
// grades gain, not timbre.
//
// diag receives progress/warning messages that are advisory rather than
// part of the final answer (the multi-candidate note, the near-silent-render
// warning). runCompare routes diag to the same writer as the report so
// nothing is lost; runCompareSh routes it to stderr so stdout carries
// nothing but the four `-sh` shell assignments.
func computeCompare(diag io.Writer, inst instrument.ID, refID string, durFlag float64, match string) (compareResult, error) {
	if match != matchRMS && match != matchPeak && match != matchNone {
		return compareResult{}, fmt.Errorf("refs compare: unknown -match mode %q (want rms|peak|none)", match)
	}

	c, err := loadCatalogue()
	if err != nil {
		return compareResult{}, err
	}
	candidates := compareCandidates(c, inst)
	if len(candidates) == 0 {
		return compareResult{}, fmt.Errorf("refs compare: no reference audio for instrument %q", inst)
	}

	ref, err := pickReference(candidates, refID)
	if err != nil {
		return compareResult{}, fmt.Errorf("refs compare: %s is not a reference for instrument %q", refID, inst)
	}
	if refID == "" && len(candidates) > 1 {
		fmt.Fprintf(diag, "using %s (1 of %d candidates for %s; pass -ref to compare against another)\n\n",
			ref.ID, len(candidates), inst)
	}

	return renderCompare(diag, inst, ref, durFlag, match)
}

// renderPitch is the semitone offset (from A3=220Hz) to render a synth at so
// it matches a reference's ACTUAL pitch.
//
// The reference's own measured f0 is used directly, NOT the rounded
// measured.semis_from_a3, because that rounding detunes the synth by up to
// half a semitone against the very recording it is being compared to. Across
// the corpus the median error is 11 cents but fifteen references exceed a
// quarter-tone and six exceed 40 cents (koto +43.9, piano-felt -37.6,
// singing-bowl +35.0) -- plainly audible as "out of tune" in an A/B listen,
// and a systematic bias on every frequency-scaled diagnostic axis. Measured
// already carries F0Hz; only the fractional part was being discarded.
//
// An unreliable f0 (percussion, where the detector's number is an artifact of
// the octave guard rather than a pitch) falls back to the stored integer: the
// exact value is meaningless either way there, and the stored one keeps those
// renders reproducible against earlier verdicts.
func renderPitch(m wavref.Measured, f0Reliable bool) float64 {
	if m.F0Hz > 0 && f0Reliable {
		return 12 * math.Log2(m.F0Hz/a3Hz)
	}
	return float64(m.SemisFromA3)
}

// renderCompare renders ONE instrument against ref. It is the single-option
// case of renderFamilyCompare, kept as a named entry point because
// computeCompare, `refs diagnose` and the acceptance test all drive exactly
// one instrument.
func renderCompare(diag io.Writer, inst instrument.ID, ref wavref.Ref, durFlag float64, match string) (compareResult, error) {
	res, err := renderFamilyCompare(diag, []instrument.ID{inst}, ref, durFlag, match)
	if err != nil {
		return compareResult{}, err
	}
	return res[0], nil
}

// renderCompareOverrides is renderCompare's param-override variant: ov's
// values render on top of inst's shipped defaults (see
// synthmatch.RenderWithParams), for diagnosing a session candidate's clip
// rather than the instrument's own shipped sound.
func renderCompareOverrides(diag io.Writer, inst instrument.ID, ov map[string]float64, ref wavref.Ref, durFlag float64, match string) (compareResult, error) {
	res, err := renderFamilyCompareOverrides(diag, []instrument.ID{inst}, []map[string]float64{ov}, ref, durFlag, match)
	if err != nil {
		return compareResult{}, err
	}
	return res[0], nil
}

// renderFamilyCompare renders EVERY instrument in insts against ONE shared
// reference, level-matches all of them together, and writes one reference WAV
// plus one synth WAV per instrument. It is renderFamilyCompareOverrides with
// no per-instrument param overrides.
func renderFamilyCompare(diag io.Writer, insts []instrument.ID, ref wavref.Ref, durFlag float64, match string) ([]compareResult, error) {
	return renderFamilyCompareOverrides(diag, insts, nil, ref, durFlag, match)
}

// renderFamilyCompareOverrides is renderFamilyCompare's param-override
// variant: overrides[i] (may be nil, and the whole slice may be nil) renders
// on top of insts[i]'s shipped defaults via synthmatch.RenderShipped — the
// sound as PLAYED (FX, gate, normalisation; percussion pitched by resampling)
// — rather than the instrument's plain shipped sound. This is what lets a
// session candidate -- an instrument plus a proposed knob tweak -- be
// diagnosed and A/B'd exactly like a shipped instrument, without a second
// synth-rendering code path.
//
// Rendering the family as a set rather than one instrument at a time is not an
// optimisation, it is a correctness requirement. Level-matching derives the
// reference's gain from the synth it is paired with, so N independent
// renderCompare calls produced N DIFFERENTLY-SCALED copies of the same
// reference -- and `refs audition` played only the first, leaving options 2..N
// matched against a clip the listener never heard. A family whose options
// differ in raw level by 4x therefore presented them ~6 dB apart, and in any
// A/B the louder source is reliably judged fuller. Every result returned here
// shares one RealPath and one common level.
//
// The reference is decoded, measured and written ONCE for the whole family
// rather than once per member, which also drops N-1 decode/encode round trips.
//
// The WAV filename for an override render carries the first 8 hex chars of
// audio.InstrumentSoundHash(inst, ov) so a candidate's render never
// overwrites (or is confused with) the instrument's plain SYNTH__<inst>.wav.
func renderFamilyCompareOverrides(diag io.Writer, insts []instrument.ID, overrides []map[string]float64, ref wavref.Ref, durFlag float64, match string) ([]compareResult, error) {
	specs := make([]compareSpec, len(insts))
	for i, inst := range insts {
		specs[i].Inst = inst
		if overrides != nil {
			specs[i].Overrides = overrides[i]
		}
	}
	return renderFamilyCompareSpecs(diag, specs, ref, durFlag, match)
}

// compareSpec is one option of a level-matched family render: a shipped
// instrument (+ overrides), or a whole document that is registered nowhere —
// a WAV-clone candidate, rendered at its own fit pitch when Pitch is set.
type compareSpec struct {
	Inst      instrument.ID
	Overrides map[string]float64
	Doc       *instrument.Doc
	DocSHA    string
	Pitch     *float64
}

// renderFamilyCompareSpecs renders every option against ONE reference copy
// (renderFamilyCompareOverrides is the overrides-only form).
func renderFamilyCompareSpecs(diag io.Writer, specs []compareSpec, ref wavref.Ref, durFlag float64, match string) ([]compareResult, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("refs compare: no instruments to render")
	}
	insts := make([]instrument.ID, len(specs))
	for i, sp := range specs {
		insts[i] = sp.Inst
	}
	if ref.Measured == nil {
		return nil, fmt.Errorf("refs compare: %s has no measured pitch yet; run `refs fetch -id %s` first", ref.ID, ref.ID)
	}

	path, ok := wavref.Resolve(ref.ID)
	if !ok {
		return nil, fmt.Errorf("refs compare: audio for %s is not available locally; run `refs fetch -id %s` first", ref.ID, ref.ID)
	}
	samples, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
	if err != nil {
		return nil, fmt.Errorf("refs compare: decode %s: %w", path, err)
	}
	realSamples := make([]float64, len(samples))
	for i, s := range samples {
		realSamples[i] = float64(s)
	}
	realWave := wave.Wave{Samples: realSamples, SampleRate: rate, Label: ref.ID}
	realMeasured := measureWave(realWave)

	// Render every member at the reference's OWN pitch -- that's the entire
	// point of this command.
	dur := durFlag
	if dur <= 0 {
		dur = ref.Measured.DurationSec
	}
	f0 := fingerprint.DetectF0Detailed(realWave)
	pitch := renderPitch(*ref.Measured, f0.Reliable())
	sr := wavref.Rate

	synthWaves := make([]wave.Wave, len(insts))
	synthMeasured := make([]wavref.Measured, len(insts))
	for i, inst := range insts {
		sp := specs[i]
		// The sound as PLAYED (document FX, gate, normalisation; percussion
		// pitched by resampling) — the raw C render skipped the FX of every
		// document that carries them, so compare analysed a sound nobody hears.
		var w wave.Wave
		var err error
		if sp.Doc != nil {
			p := pitch
			if sp.Pitch != nil {
				p = *sp.Pitch
			}
			w, err = synthmatch.RenderShippedDoc(*sp.Doc, p, sr, dur)
		} else {
			w, err = synthmatch.RenderShipped(inst, pitch, sr, dur, sp.Overrides)
		}
		if err != nil {
			return nil, fmt.Errorf("refs compare: render %s at %+.2f semis: %w", inst, pitch, err)
		}
		synthWaves[i] = w
		synthMeasured[i] = measureWave(w)
		if isNearSilent(w.Samples) {
			warnNearSilent(diag, inst, synthMeasured[i])
		}
	}

	// One level plan for the whole set: index 0 is the reference.
	rmsLevels := make([]float64, 0, len(insts)+1)
	peakLevels := make([]float64, 0, len(insts)+1)
	rmsLevels = append(rmsLevels, realMeasured.RMS)
	peakLevels = append(peakLevels, realMeasured.Peak)
	for _, m := range synthMeasured {
		rmsLevels = append(rmsLevels, m.RMS)
		peakLevels = append(peakLevels, m.Peak)
	}
	gains, err := familyLevelGains(match, rmsLevels, peakLevels)
	if err != nil {
		return nil, err
	}

	// The reference lands in the FIRST member's directory and every result
	// points at that one file, so the clip the listener hears is the clip every
	// option was matched against.
	refDir, err := compareDir(insts[0])
	if err != nil {
		return nil, err
	}
	realPath := filepath.Join(refDir, fmt.Sprintf("REAL__%s.wav", ref.ID))
	if err := writeCompareWAV(realPath, scaleSamples(realWave.Samples, gains[0]), rate); err != nil {
		return nil, err
	}
	realPath = absOrSelf(realPath)

	out := make([]compareResult, len(insts))
	for i, inst := range insts {
		dir, err := compareDir(inst)
		if err != nil {
			return nil, err
		}
		ov := specs[i].Overrides
		synthName := fmt.Sprintf("SYNTH__%s.wav", inst)
		if specs[i].Doc != nil {
			synthName = fmt.Sprintf("SYNTH__%s__clone-%s.wav", inst, strings.TrimPrefix(specs[i].DocSHA, "sha256:"))
		} else if ov != nil {
			hash := audio.InstrumentSoundHash(inst, ov)
			if len(hash) > 8 {
				hash = hash[:8]
			}
			synthName = fmt.Sprintf("SYNTH__%s__%s.wav", inst, hash)
		}
		synthPath := filepath.Join(dir, synthName)
		if err := writeCompareWAV(synthPath, scaleSamples(synthWaves[i].Samples, gains[i+1]), sr); err != nil {
			return nil, err
		}
		out[i] = compareResult{
			Match:      match,
			Ref:        ref,
			Dur:        dur,
			Pitch:      pitch,
			PitchExact: f0.Reliable() && ref.Measured.F0Hz > 0,
			RealPath:   realPath,
			SynthPath:  absOrSelf(synthPath),
			RealM:      realMeasured,
			SynthM:     synthMeasured[i],
			RealGain:   gains[0],
			SynthGain:  gains[i+1],
		}
	}
	return out, nil
}

// warnNearSilent explains a silent render, including the build-tag caveat that
// makes silence expected (and meaningless) under `-tags test`.
func warnNearSilent(diag io.Writer, inst instrument.ID, m wavref.Measured) {
	fmt.Fprintf(diag, "WARNING: the synthesized %s render is silent or near-silent (RMS=%.6f, peak=%.6f).\n",
		inst, m.RMS, m.Peak)
	fmt.Fprintln(diag, "Writing it anyway so you can inspect it, but don't trust the comparison below.")
	fmt.Fprintln(diag, "synthmatch's renderer is build-tag split: under `-tags test` it is a stub, so silence")
	fmt.Fprintln(diag, "there is EXPECTED and meaningless; under a real CGO_ENABLED=1 build, silence means the")
	fmt.Fprintln(diag, "instrument's recipe produced no audio at this pitch -- investigate before trusting this.")
	fmt.Fprintln(diag)
}

// absOrSelf makes a path absolute, falling back to the original on failure.
// `-sh`'s whole point is to be eval'd from an arbitrary cwd (the Makefile's own
// shell), so a relative path would silently resolve against the wrong
// directory.
func absOrSelf(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// printCompareReport writes the human-readable comparison table -- pitch,
// duration, peak/RMS, applied gain -- plus the file paths and listening
// guidance.
func printCompareReport(w io.Writer, res compareResult) error {
	switch res.Match {
	case matchNone:
		fmt.Fprintln(w, "Levels NOT matched (-match none): both clips written exactly as recorded/rendered.")
	default:
		fmt.Fprintf(w, "Levels equalised by %s (target = average of the two clips' %s) so an A/B listen judges\n",
			res.Match, strings.ToUpper(res.Match))
		fmt.Fprintln(w, "timbre, not loudness -- the louder clip is reliably judged fuller/better otherwise, and")
		fmt.Fprintln(w, "this corpus's own reference RMS values already vary roughly 2x instrument to instrument.")
	}
	fmt.Fprintln(w)

	if res.PitchExact {
		fmt.Fprintf(w, "Synth rendered at %+.2f semis from A3 (%.2f Hz) -- the reference's EXACT measured\n",
			res.Pitch, 220*math.Pow(2, res.Pitch/12))
		fmt.Fprintf(w, "pitch, not its rounded %+d, so the two clips are in tune with each other.\n\n",
			res.Ref.Measured.SemisFromA3)
	} else {
		fmt.Fprintf(w, "Synth rendered at %+.0f semis from A3 (the stored value): this reference has no\n", res.Pitch)
		fmt.Fprintln(w, "reliable fundamental, so there is no exact pitch to match.")
		fmt.Fprintln(w)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "\tF0 (Hz)\tSEMIS\tDUR (s)\tPEAK (pre)\tRMS (pre)\tGAIN")
	fmt.Fprintf(tw, "REAL\t%.1f\t%+d\t%.2f\t%.3f\t%.3f\t%.3f\n",
		res.RealM.F0Hz, res.RealM.SemisFromA3, res.RealM.DurationSec, res.RealM.Peak, res.RealM.RMS, res.RealGain)
	fmt.Fprintf(tw, "SYNTH\t%.1f\t%+d\t%.2f\t%.3f\t%.3f\t%.3f\n",
		res.SynthM.F0Hz, res.SynthM.SemisFromA3, res.SynthM.DurationSec, res.SynthM.Peak, res.SynthM.RMS, res.SynthGain)
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintf(w, "\n  real:  %s\n  synth: %s\n", res.RealPath, res.SynthPath)
	fmt.Fprintln(w, "\nWhat to listen for: attack (pluck/bow/breath onset), vibrato or pitch steadiness,")
	fmt.Fprintln(w, "brightness (high-frequency content), noise/breath character (bow hiss, reed buzz, air),")
	fmt.Fprintln(w, "and decay/release shape.")
	return nil
}

// formatCompareSh renders the `-sh` output: four single-quoted, eval-able
// shell assignments and nothing else. Quoting goes through shQuote, which also
// escapes an embedded single quote -- a bare '%s' wrapper is safe for spaces
// but a path containing an apostrophe would close the string and hand the
// remainder to the shell.
func formatCompareSh(realPath, synthPath, refID string, dur float64) string {
	return fmt.Sprintf(
		"BEATMO_AB_REAL=%s\nBEATMO_AB_SYNTH=%s\nBEATMO_AB_DUR=%s\nBEATMO_AB_REF=%s\n",
		shQuote(realPath), shQuote(synthPath), shQuote(fmt.Sprintf("%.2f", dur)), shQuote(refID))
}

// runCompare runs the full render + level-match pipeline and prints the
// human-readable report.
func runCompare(w io.Writer, inst instrument.ID, refID string, durFlag float64, match string) error {
	res, err := computeCompare(w, inst, refID, durFlag, match)
	if err != nil {
		return err
	}
	return printCompareReport(w, res)
}

// runCompareSh runs the IDENTICAL render + level-match pipeline as
// runCompare (via computeCompare -- `-sh` never skips the work, only the
// reporting) but SPLITS the two reports across streams instead of doing a
// second full render: the human-readable table goes to diag (stderr in
// practice), and w (stdout) carries ONLY the four BEATMO_AB_* shell
// assignments. That's the standard split for a command whose output is
// meant to be captured -- `eval "$(refs compare -sh ...)"` captures stdout
// while the table flows to the terminal via stderr, unimpeded, from the
// SAME invocation. Earlier this command's only wiring was "run compare for
// the report, then run it again with -sh for the paths" -- two full synth
// renders and two catalogue loads per listen for no benefit; this is the
// one-invocation replacement.
func runCompareSh(w, diag io.Writer, inst instrument.ID, refID string, durFlag float64, match string) error {
	res, err := computeCompare(diag, inst, refID, durFlag, match)
	if err != nil {
		return err
	}
	if err := printCompareReport(diag, res); err != nil {
		return err
	}
	fmt.Fprint(w, formatCompareSh(res.RealPath, res.SynthPath, res.Ref.ID, res.Dur))
	return nil
}

// runCompareList prints every instrument comparable via `refs compare
// -instrument` -- the single source of truth for "what can I pass to
// -instrument". Callers (including `make ab-list`) must delegate here
// rather than reimplementing the enumeration.
func runCompareList(w io.Writer) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	insts := comparableInstruments(c)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "INSTRUMENT\tREF\tSEMIS\tALT")
	for _, inst := range insts {
		candidates := compareCandidates(c, inst)
		ref, err := pickReference(candidates, "")
		if err != nil {
			return err
		}
		semis := "-"
		if ref.Measured != nil {
			semis = fmt.Sprintf("%+d", ref.Measured.SemisFromA3)
		}
		alt := ""
		if len(candidates) > 1 {
			alt = "*"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", inst, ref.ID, semis, alt)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d instrument(s) comparable via `refs compare -instrument <id>` (* = multiple candidates; pass -ref to pick another)\n",
		len(insts))
	return nil
}

func cmdCompare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to compare")
	ref := fs.String("ref", "", "reference id to compare against (default: first candidate)")
	dur := fs.Float64("dur", 0, "clip duration in seconds (default: the reference's measured duration)")
	match := fs.String("match", matchRMS, "level-matching mode: rms|peak|none")
	list := fs.Bool("list", false, "list every instrument comparable via -instrument, then exit")
	sh := fs.Bool("sh", false, "print machine-readable BEATMO_AB_* shell assignments instead of the report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *list {
		return runCompareList(os.Stdout)
	}
	if *inst == "" {
		return fmt.Errorf("refs compare: -instrument is required")
	}
	if *sh {
		return runCompareSh(os.Stdout, os.Stderr, instrument.ID(*inst), *ref, *dur, *match)
	}
	return runCompare(os.Stdout, instrument.ID(*inst), *ref, *dur, *match)
}
