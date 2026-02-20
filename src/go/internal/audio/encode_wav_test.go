//go:build test

package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestWAVEncoderRegistry(t *testing.T) {
	for _, fmt := range []AudioFormat{FormatWAV16, FormatWAV24, FormatWAV32} {
		enc, err := NewEncoder(fmt)
		if err != nil {
			t.Fatalf("NewEncoder(%s): %v", fmt, err)
		}
		if enc.Format() != fmt {
			t.Errorf("Format() = %s, want %s", enc.Format(), fmt)
		}
		if enc.FileExtension() != ".wav" {
			t.Errorf("FileExtension() = %s, want .wav", enc.FileExtension())
		}
	}
}

func TestWAV16Encode(t *testing.T) {
	enc, _ := NewEncoder(FormatWAV16)
	samples := []float64{0, 0.5, -0.5, 1.0, -1.0}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 44100); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	// Verify RIFF header
	if string(data[0:4]) != "RIFF" {
		t.Fatalf("missing RIFF header")
	}
	if string(data[8:12]) != "WAVE" {
		t.Fatalf("missing WAVE format")
	}

	// Verify fmt chunk
	if string(data[12:16]) != "fmt " {
		t.Fatalf("missing fmt chunk")
	}
	audioFmt := binary.LittleEndian.Uint16(data[20:22])
	if audioFmt != 1 { // PCM
		t.Errorf("audio format = %d, want 1 (PCM)", audioFmt)
	}
	channels := binary.LittleEndian.Uint16(data[22:24])
	if channels != 1 {
		t.Errorf("channels = %d, want 1", channels)
	}
	sr := binary.LittleEndian.Uint32(data[24:28])
	if sr != 44100 {
		t.Errorf("sample rate = %d, want 44100", sr)
	}
	bps := binary.LittleEndian.Uint16(data[34:36])
	if bps != 16 {
		t.Errorf("bits per sample = %d, want 16", bps)
	}

	// Verify data chunk
	if string(data[36:40]) != "data" {
		t.Fatalf("missing data chunk")
	}
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if dataSize != uint32(len(samples)*2) {
		t.Errorf("data size = %d, want %d", dataSize, len(samples)*2)
	}
}

func TestWAV24Encode(t *testing.T) {
	enc, _ := NewEncoder(FormatWAV24)
	samples := []float64{0, 0.5, -0.5}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 48000); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	bps := binary.LittleEndian.Uint16(data[34:36])
	if bps != 24 {
		t.Errorf("bits per sample = %d, want 24", bps)
	}
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	if dataSize != uint32(len(samples)*3) {
		t.Errorf("data size = %d, want %d", dataSize, len(samples)*3)
	}
}

func TestWAV32FloatEncode(t *testing.T) {
	enc, _ := NewEncoder(FormatWAV32)
	samples := []float64{0, 0.5, -0.5, 1.0}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 44100); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	// IEEE float format tag = 3
	audioFmt := binary.LittleEndian.Uint16(data[20:22])
	if audioFmt != 3 {
		t.Errorf("audio format = %d, want 3 (IEEE float)", audioFmt)
	}
	bps := binary.LittleEndian.Uint16(data[34:36])
	if bps != 32 {
		t.Errorf("bits per sample = %d, want 32", bps)
	}

	// Verify first sample (0.0) in float32
	dataOffset := 44
	bits := binary.LittleEndian.Uint32(data[dataOffset : dataOffset+4])
	val := math.Float32frombits(bits)
	if val != 0.0 {
		t.Errorf("first sample = %f, want 0.0", val)
	}
}

func TestWAVEncodeEmpty(t *testing.T) {
	enc, _ := NewEncoder(FormatWAV16)
	var buf bytes.Buffer
	err := enc.Encode(&buf, nil, 44100)
	if err == nil {
		t.Fatal("expected error for empty samples")
	}
}

func TestAvailableFormats(t *testing.T) {
	fmts := AvailableFormats()
	if len(fmts) < 3 {
		t.Errorf("expected at least 3 formats, got %d", len(fmts))
	}
	// Should include at least wav16
	found := false
	for _, f := range fmts {
		if f == FormatWAV16 {
			found = true
		}
	}
	if !found {
		t.Error("FormatWAV16 not in AvailableFormats()")
	}
}

func TestNewEncoderInvalidFormat(t *testing.T) {
	_, err := NewEncoder("nonexistent_format")
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestFormatFromExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want AudioFormat
	}{
		{".wav", FormatWAV24},
		{".flac", FormatFLAC},
		{".ogg", FormatOGG},
		{".mp3", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := FormatFromExtension(tt.ext)
		if got != tt.want {
			t.Errorf("FormatFromExtension(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestWAVClampSample(t *testing.T) {
	// Samples outside [-1,1] should be clamped
	enc, _ := NewEncoder(FormatWAV16)
	samples := []float64{2.0, -2.0, 0.5}

	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, 44100); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	data := buf.Bytes()
	// First sample should be clamped to 1.0 → 32767
	s0 := int16(binary.LittleEndian.Uint16(data[44:46]))
	if s0 != 32767 {
		t.Errorf("clamped sample 0 = %d, want 32767", s0)
	}
	// Second sample clamped to -1.0 → -32767
	s1 := int16(binary.LittleEndian.Uint16(data[46:48]))
	if s1 != -32767 {
		t.Errorf("clamped sample 1 = %d, want -32767", s1)
	}
}
