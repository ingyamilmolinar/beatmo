//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// calibrate_thresholds.go — `refs calibrate-thresholds`: earn the drift and
// mismatch thresholds from the corpus instead of guessing them.
//
// The hand-written thresholds saturated (see internal/audio/fingerprint/
// thresholds.go for the numbers). This command replaces the guess with the
// only ground truth available: the families the listener graded "y". Those
// are, by definition, instruments whose distance from their reference is
// ACCEPTABLE, so their per-descriptor spread IS the threshold — a descriptor
// should stay quiet inside it and speak outside it.
//
// Percentiles rather than mean+k·sigma: these distributions are heavy-tailed
// and small (single-digit families per axis), so a mean is dragged by one
// outlier and a sigma is barely estimable. p50/p90 are rank statistics —
// they degrade gracefully and are meaningful at n=3.

// goodGroupCalibrationMinN is the smallest good-group sample an axis may be
// calibrated from.
//
// Five, not three. Even at the corpus's current 8 good families the p90 sits
// between the top two values; at n=3 it IS the maximum, so the "threshold"
// would be one family's worst reading with no evidence that anything else
// resembles it. An axis below the floor keeps its hand threshold, which was at
// least chosen by someone who knew what the units meant.
const goodGroupCalibrationMinN = 5

// axisMagnitudeFunc resolves one axis row to the magnitude its classifier
// actually compared against the drift/mismatch pair.
//
// It is a parameter rather than a hardcoded expression because that quantity
// is NOT the same thing on every axis (see registryAxisMagnitude), while the
// percentile statistics being calibrated are; separating them keeps the
// statistics unit-testable without a registry and a corpus.
type axisMagnitudeFunc func(axisResult) (float64, bool)

// registryAxisMagnitude is the production resolver.
//
// WHY NOT |Signed| (the obvious choice, and wrong). Signed is the raw
// difference in the descriptor's own units, but that is only the quantity the
// classifier compares for the absolute-delta kinds. compareScalar compares a
// KindSeconds/KindRatio axis as a FOLDED RATIO (>= 1 by construction) and a
// KindHz axis as a PERCENTAGE of the reference. Calibrating on |Signed|
// silently changes the unit: it produced drift=0.07 / mismatch=0.17 for
// `env.attack`, and a folded ratio below 1 means EVERY instrument reads
// MISMATCH forever; and mismatch=1314 for `bright.centroid`, which as a
// percentage means "never fires". Neither is visible in the JSON.
//
// WHY NOT Norm x mismatch either (the next obvious choice, also wrong). Norm
// is magnitude/mismatch in every DEFAULT comparator, so the product looks
// like it recovers the magnitude — and it does, until a comparator
// TRANSFORMS its thresholds. band.t60 declares T60 ratios and compares
// mean|log2 ratio|, so Norm x mismatch there is neither a ratio nor a log2.
//
// So the comparator that knows its own domain reports the magnitude itself,
// in the units the thresholds are declared in: fingerprint.Delta.Mag. The
// invariant that makes it trustworthy — classifying Mag against the
// descriptor's thresholds reproduces the comparator's verdict — is gated for
// every descriptor by fingerprint's TestDeltaMagReproducesTheVerdict.
//
// Rows refused rather than guessed:
//   - MagOK false: the "one side had no event at all" sentinel branches,
//     where Norm is set to 1 by fiat rather than measured;
//   - an unavailable measurement on either side;
//   - a Demoted descriptor, which is excluded from the trusted set anyway —
//     spending corpus evidence on thresholds nobody is allowed to believe
//     buys nothing, and leaves its documented hand values interpretable.
func registryAxisMagnitude(a axisResult) (float64, bool) {
	if a.ID == "" || a.RefNA != "" || a.SynthNA != "" || !a.MagOK {
		return 0, false
	}
	d, ok := fingerprint.DefaultRegistry().ByID(a.ID)
	if !ok || d.Demoted {
		return 0, false
	}
	if math.IsNaN(a.Mag) || math.IsInf(a.Mag, 0) || a.Mag < 0 {
		return 0, false
	}
	return a.Mag, true
}

// goodGroupThresholds computes, per descriptor ID, the p50/p90 of the
// compared magnitude over the families the listener graded "y".
//
// Axes magOf refuses (unavailable on either side, custom comparator, "n/a"
// sentinel) carry no usable magnitude and are skipped, so an axis that could
// not be measured on the good group is left at its hand threshold rather than
// calibrated from the handful of families where it happened to resolve.
func goodGroupThresholds(samples []validationSample, magOf axisMagnitudeFunc) fingerprint.ThresholdFile {
	byID := map[string][]float64{}
	for _, s := range samples {
		if s.Class != classGood {
			continue
		}
		for _, a := range s.Diagnosis.Axes {
			mag, ok := magOf(a)
			if !ok || math.IsNaN(mag) || math.IsInf(mag, 0) {
				continue
			}
			byID[a.ID] = append(byID[a.ID], math.Abs(mag))
		}
	}
	out := fingerprint.ThresholdFile{}
	for id, vs := range byID {
		if len(vs) < goodGroupCalibrationMinN {
			continue
		}
		sort.Float64s(vs)
		drift := fingerprint.Percentile(vs, 50)
		mismatch := fingerprint.Percentile(vs, 90)
		// A degenerate good group (every family identical on this axis)
		// collapses the two bands into one. Spreading them keeps "drift" and
		// "MISMATCH" distinguishable rather than making every flagged row a
		// mismatch.
		//
		// The spread is SYNTHESISED, not measured, so it is recorded in the
		// entry's Note. Source stays "corpus" -- the drift half really is the
		// corpus p50, and "hand"/"probe" mean pinned-against-recalibration,
		// which this is not -- but a reader comparing two corpus entries has
		// to be able to tell a p90 earned from eight families apart from a
		// 1.5x multiple of the p50, and previously could not.
		var note string
		if mismatch <= drift {
			mismatch = drift * 1.5
			note = fmt.Sprintf("p90 <= p50 over n=%d; mismatch synthesised as 1.5*drift", len(vs))
		}
		if mismatch <= 0 {
			continue
		}
		// A p50 of exactly zero (the good group matches the reference on this
		// axis more often than not) would leave Drift at 0, which
		// Descriptor.Thresholds reads as "no override" and silently falls
		// back to the KIND's drift — potentially larger than the calibrated
		// mismatch, inverting the two bands. Anchor it below mismatch
		// instead, keeping the same 1.5x separation the degenerate case uses.
		//
		// This is the OTHER synthesised half, and it needs saying for the
		// same reason: mode.harmonicity's 0.349/0.524 pair read like two
		// percentiles when the drift is a derived fraction of the mismatch
		// and no drift was measured at all.
		if drift <= 0 {
			drift = mismatch / 1.5
			note = fmt.Sprintf("p50 == 0 over n=%d; drift synthesised as mismatch/1.5", len(vs))
		}
		out[id] = fingerprint.ThresholdEntry{Drift: drift, Mismatch: mismatch, N: len(vs), Source: "corpus", Note: note}
	}
	return out
}

// mergeProbeEntries carries the PINNED entries of the existing file over a
// fresh calibration.
//
// Two sources are pinned. "probe" is a listener-measured audibility step from
// `refs probe` — a number an ear produced, which outranks a percentile over a
// corpus. "hand" is a deliberate exclusion: a descriptor whose calibrated
// value stopped cmd/refs's acceptance gate reproducing a listener complaint,
// pinned back to the value that does. Neither may be silently overwritten by
// re-running calibration, or the evidence would be lost the first time
// somebody regenerated the file.
func mergeProbeEntries(fresh, existing fingerprint.ThresholdFile) fingerprint.ThresholdFile {
	for id, e := range existing {
		if e.Source == "probe" || e.Source == "hand" {
			fresh[id] = e
		}
	}
	return fresh
}

// readThresholdFileAt loads the calibration that is ON DISK at path.
//
// Deliberately not fingerprint.LoadThresholdFile(): that returns the copy
// EMBEDDED in this binary, which is the state of the file at the last build.
// Pinning a hand exclusion is a hand edit followed by a recalibration, and
// reading the embedded copy would drop the edit unless the operator happened
// to rebuild in between — losing exactly the evidence the pin exists to
// preserve, silently. A missing file is not an error: the first run has
// nothing to carry over.
func readThresholdFileAt(path string) (fingerprint.ThresholdFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fingerprint.ThresholdFile{}, nil
		}
		return nil, err
	}
	out := fingerprint.ThresholdFile{}
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("refs calibrate-thresholds: parse %s: %w", path, err)
	}
	return out, nil
}

func defaultThresholdPath() (string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return "", fmt.Errorf("refs calibrate-thresholds: cannot locate the module root (no go.mod above cwd)")
	}
	return filepath.Join(root, "internal", "audio", "fingerprint", "testdata", "thresholds.json"), nil
}

func runCalibrateThresholds(w, diag io.Writer, verdictsPath, outPath string) error {
	verdicts, err := readVerdicts(verdictsPath)
	if err != nil {
		return err
	}
	samples := collectValidationSamples(diag, verdicts)
	if len(samples) == 0 {
		return fmt.Errorf("refs calibrate-thresholds: no family could be diagnosed from %s", verdictsPath)
	}
	var nGood int
	for _, s := range samples {
		if s.Class == classGood {
			nGood++
		}
	}
	fresh := goodGroupThresholds(samples, registryAxisMagnitude)
	if len(fresh) == 0 {
		return fmt.Errorf("refs calibrate-thresholds: no descriptor had %d good-group samples -- "+
			"refusing to write an empty calibration over the existing one", goodGroupCalibrationMinN)
	}
	existing, err := readThresholdFileAt(outPath)
	if err != nil {
		return err
	}
	fresh = mergeProbeEntries(fresh, existing)
	data, err := json.MarshalIndent(fresh, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	var nPinned int
	for _, e := range fresh {
		if e.Source != "corpus" {
			nPinned++
		}
	}
	fmt.Fprintf(w, "wrote %s: %d descriptors calibrated from the good group (%d pinned probe/hand entries carried over)\n",
		outPath, len(fresh)-nPinned, nPinned)
	fmt.Fprintf(w, "drift = p50, mismatch = p90 of the good group's per-axis magnitude. With only %d good\n", nGood)
	fmt.Fprintf(w, "families a p90 sits between the top two readings -- these are evidence-shaped estimates,\n")
	fmt.Fprintf(w, "not settled values; recompute as ear_verdicts.jsonl grows.\n")
	fmt.Fprintf(w, "the file is EMBEDDED -- rebuild refs before the new thresholds take effect\n")
	return nil
}

func cmdCalibrateThresholds(args []string) error {
	fs := flag.NewFlagSet("calibrate-thresholds", flag.ExitOnError)
	verdicts := fs.String("verdicts", "", "verdicts jsonl (default: ear_verdicts.jsonl)")
	out := fs.String("out", "", "output JSON (default: internal/audio/fingerprint/testdata/thresholds.json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	vp := *verdicts
	if vp == "" {
		var err error
		if vp, err = defaultVerdictsPath(); err != nil {
			return err
		}
	}
	op := *out
	if op == "" {
		var err error
		if op, err = defaultThresholdPath(); err != nil {
			return err
		}
	}
	return runCalibrateThresholds(os.Stdout, os.Stderr, vp, op)
}
