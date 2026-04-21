//go:build test

package scopeexport

import (
	"bytes"
	"encoding/json"
	"testing"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestBufferSnapshot_EmptyBufferReturnsNil(t *testing.T) {
	s := NewService(Config{SampleRate: 48000})
	if got := s.DumpBuffer(); got != nil {
		t.Fatalf("empty buffer should return nil, got %d bytes", len(got))
	}
}

func TestBufferSnapshot_AppendsJSONL(t *testing.T) {
	s := NewService(Config{SampleRate: 48000})
	// Seed one ring buffer so buildSnapshot produces at least one channel.
	samples := make([]float64, 256)
	for i := range samples {
		samples[i] = 0.5
	}
	s.PushSamples(scope.StageSynth, "kick", samples)

	s.BufferSnapshot()
	out := s.DumpBuffer()
	if len(out) == 0 {
		t.Fatal("expected DumpBuffer to return one JSONL line")
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatal("each JSONL line must end with newline")
	}
	// Each line must parse as valid JSON.
	line := bytes.TrimRight(out, "\n")
	var snap Snapshot
	if err := json.Unmarshal(line, &snap); err != nil {
		t.Fatalf("JSON parse failed: %v (line=%q)", err, string(line))
	}
	if snap.SampleRate != 48000 {
		t.Fatalf("expected SampleRate=48000, got %d", snap.SampleRate)
	}
}

func TestBufferSnapshot_MultipleAppends(t *testing.T) {
	s := NewService(Config{SampleRate: 48000})
	// Each BufferSnapshot after a new PushSamples produces a new line.
	for i := 0; i < 3; i++ {
		s.PushSamples(scope.StageSynth, "kick", make([]float64, 128))
		s.BufferSnapshot()
	}
	out := s.DumpBuffer()
	lines := bytes.Count(out, []byte("\n"))
	if lines != 3 {
		t.Fatalf("expected 3 lines, got %d (total=%d bytes)", lines, len(out))
	}
}

func TestBufferSnapshot_DumpClearsBuffer(t *testing.T) {
	s := NewService(Config{SampleRate: 48000})
	s.PushSamples(scope.StageSynth, "kick", make([]float64, 128))
	s.BufferSnapshot()
	first := s.DumpBuffer()
	if len(first) == 0 {
		t.Fatal("first dump empty")
	}
	second := s.DumpBuffer()
	if second != nil {
		t.Fatalf("buffer should be cleared after dump, got %d bytes", len(second))
	}
}
