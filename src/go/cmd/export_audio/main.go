//go:build !test && !js

// export_audio is a tool that renders desktop audio samples and exports them
// to JSON for cross-platform comparison with WASM output.
//
// Usage:
//
//	cd src/go && go run ./cmd/export_audio > ../js/desktop_audio.json
//	cd src/go && go run ./cmd/export_audio --config > ../js/audio_config.generated.js
//	cd src/go && go run ./cmd/export_audio --mixed > ../js/desktop_mixed_audio.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// InstrumentSample contains rendered audio data for one instrument.
type InstrumentSample struct {
	Name       string    `json:"name"`
	SampleRate int       `json:"sampleRate"`
	Frames     int       `json:"frames"`
	Amplitude  float64   `json:"amplitude"`
	Duration   float64   `json:"duration"`
	Samples    []float64 `json:"samples"`
	Peak       float64   `json:"peak"`
	RMS        float64   `json:"rms"`
	DCOffset   float64   `json:"dcOffset"`
}

// ExportData contains all exported audio samples.
type ExportData struct {
	Version     int                `json:"version"`
	Description string             `json:"description"`
	Instruments []InstrumentSample `json:"instruments"`
}

func main() {
	// Check for --config flag
	for _, arg := range os.Args[1:] {
		if arg == "--config" {
			exportJSConfig()
			return
		}
	}

	// Default: export audio samples
	exportAudioSamples()
}

func exportJSConfig() {
	fmt.Println("// AUTO-GENERATED from Go - do not edit")
	fmt.Println("// Run: cd src/go && go run ./cmd/export_audio --config > ../js/audio_config.generated.js")
	fmt.Println("")
	fmt.Println("export const RENDER_INFO = {")

	// Export all instrument configs
	instruments := []string{"snare", "kick", "hihat", "tom", "clap", "cowbell"}
	for i, id := range instruments {
		cfg := audio.ConfigForInstrument(id)
		comma := ","
		if i == len(instruments)-1 {
			comma = ""
		}
		fmt.Printf("  %s: { seconds: %.2f, amp: %.1f }%s\n", id, cfg.DurationSec, cfg.Amplitude, comma)
	}
	fmt.Println("};")
	fmt.Println("")
	fmt.Printf("export const MIX_HEADROOM = %.2f;\n", audio.MixHeadroom)
}

func exportAudioSamples() {
	// Initialize audio system
	audio.Reset()
	audio.ResetInstruments()

	data := ExportData{
		Version:     1,
		Description: "Desktop Go audio samples for cross-platform comparison",
	}

	// Define instruments to export. We don't specify duration here because
	// the Voice generator determines duration based on BPM.
	// At BPM=120, the durations are: snare=0.5s, kick=0.25s, hihat=0.125s,
	// tom=0.25s, clap=0.125s, cowbell=0.25s
	// Note: WASM uses different (longer) fixed durations which causes
	// waveform divergence since the C synth envelopes are proportional to buffer length.
	instruments := []string{
		"snare",
		"kick",
		"hihat",
		"tom",
		"clap",
		"cowbell",
	}

	const sampleRate = 48000 // Match the new desktop sample rate
	const bpm = 120          // Default BPM for voice generation

	for _, instID := range instruments {
		// Get a voice for this instrument which includes full processing pipeline
		// (C rendering + normalization + amplitude scaling)
		voice := audio.ExportVoice(instID, bpm, sampleRate)
		if voice == nil {
			fmt.Fprintf(os.Stderr, "Warning: could not get voice for %s\n", instID)
			continue
		}

		// Extract all samples from voice (don't specify frames - let voice determine length)
		samples := make([]float64, 0, sampleRate) // Pre-allocate for up to 1 second
		for {
			s, done := voice.Sample()
			if done {
				break
			}
			samples = append(samples, s)
		}

		// Compute metrics
		peak := computePeak(samples)
		rms := computeRMS(samples)
		dc := computeDCOffset(samples)

		// Calculate actual duration from sample count
		duration := float64(len(samples)) / float64(sampleRate)

		data.Instruments = append(data.Instruments, InstrumentSample{
			Name:       instID,
			SampleRate: sampleRate,
			Frames:     len(samples),
			Amplitude:  audio.AmplitudeForInstrumentExport(instID),
			Duration:   duration,
			Samples:    samples,
			Peak:       peak,
			RMS:        rms,
			DCOffset:   dc,
		})
	}

	// Output as JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

func computePeak(data []float64) float64 {
	var peak float64
	for _, v := range data {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	return peak
}

func computeRMS(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v * v
	}
	return sqrt(sum / float64(len(data)))
}

func computeDCOffset(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
