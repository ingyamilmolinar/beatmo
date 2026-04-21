package scopeexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestNewServiceDefaults(t *testing.T) {
	s := NewService(Config{})
	if s.cfg.SampleRate != 44100 {
		t.Errorf("expected SampleRate 44100, got %d", s.cfg.SampleRate)
	}
	if s.cfg.Interval != 2*time.Second {
		t.Errorf("expected Interval 2s, got %v", s.cfg.Interval)
	}
	if s.cfg.OutputPath != "scope_export.jsonl" {
		t.Errorf("expected OutputPath 'scope_export.jsonl', got %q", s.cfg.OutputPath)
	}
	if s.cfg.FFTSize != 1024 {
		t.Errorf("expected FFTSize 1024, got %d", s.cfg.FFTSize)
	}
	if s.cfg.WaveformBins != 64 {
		t.Errorf("expected WaveformBins 64, got %d", s.cfg.WaveformBins)
	}
	if s.cfg.FFTTopN != 16 {
		t.Errorf("expected FFTTopN 16, got %d", s.cfg.FFTTopN)
	}
}

func TestPushAndDrain(t *testing.T) {
	s := NewService(Config{SampleRate: 44100})

	// Push to two different instruments at two different stages.
	s.PushSamples(scope.StageSynth, "kick", []float64{0.1, 0.2})
	s.PushSamples(scope.StageSynth, "snare", []float64{0.3})
	s.PushSamples(scope.StageEQ, "kick", []float64{0.4, 0.5, 0.6})

	// Drain stage 0 (Synth).
	stage0 := s.stages[scope.StageSynth].drainAll()
	if len(stage0) != 2 {
		t.Fatalf("expected 2 instruments in stage 0, got %d", len(stage0))
	}
	if len(stage0["kick"]) != 2 {
		t.Errorf("expected 2 kick samples in stage 0, got %d", len(stage0["kick"]))
	}
	if len(stage0["snare"]) != 1 {
		t.Errorf("expected 1 snare sample in stage 0, got %d", len(stage0["snare"]))
	}

	// Drain stage 3 (EQ).
	stage3 := s.stages[scope.StageEQ].drainAll()
	if len(stage3["kick"]) != 3 {
		t.Errorf("expected 3 kick samples in stage 3, got %d", len(stage3["kick"]))
	}

	// Second drain should be empty.
	stage0again := s.stages[scope.StageSynth].drainAll()
	if len(stage0again) != 0 {
		t.Errorf("expected empty after second drain, got %d", len(stage0again))
	}
}

func TestSnapshotJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_export.jsonl")

	s := NewService(Config{
		SampleRate: 44100,
		Interval:   50 * time.Millisecond,
		OutputPath: path,
		BPMFunc:    func() int { return 120 },
		LookupMeta: func(id string) (string, string, bool) {
			return "TestKick", "synth", true
		},
		ChannelVolume: func(id string) float64 { return 0.8 },
		ChannelPan:    func(id string) float64 { return 0.0 },
		MainVolume:    func() float64 { return 1.0 },
	})

	// Push some data.
	samples := make([]float64, 1024)
	for i := range samples {
		samples[i] = float64(i%2)*0.5 - 0.25 // square-ish
	}
	s.PushSamples(scope.StageSynth, "kick", samples)
	s.PushSamples(scope.StageMaster, "master", samples)

	// Start and let one tick fire.
	go s.Run()
	time.Sleep(120 * time.Millisecond)
	s.Stop()

	// Read the output file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("output file is empty")
	}

	// Parse the first line.
	var snap Snapshot
	if err := json.Unmarshal(data[:len(data)-1], &snap); err != nil {
		// Try to find the first newline for multi-line output.
		for i, b := range data {
			if b == '\n' {
				if err2 := json.Unmarshal(data[:i], &snap); err2 != nil {
					t.Fatalf("failed to parse JSONL: %v (first line: %s)", err2, string(data[:i]))
				}
				break
			}
		}
	}

	if snap.BPM != 120 {
		t.Errorf("expected BPM 120, got %d", snap.BPM)
	}
	if snap.SampleRate != 44100 {
		t.Errorf("expected SampleRate 44100, got %d", snap.SampleRate)
	}
	if snap.Tick < 1 {
		t.Errorf("expected tick >= 1, got %d", snap.Tick)
	}

	// Should have at least 1 channel (kick).
	if len(snap.Channels) < 1 {
		t.Fatalf("expected at least 1 channel, got %d", len(snap.Channels))
	}
	kickCh := snap.Channels[0]
	if kickCh.ID != "kick" {
		t.Errorf("expected channel ID 'kick', got %q", kickCh.ID)
	}
	if kickCh.Name != "TestKick" {
		t.Errorf("expected channel name 'TestKick', got %q", kickCh.Name)
	}
	if _, ok := kickCh.Stages["synth"]; !ok {
		t.Error("expected 'synth' stage in kick channel")
	}

	// Master should have data.
	if _, ok := snap.Master.Stages["master"]; !ok {
		t.Error("expected 'master' stage in master")
	}
}

func TestMultipleInstruments(t *testing.T) {
	s := NewService(Config{SampleRate: 44100})

	// Push instruments at different times.
	s.PushSamples(scope.StageSynth, "kick", []float64{0.5})
	s.PushSamples(scope.StageSynth, "snare", []float64{0.3})
	s.PushSamples(scope.StageSynth, "hihat", []float64{0.1})

	drained := s.stages[scope.StageSynth].drainAll()
	if len(drained) != 3 {
		t.Errorf("expected 3 instruments, got %d", len(drained))
	}
	for _, id := range []string{"kick", "snare", "hihat"} {
		if _, ok := drained[id]; !ok {
			t.Errorf("missing instrument %q in drained data", id)
		}
	}
}

func TestWriterAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "append_test.jsonl")

	w, err := openWriter(path)
	if err != nil {
		t.Fatal(err)
	}

	snap1 := &Snapshot{Tick: 1, BPM: 120}
	snap2 := &Snapshot{Tick: 2, BPM: 120}

	if err := w.writeLine(snap1); err != nil {
		t.Fatal(err)
	}
	if err := w.writeLine(snap2); err != nil {
		t.Fatal(err)
	}
	w.close()

	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", lines)
	}
}
