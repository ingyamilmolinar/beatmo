//go:build test

package audio

import (
	"bytes"
	"testing"
)

func TestOGGEncoderRegistry(t *testing.T) {
	enc, err := NewEncoder(FormatOGG)
	if err != nil {
		t.Fatalf("NewEncoder(ogg): %v", err)
	}
	if enc.Format() != FormatOGG {
		t.Errorf("Format() = %s, want ogg", enc.Format())
	}
	if enc.FileExtension() != ".ogg" {
		t.Errorf("FileExtension() = %s, want .ogg", enc.FileExtension())
	}
}

func TestOGGEncodeReturnsNotImplemented(t *testing.T) {
	enc, _ := NewEncoder(FormatOGG)
	samples := []float64{0.1, 0.2, 0.3}
	var buf bytes.Buffer
	err := enc.Encode(&buf, samples, 44100)
	if err == nil {
		t.Fatal("expected error from OGG encoder (not yet implemented)")
	}
	// Should mention "not yet implemented"
	if !containsStr(err.Error(), "not yet implemented") {
		t.Errorf("error should mention 'not yet implemented', got: %s", err.Error())
	}
}

func TestOGGEncodeEmpty(t *testing.T) {
	enc, _ := NewEncoder(FormatOGG)
	var buf bytes.Buffer
	err := enc.Encode(&buf, nil, 44100)
	if err == nil {
		t.Error("expected error for empty samples")
	}
}
