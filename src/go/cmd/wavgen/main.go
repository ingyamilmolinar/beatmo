//go:build !test && !js

// Command wavgen produces WAV files for tests and manual use WITHOUT any
// real/copyrighted audio: a sine tone, deterministic white noise, or a one-shot
// hit of one of the app's synth instruments. Encoding reuses the app's recording
// WAV encoder (via internal/wavgen); the synth-hit render reuses the Sampler's
// "capture from synth" path (audio.RenderInstrumentOneShot).
//
// Usage (run from src/go):
//
//	../../.tools/go/bin/go run ./cmd/wavgen -mode sine  -freq 440 -dur 1 -out sine.wav
//	../../.tools/go/bin/go run ./cmd/wavgen -mode noise -seed 7  -dur 0.5 -out noise.wav
//	../../.tools/go/bin/go run ./cmd/wavgen -mode synth -inst kick -out kick.wav
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

const sampleRate = 48000

func main() {
	mode := flag.String("mode", "sine", "sine | noise | synth")
	freq := flag.Float64("freq", 440, "sine frequency (Hz)")
	amp := flag.Float64("amp", 0.6, "amplitude 0..1")
	dur := flag.Float64("dur", 1.0, "duration (seconds)")
	seed := flag.Int("seed", 1, "noise seed (deterministic)")
	inst := flag.String("inst", "", "instrument id (synth mode) — e.g. kick, snare")
	out := flag.String("out", "out.wav", "output .wav path")
	flag.Parse()

	var pcm []float32
	sr := sampleRate
	switch *mode {
	case "sine":
		pcm = wavgen.Sine(*freq, *amp, *dur, sampleRate)
	case "noise":
		pcm = wavgen.Noise(*amp, *dur, sampleRate, *seed)
	case "synth":
		if *inst == "" {
			fatalf("synth mode requires -inst <instrument id>")
		}
		pcm, sr = audio.RenderInstrumentOneShot(instrument.ID(*inst))
		if len(pcm) == 0 {
			fatalf("instrument %q rendered no audio (unknown id?)", *inst)
		}
	default:
		fatalf("unknown -mode %q (want sine|noise|synth)", *mode)
	}

	data, err := wavgen.EncodeWAV(pcm, sr)
	if err != nil {
		fatalf("encode: %v", err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fatalf("write %s: %v", *out, err)
	}
	fmt.Printf("wrote %s — %d frames @ %d Hz (%s)\n", *out, len(pcm), sr, *mode)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "wavgen: "+format+"\n", args...)
	os.Exit(1)
}
