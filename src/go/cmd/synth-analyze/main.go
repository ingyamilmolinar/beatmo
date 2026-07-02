//go:build !test && !js

// synth-analyze: spectral + envelope fingerprint tool for comparing real
// instrument recordings against synthesized equivalents.
//
// Usage:
//
//	synth-analyze ref   -file <audio> [-offset <sec>] [-length <sec>] [-json] [-label <name>]
//	synth-analyze synth -inst <id>    [-pitch <semi>]  [-dur <sec>]   [-json] [-label <name>]
//	synth-analyze diff  -a <fp.json>  -b <fp.json>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
)

// ---- Subcommands --------------------------------------------------------

func cmdRef(args []string) {
	fs := flag.NewFlagSet("ref", flag.ExitOnError)
	file := fs.String("file", "", "path to audio file")
	offset := fs.Float64("offset", 0, "start offset in seconds")
	length := fs.Float64("length", 0, "length to analyze in seconds (0 = auto-segment)")
	label := fs.String("label", "", "label for fingerprint")
	emitJSON := fs.Bool("json", false, "emit fingerprint JSON to stdout")
	_ = fs.Parse(args)

	if *file == "" {
		fmt.Fprintln(os.Stderr, "ref: -file is required")
		os.Exit(1)
	}

	fp, err := analyzeFile(*file, analyzeOpts{
		OffsetSec: *offset,
		LengthSec: *length,
		Label:     *label,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ref: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Fingerprint: %q  F0=%.2f Hz  SR=%d\n", fp.Label, fp.F0Hz, fp.SampleRate)

	if *emitJSON {
		printFP(os.Stderr, &fp)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fp); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
	} else {
		printFP(os.Stdout, &fp)
	}
}

func cmdSynth(args []string) {
	fs := flag.NewFlagSet("synth", flag.ExitOnError)
	inst := fs.String("inst", "violin", "instrument id")
	pitch := fs.Float64("pitch", 0, "pitch offset in semitones from A3=220Hz")
	dur := fs.Float64("dur", 0, "duration in seconds (0 = instrument default)")
	label := fs.String("label", "", "label for fingerprint")
	emitJSON := fs.Bool("json", false, "emit fingerprint JSON to stdout")
	out := fs.String("out", "", "write the rendered mono WAV to this path")
	_ = fs.Parse(args)

	lbl := *label
	if lbl == "" {
		lbl = *inst
	}

	sr := audio.SampleRate()
	fmt.Fprintf(os.Stderr, "Rendering %q at pitch=%+.1f, sr=%d...\n", *inst, *pitch, sr)

	w, err := synthmatch.RenderInstrument(*inst, *pitch, sr, *dur)
	if err != nil {
		fmt.Fprintf(os.Stderr, "synth: render error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Rendered %d samples (%.2f sec)\n", len(w.Samples), float64(len(w.Samples))/float64(sr))

	if *out != "" {
		if err := audio.ExportCaptureToWAV(w.Samples, *out); err != nil {
			fmt.Fprintf(os.Stderr, "synth: write WAV: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *out)
	}

	fp := fingerprint.FromWave(w, lbl)
	if *emitJSON {
		printFP(os.Stderr, &fp)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fp); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
	} else {
		printFP(os.Stdout, &fp)
	}
}

func cmdDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	fileA := fs.String("a", "", "first fingerprint JSON")
	fileB := fs.String("b", "", "second fingerprint JSON")
	_ = fs.Parse(args)

	if *fileA == "" || *fileB == "" {
		fmt.Fprintln(os.Stderr, "diff: -a and -b are required")
		os.Exit(1)
	}

	loadFP := func(path string) fingerprint.Fingerprint {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "diff: read %q: %v\n", path, err)
			os.Exit(1)
		}
		var fp fingerprint.Fingerprint
		if err := json.Unmarshal(data, &fp); err != nil {
			fmt.Fprintf(os.Stderr, "diff: parse %q: %v\n", path, err)
			os.Exit(1)
		}
		return fp
	}

	a := loadFP(*fileA)
	b := loadFP(*fileB)
	printFP(os.Stdout, &a)
	printFP(os.Stdout, &b)

	score := fingerprint.Distance(a, b)
	fmt.Printf("\n=== DIFF: %q vs %q ===\n\n", a.Label, b.Label)
	fmt.Printf("%-20s %10s\n", "Term", "Score")
	fmt.Printf("%s\n", "──────────────────────────────")
	fmt.Printf("%-20s %10.4f\n", "Spectral", score.Spectral)
	fmt.Printf("%-20s %10.4f\n", "MFCC", score.MFCC)
	fmt.Printf("%-20s %10.4f\n", "Partials", score.Partials)
	fmt.Printf("%-20s %10.4f\n", "Temporal", score.Temporal)
	fmt.Printf("%-20s %10.4f\n", "Envelope", score.Envelope)
	fmt.Printf("%-20s %10.4f\n", "Noise", score.Noise)
	fmt.Printf("%-20s %10.4f\n", "Timbre", score.Timbre)
	fmt.Printf("%s\n", "──────────────────────────────")
	fmt.Printf("%-20s %10.4f  (lower is better)\n\n", "Total", score.Total)
}

// ---- main ---------------------------------------------------------------

func main() {
	audio.Reset()
	audio.ResetInstruments()

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: synth-analyze <ref|synth|diff> [flags...]")
		fmt.Fprintln(os.Stderr, "  ref   -file <audio> [-offset <sec>] [-length <sec>] [-json] [-label <name>]")
		fmt.Fprintln(os.Stderr, "  synth -inst <id> [-pitch <semi>] [-dur <sec>] [-json] [-label <name>]")
		fmt.Fprintln(os.Stderr, "  diff  -a <fp.json> -b <fp.json>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "ref":
		cmdRef(os.Args[2:])
	case "synth":
		cmdSynth(os.Args[2:])
	case "diff":
		cmdDiff(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
