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

// Earns the drift / mismatch thresholds from the only ground truth there is: the families graded "y" are ACCEPTABLY
// far from their reference, so their per-descriptor spread IS the threshold. Percentiles, not mean+k·sigma: the
// distributions are heavy-tailed and tiny, and p50/p90 stay meaningful at small n.

// Five, not three: at n=3 the p90 IS the maximum, one family's worst reading. Below the floor an axis keeps its hand
// threshold.
const goodGroupCalibrationMinN = 5

// A parameter because the compared quantity differs per axis while the percentile statistics do not, which keeps the
// statistics testable without a registry and a corpus.
type axisMagnitudeFunc func(axisResult) (float64, bool)

// Not |Signed|: a KindSeconds/KindRatio axis is compared as a folded ratio and KindHz as a percentage. Not Norm x
// mismatch: a comparator may transform its thresholds (band.t60). The comparator reports Delta.Mag in its own units;
// rows refused: MagOK false (sentinel branches), an unavailable side, a Demoted descriptor.
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

// Axes magOf refuses are skipped, so one unmeasurable on the good group keeps its hand threshold rather than one
// calibrated from the few families where it resolved.
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
		// A degenerate good group (all identical here) would collapse the two bands; the spread is SYNTHESISED, so the Note
		// says so and a reader can tell an earned p90 from 1.5x the p50.
		var note string
		if mismatch <= drift {
			mismatch = drift * 1.5
			note = fmt.Sprintf("p90 <= p50 over n=%d; mismatch synthesised as 1.5*drift", len(vs))
		}
		if mismatch <= 0 {
			continue
		}
		// A p50 of exactly zero would read as "no override" and fall back to the KIND's drift, possibly above the calibrated
		// mismatch. Anchored at mismatch/1.5 instead, and noted as derived.
		if drift <= 0 {
			drift = mismatch / 1.5
			note = fmt.Sprintf("p50 == 0 over n=%d; drift synthesised as mismatch/1.5", len(vs))
		}
		out[id] = fingerprint.ThresholdEntry{Drift: drift, Mismatch: mismatch, N: len(vs), Source: "corpus", Note: note}
	}
	return out
}

// PINNED entries survive recalibration: "probe" is an ear-measured audibility step, which outranks a percentile;
// "hand" pins a descriptor back to the value that keeps the acceptance gate reproducing a listener complaint.
func mergeProbeEntries(fresh, existing fingerprint.ThresholdFile) fingerprint.ThresholdFile {
	for id, e := range existing {
		if e.Source == "probe" || e.Source == "hand" {
			fresh[id] = e
		}
	}
	return fresh
}

// The file ON DISK, never the embedded copy (the file as of the last build): pinning is a hand edit followed by a
// recalibration, and the embedded copy would silently drop the edit. A missing file is not an error.
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
