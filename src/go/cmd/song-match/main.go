//go:build !test && !js

// song-match: song-level analysis tooling (Plan 1: the `timeline` subcommand; Plan 2: the `render` subcommand; Plan 3: the `compare` subcommand).
//
// Usage:
//
//	song-match timeline -file <wav> [-config c.json] [-hop 0.5] [-out tl.csv]
//	song-match render -template <stem> [-out <dir>] [-bars N] [-sr N]
//	song-match compare -template <stem> -ref <wav> [-manifest m.json] [-config c.json] [-bars N] [-sr N] [-tune] [-dry-run]
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/songmatch"
	"github.com/ingyamilmolinar/beatmo/internal/audio/songrender"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: song-match timeline -file <wav> [-config c.json] [-hop N] [-out tl.csv]")
		fmt.Fprintln(os.Stderr, "       song-match render -template <stem> [-out <dir>] [-bars N] [-sr N]")
		fmt.Fprintln(os.Stderr, "       song-match compare -template <stem> -ref <wav> [-manifest m.json] [-config c.json] [-bars N] [-sr N] [-tune] [-dry-run]")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "timeline":
		cmdTimeline(os.Args[2:])
	case "render":
		cmdRender(os.Args[2:])
	case "compare":
		cmdCompare(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}

func cmdTimeline(args []string) {
	fs := flag.NewFlagSet("timeline", flag.ExitOnError)
	file := fs.String("file", "", "path to WAV (required)")
	configPath := fs.String("config", "", "AnalysisConfig JSON (optional)")
	hop := fs.Float64("hop", 0, "override hop seconds (optional)")
	out := fs.String("out", "", "write per-frame CSV to this path (optional)")
	_ = fs.Parse(args)

	if *file == "" {
		fmt.Fprintln(os.Stderr, "song-match timeline: -file is required")
		os.Exit(1)
	}

	cfg := fingerprint.DefaultAnalysisConfig()
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read config: %v\n", err)
			os.Exit(1)
		}
		cfg, err = fingerprint.LoadAnalysisConfig(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse config: %v\n", err)
			os.Exit(1)
		}
	}
	if *hop > 0 {
		cfg.HopSec = *hop
	}

	audio.Reset()
	audio.ResetInstruments()
	pcm, sr, err := audio.DecodeWAVToPCM(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode %q: %v\n", *file, err)
		os.Exit(1)
	}
	f64 := make([]float64, len(pcm))
	for i, v := range pcm {
		f64[i] = float64(v)
	}
	w := wave.Wave{Samples: f64, SampleRate: sr, Label: *file}

	fp := fingerprint.SongFingerprintOf(w, cfg)
	tl := fingerprint.SongTimelineOf(w, cfg)

	// Whole-segment summary to STDOUT.
	noteNames := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	mode := "major"
	if fp.Mode == 1 {
		mode = "minor"
	}
	fmt.Printf("file: %s  dur=%.2fs sr=%d\n", *file, fp.DurationSec, fp.SampleRate)
	fmt.Printf("tempo: %.1f BPM (conf %.2f)\n", fp.TempoBPM, fp.TempoConfidence)
	fmt.Printf("key:   %s %s (conf %.2f)\n", noteNames[fp.Key], mode, fp.KeyConfidence)
	fmt.Printf("primary: %.1f Hz   intonation: %+.1f cents   tonal/perc: %.2f\n",
		fp.PrimaryHz, fp.IntonationCents, fp.TonalPercussiveRatio)

	// Section narrative.
	fmt.Println("sections:")
	for _, s := range tl.Sections {
		fmt.Printf("  %6.2f–%6.2f  %s\n", s.StartSec, s.EndSec, s.Label)
	}

	if *out != "" {
		if err := writeTimelineCSV(*out, tl, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %d frames to %s\n", len(tl.Frames), *out)
	}
}

func writeTimelineCSV(path string, tl fingerprint.SongTimeline, cfg fingerprint.AnalysisConfig) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	wr := csv.NewWriter(f)
	defer wr.Flush()
	header := []string{"time_sec", "rms", "onset_density", "register_hz", "key", "mode"}
	for _, b := range cfg.Bands {
		header = append(header, "band_"+b.Name)
	}
	if err := wr.Write(header); err != nil {
		return err
	}
	for _, fr := range tl.Frames {
		row := []string{
			strconv.FormatFloat(fr.TimeSec, 'f', 3, 64),
			strconv.FormatFloat(fr.RMS, 'g', 4, 64),
			strconv.FormatFloat(fr.OnsetDensity, 'g', 4, 64),
			strconv.FormatFloat(fr.RegisterCentroidHz, 'f', 1, 64),
			strconv.Itoa(fr.Key), strconv.Itoa(fr.Mode),
		}
		for _, b := range fr.Bands {
			row = append(row, strconv.FormatFloat(b, 'g', 4, 64))
		}
		if err := wr.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func cmdRender(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	stem := fs.String("template", "", "template stem id, e.g. bach-toccata (required)")
	outDir := fs.String("out", ".", "output directory for master.wav + <inst>.wav stems")
	bars := fs.Int("bars", 8, "number of bars to render")
	sr := fs.Int("sr", 44100, "sample rate")
	_ = fs.Parse(args)
	if *stem == "" {
		fmt.Fprintln(os.Stderr, "song-match render: -template is required")
		os.Exit(1)
	}
	audio.Reset()
	audio.ResetInstruments()
	r, err := songrender.Render(*stem, *bars, *sr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	masterPath := filepath.Join(*outDir, "master.wav")
	if err := audio.ExportCaptureToWAV(r.Master, masterPath); err != nil {
		fmt.Fprintf(os.Stderr, "write master: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d samples)\n", masterPath, len(r.Master))
	for id, s := range r.Stems {
		p := filepath.Join(*outDir, id+".wav")
		if err := audio.ExportCaptureToWAV(s, p); err != nil {
			fmt.Fprintf(os.Stderr, "write stem %s: %v\n", id, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", p)
	}
}

func cmdCompare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)
	stem := fs.String("template", "", "template stem id (required)")
	ref := fs.String("ref", "", "reference WAV path (required)")
	manifestPath := fs.String("manifest", "", "optional TimeManifest JSON")
	configPath := fs.String("config", "", "optional AnalysisConfig JSON")
	bars := fs.Int("bars", 8, "bars to render")
	srFlag := fs.Int("sr", 44100, "sample rate")
	tune := fs.Bool("tune", false, "run solo-passage tuning from the manifest")
	dryRun := fs.Bool("dry-run", false, "with -tune, do not patch the seed file")
	seedfile := fs.String("seedfile", "internal/audio/instrument_seeds.go", "seed file to patch")
	_ = fs.Parse(args)
	if *stem == "" || *ref == "" {
		fmt.Fprintln(os.Stderr, "song-match compare: -template and -ref are required")
		os.Exit(1)
	}

	cfg := fingerprint.DefaultAnalysisConfig()
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err == nil {
			cfg, err = fingerprint.LoadAnalysisConfig(b)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			os.Exit(1)
		}
	}
	var manifest fingerprint.TimeManifest
	if *manifestPath != "" {
		b, err := os.ReadFile(*manifestPath)
		if err == nil {
			manifest, err = fingerprint.LoadTimeManifest(b)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "manifest: %v\n", err)
			os.Exit(1)
		}
	}

	audio.Reset()
	audio.ResetInstruments()

	pcm, sr, err := audio.DecodeWAVToPCM(*ref)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decode ref: %v\n", err)
		os.Exit(1)
	}
	rf := make([]float64, len(pcm))
	for i, v := range pcm {
		rf[i] = float64(v)
	}
	refWav := wave.Wave{Samples: rf, SampleRate: sr, Label: *ref}
	refFP := fingerprint.SongFingerprintOf(refWav, cfg)

	rendered, err := songrender.Render(*stem, *bars, *srFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render template: %v\n", err)
		os.Exit(1)
	}
	candFP := fingerprint.SongFingerprintOf(wave.Wave{Samples: rendered.Master, SampleRate: *srFlag}, cfg)

	rep := songmatch.Compare(refFP, candFP, cfg)
	fmt.Printf("\n=== compare: template %q vs %q ===\n", *stem, *ref)
	for _, a := range rep.Axes {
		fmt.Printf("  %-7s %-9s %6.3f  %s\n", a.Axis, a.State, a.Magnitude, a.Detail)
	}
	fmt.Printf("  %-7s %18.3f\n", "TOTAL", rep.Distance.Total)

	for _, h := range songmatch.BandAttribution(refFP, rendered, cfg) {
		fmt.Printf("  hint [%s] %s: %s\n", h.Kind, h.Instrument, h.Suggestion)
	}

	// Approximate per-instrument activity over the reference timeline (spec §7.1).
	realTL := fingerprint.SongTimelineOf(refWav, cfg)
	actIDs, act := songmatch.InstrumentActivity(realTL, rendered.Stems, *srFlag, cfg)
	if len(act) > 0 {
		fmt.Printf("\n  --- instrument activity (approximate; reliable on solo/sparse) ---\n")
		for i, id := range actIDs {
			dom := 0
			for f := range act {
				best, bestIdx := 0.0, -1
				for j := range act[f] {
					if act[f][j] > best {
						best, bestIdx = act[f][j], j
					}
				}
				if bestIdx == i {
					dom++
				}
			}
			fmt.Printf("  %-20s dominant in %d/%d frames (%.0f%%)\n", id, dom, len(act), 100*float64(dom)/float64(len(act)))
		}
	}

	if *tune {
		hints, err := songmatch.TunePassages(refWav, manifest, 400, 3)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tune: %v\n", err)
			os.Exit(1)
		}
		for _, h := range hints {
			fmt.Printf("  hint [seed] %s: %s\n", h.Instrument, h.Suggestion)
			if !*dryRun {
				if err := synthmatch.PatchSeedFile(*seedfile, songmatch.SeedVarName(h.Instrument), h.SeedDeltas); err != nil {
					fmt.Fprintf(os.Stderr, "patch %s: %v\n", h.Instrument, err)
					os.Exit(1)
				}
				fmt.Printf("  patched %s in %s\n", songmatch.SeedVarName(h.Instrument), *seedfile)
			}
		}
		if *dryRun {
			fmt.Println("  (dry-run: seeds not modified)")
		}
	}
}
