//go:build !test && !js

// synth-match: tune a synthesized instrument toward a reference WAV by
// minimizing the fingerprint distance, then (optionally) patch the seed file.
//
// Usage:
//
//	synth-match -inst <id> -ref <wav> [-budget N] [-restarts N] [-dry-run] [-seedfile <path>]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func main() {
	inst := flag.String("inst", "", "instrument id (required)")
	ref := flag.String("ref", "", "path to reference WAV file (required)")
	budget := flag.Int("budget", 400, "total evaluation budget")
	restarts := flag.Int("restarts", 3, "number of random restarts")
	dryRun := flag.Bool("dry-run", false, "print report but do not patch the seed file")
	seedfile := flag.String("seedfile", "internal/audio/instrument_seeds.go", "seed file to patch")
	flag.Parse()

	if *inst == "" {
		fmt.Fprintln(os.Stderr, "synth-match: -inst is required")
		os.Exit(1)
	}
	if *ref == "" {
		fmt.Fprintln(os.Stderr, "synth-match: -ref is required")
		os.Exit(1)
	}

	// Step 1: initialise audio engine.
	audio.Reset()
	audio.ResetInstruments()

	// Step 2: decode the reference WAV.
	pcm, sr, err := audio.DecodeWAVToPCM(*ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "synth-match: decode %q: %v\n", *ref, err)
		os.Exit(1)
	}
	refWave := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}

	// Auto-segment: pick the loudest SustainLenSec window so multi-note or
	// long files don't misalign the match against a single synthesized tone.
	refWave = fingerprint.AutoSegment(refWave, fingerprint.SustainLenSec)
	fmt.Fprintf(os.Stderr, "synth-match: analyzed segment: %d samples (%.2f sec)\n",
		len(refWave.Samples), float64(len(refWave.Samples))/float64(sr))

	// Step 3: run the matcher.
	fmt.Fprintf(os.Stderr, "synth-match: tuning %q against %q (%d samples @ %d Hz) budget=%d restarts=%d\n",
		*inst, *ref, len(pcm), sr, *budget, *restarts)

	res, err := synthmatch.Match(*inst, refWave, *budget, *restarts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "synth-match: match failed: %v\n", err)
		os.Exit(1)
	}

	// Step 4: print the report to STDOUT.
	printReport(res)

	// Step 5: patch or skip.
	if !*dryRun {
		seedVar := seedVarName(*inst)
		if err := synthmatch.PatchSeedFile(*seedfile, seedVar, res.Params); err != nil {
			fmt.Fprintf(os.Stderr, "synth-match: patch failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("patched %s in %s\n", seedVar, *seedfile)
	} else {
		fmt.Println("(dry-run: seed not modified)")
	}
}

// printReport writes the before/after summary and per-parameter table to STDOUT.
func printReport(res synthmatch.Result) {
	improv := 0.0
	if res.StartLoss > 0 {
		improv = (res.StartLoss - res.BestLoss) / res.StartLoss * 100
	}

	fmt.Printf("\n=== synth-match: %q ===\n\n", res.InstID)
	fmt.Printf("  StartLoss → BestLoss:  %.6f → %.6f  (%.1f%% improvement)\n",
		res.StartLoss, res.BestLoss, improv)
	fmt.Printf("  Evaluations:           %d\n", res.Evals)

	// Fingerprint distance breakdown.
	dist := fingerprint.Distance(res.RefFP, res.BestFP)
	fmt.Printf("\n  --- Fingerprint distance (ref vs best) ---\n")
	fmt.Printf("  %-12s %10.4f\n", "Spectral", dist.Spectral)
	fmt.Printf("  %-12s %10.4f\n", "MFCC", dist.MFCC)
	fmt.Printf("  %-12s %10.4f\n", "Partials", dist.Partials)
	fmt.Printf("  %-12s %10.4f\n", "Temporal", dist.Temporal)
	fmt.Printf("  %-12s %10.4f\n", "Envelope", dist.Envelope)
	fmt.Printf("  %-12s %10.4f\n", "Noise", dist.Noise)
	fmt.Printf("  %-12s %10.4f\n", "Timbre", dist.Timbre)
	fmt.Printf("  %-12s %10.4f  (lower is better)\n", "Total", dist.Total)

	// Per-parameter table (sorted keys).
	keys := make([]string, 0, len(res.Params))
	for k := range res.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("\n  --- Tuned parameters ---\n")
	for _, k := range keys {
		fmt.Printf("  %-40s %g\n", k+":", res.Params[k])
	}
	fmt.Println()
}

// f32ToF64 converts a []float32 slice to []float64.
func f32ToF64(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}
