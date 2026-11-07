//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The thresholds file is embedded generated data: an analysis change moves every magnitude while the JSON keeps the
// old numbers. Untagged build only (CGo synth), at wavref.Rate, the corpus rate. Regenerate: refs calibrate-thresholds,
// REBUILD, re-run this, then TestCalibratedThresholdsStillReproduceTheListener.
func checkThresholdsGolden(r *checkReport) error {
	committedPath, err := defaultThresholdPath()
	if err != nil {
		return err
	}
	verdictsPath, err := defaultVerdictsPath()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "refs-check-thresholds-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// Seeded with the committed file so the pinned-entry carry-over (read from the OUTPUT path) is exercised.
	outPath := filepath.Join(tmp, "thresholds.json")
	committed, err := os.ReadFile(committedPath)
	if err != nil {
		return fmt.Errorf("read committed thresholds.json: %w", err)
	}
	if err := os.WriteFile(outPath, committed, 0o644); err != nil {
		return err
	}
	var diag strings.Builder
	if err := runCalibrateThresholds(io.Discard, &diag, verdictsPath, outPath); err != nil {
		return fmt.Errorf("calibrate-thresholds: %w", err)
	}
	got, err := readThresholdFileAt(outPath)
	if err != nil {
		return err
	}
	want, err := readThresholdFileAt(committedPath)
	if err != nil {
		return err
	}
	if len(got) == 0 {
		r.Errorf("fresh calibration is empty -- the check would be vacuous")
		return nil
	}
	if mis := diffThresholdFiles(got, want); len(mis) > 0 {
		r.Errorf("testdata/thresholds.json drifted from a fresh calibration over the corpus:\n  %s\n"+
			"regenerate with `refs calibrate-thresholds`, rebuild, and re-run the calibrated replay gate",
			strings.Join(mis, "\n  "))
		return nil
	}
	r.Logf("%d calibrated descriptors reproduce from the corpus", len(got))
	return nil
}

// diffThresholdFiles lists every descriptor whose fresh calibration differs
// from the committed one, sorted.
func diffThresholdFiles(got, want fingerprint.ThresholdFile) []string {
	ids := map[string]bool{}
	for id := range got {
		ids[id] = true
	}
	for id := range want {
		ids[id] = true
	}
	var mismatched []string
	for id := range ids {
		g, gok := got[id]
		w, wok := want[id]
		switch {
		case gok != wok:
			mismatched = append(mismatched, fmt.Sprintf("%s (fresh=%v committed=%v)", id, gok, wok))
		case g.N != w.N:
			mismatched = append(mismatched, fmt.Sprintf("%s (n %d -> %d)", id, w.N, g.N))
		case g.Source != w.Source:
			mismatched = append(mismatched, fmt.Sprintf("%s (source %s -> %s)", id, w.Source, g.Source))
		case math.Abs(g.Drift-w.Drift) > 1e-9 || math.Abs(g.Mismatch-w.Mismatch) > 1e-9:
			mismatched = append(mismatched, fmt.Sprintf("%s (%.6g/%.6g -> %.6g/%.6g)",
				id, w.Drift, w.Mismatch, g.Drift, g.Mismatch))
		}
	}
	sort.Strings(mismatched)
	return mismatched
}

// Same build and rate rules as checkThresholdsGolden; regenerate with refs calibrate-organic.
func checkOrganicRangeGolden(r *checkReport) error {
	committedPath, err := defaultOrganicRangePath()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "refs-check-organic-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	outPath := filepath.Join(tmp, "organic_range.json")
	var diag strings.Builder
	if err := runCalibrateOrganic(io.Discard, &diag, outPath); err != nil {
		return fmt.Errorf("calibrate-organic: %w", err)
	}
	got, err := readOrganicRangeAt(outPath)
	if err != nil {
		return err
	}
	want, err := readOrganicRangeAt(committedPath)
	if err != nil {
		return err
	}
	if mis := diffOrganicRanges(got, want); len(mis) > 0 {
		r.Errorf("testdata/organic_range.json drifted from a fresh calibration for: %v -- "+
			"regenerate with `refs calibrate-organic`", mis)
		return nil
	}
	r.Logf("%d organic ranges reproduce from the corpus", len(got))
	return nil
}

func readOrganicRangeAt(path string) (fingerprint.OrganicRange, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out fingerprint.OrganicRange
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

// Also ids in either file that OrganicIDs() no longer covers: both mean regenerate.
func diffOrganicRanges(got, want fingerprint.OrganicRange) []string {
	var mismatched []string
	seen := map[string]bool{}
	for _, id := range fingerprint.OrganicIDs() {
		seen[id] = true
		g, gok := got[id]
		w, wok := want[id]
		switch {
		case gok != wok:
			mismatched = append(mismatched, id)
		case !gok:
			// neither side has this id -- nothing to compare.
		case g.N != w.N ||
			math.Abs(g.P10-w.P10) > 1e-9 ||
			math.Abs(g.P50-w.P50) > 1e-9 ||
			math.Abs(g.P90-w.P90) > 1e-9:
			mismatched = append(mismatched, id)
		}
	}
	for id := range got {
		if !seen[id] {
			mismatched = append(mismatched, id)
		}
	}
	for id := range want {
		if !seen[id] {
			mismatched = append(mismatched, id)
		}
	}
	sort.Strings(mismatched)
	return dedupSorted(mismatched)
}

func dedupSorted(s []string) []string {
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}
