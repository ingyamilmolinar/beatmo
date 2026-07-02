//go:build !js

// Package main tests for synth-analyze CLI tool.
// These tests exercise the analyzeFile function and printFP output against the
// real API. The old self-contained FFT/fingerprint helpers (fft, nextPow2,
// computeFingerprint, renderSynth, rmsEnvelope, peakNormalize,
// computeTemporalFingerprint) were removed when those concerns were subsumed by
// the fingerprint package and its own tests.
//
// COVERED-BY-GO: internal/audio/fingerprint/*_test.go covers all DSP
// correctness (FFT, F0 detection, vibrato, harmonics, formants, coupling,
// noise profile, STFT flux/centroid). This file tests the CLI layer only:
// analyzeFile integration, printFP human-readable output sections.
package main

import (
	"strings"
	"testing"
)

// TestAnalyzeFile_RefReportsCorrectF0AndVibrato decodes the violin D5 reference
// WAV at its native sample rate and checks that F0 is plausibly ~591 Hz
// (D5 ≈ 587.3 Hz) and that the Vibrato sub-struct is populated.
func TestAnalyzeFile_RefReportsCorrectF0AndVibrato(t *testing.T) {
	fp, err := analyzeFile("../../../../356181__mtg__violin-d5.wav", analyzeOpts{})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}
	if fp.F0Hz < 575 || fp.F0Hz > 605 {
		t.Fatalf("F0=%.1f, want ~591", fp.F0Hz)
	}
	if fp.Vibrato == nil {
		t.Fatal("Vibrato nil")
	}
}

// TestPrintFP_IncludesNewMetricSections verifies that printFP emits sections for
// Vibrato, SpectralEnv (Bridge), and Coupling when those sub-structs are present.
func TestPrintFP_IncludesNewMetricSections(t *testing.T) {
	fp, err := analyzeFile("../../../../356181__mtg__violin-d5.wav", analyzeOpts{})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}
	var sb strings.Builder
	printFP(&sb, &fp)
	out := sb.String()
	for _, want := range []string{"Vibrato", "Bridge", "Coupling"} {
		if !strings.Contains(out, want) {
			t.Errorf("printFP output missing %q section", want)
		}
	}
}

// TestAnalyzeFile_SilentOrMissingFileErrors verifies that analyzeFile returns an
// error for a non-existent path (callers must not panic or os.Exit).
func TestAnalyzeFile_SilentOrMissingFileErrors(t *testing.T) {
	_, err := analyzeFile("/nonexistent/path/to/file.wav", analyzeOpts{})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestAnalyzeFile_LabelDefault verifies that when no Label is set in opts,
// analyzeFile uses the file path as the fingerprint label.
func TestAnalyzeFile_LabelDefault(t *testing.T) {
	fp, err := analyzeFile("../../../../356181__mtg__violin-d5.wav", analyzeOpts{})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}
	if fp.Label == "" {
		t.Fatal("expected non-empty label, got empty")
	}
}

// TestAnalyzeFile_LabelOverride verifies that a custom Label in opts is used.
func TestAnalyzeFile_LabelOverride(t *testing.T) {
	fp, err := analyzeFile("../../../../356181__mtg__violin-d5.wav", analyzeOpts{Label: "my-violin"})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}
	if fp.Label != "my-violin" {
		t.Fatalf("expected label %q, got %q", "my-violin", fp.Label)
	}
}

// TestPrintFP_NewMetricSectionsAbsentWhenNil verifies that printFP does not
// panic and does not emit metric-section headers when the pointer fields are nil.
func TestPrintFP_NewMetricSectionsAbsentWhenNil(t *testing.T) {
	fp, err := analyzeFile("../../../../356181__mtg__violin-d5.wav", analyzeOpts{})
	if err != nil {
		t.Skipf("ref unavailable: %v", err)
	}

	// Set all five new pointer fields to nil to test the "absent" path.
	fp.Vibrato = nil
	fp.Harmonics = nil
	fp.SpectralEnv = nil
	fp.Coupling = nil
	fp.NoiseProf = nil

	var sb strings.Builder
	// Should not panic.
	printFP(&sb, &fp)
	out := sb.String()

	// With nil pointers, the five new section headers must NOT appear.
	for _, absent := range []string{"--- Vibrato ---", "--- Spectral Envelope ---", "--- Coupling ---", "--- Harmonics ---", "--- Noise Profile ---"} {
		if strings.Contains(out, absent) {
			t.Errorf("printFP emitted section %q despite nil pointer", absent)
		}
	}
}
