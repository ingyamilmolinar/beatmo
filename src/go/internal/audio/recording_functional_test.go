//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestPlatformCapturePerInstrumentChannels(t *testing.T) {
	// Save original callbacks and restore after test
	origStart := platformRecordingStart
	origStop := platformRecordingStop
	t.Cleanup(func() {
		platformRecordingStart = origStart
		platformRecordingStop = origStop
	})

	startCalled := false
	platformRecordingStart = func(instruments []InstrumentMeta) {
		startCalled = true
	}

	// Simulate platform capture returning per-instrument + master data
	platformRecordingStop = func() (map[channel.ID][]float64, []float64, int) {
		perInst := map[channel.ID][]float64{
			channel.Of(roster.Kick):  {0.1, 0.2, 0.3, 0.4},
			channel.Of(roster.Snare): {0.5, 0.6, 0.7, 0.8},
		}
		master := []float64{0.6, 0.8, 1.0, 0.5}
		return perInst, master, 44100
	}

	instruments := []InstrumentMeta{
		{ID: channel.Of(roster.Kick), Name: "Kick"},
		{ID: channel.Of(roster.Snare), Name: "Snare"},
	}
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: instruments,
		BPM:         120,
	}

	t.Cleanup(env.RecordingsDir.SetForTest(t.TempDir()))
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if !startCalled {
		t.Error("platformRecordingStart was not called")
	}

	// No Go-side capture: WASM mode, where the Go mixer does not run.

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	// Should have master + 2 instruments = 3 channels
	if len(result.Channels) != 3 {
		t.Errorf("channels = %d, want 3 (master + kick + snare)", len(result.Channels))
		for _, ch := range result.Channels {
			t.Logf("  channel: %s (%s) size=%d", ch.ID, ch.Filename, len(ch.Data))
		}
	}

	// Verify master channel
	var hasMaster, hasKick, hasSnare bool
	for _, ch := range result.Channels {
		if len(ch.Data) == 0 {
			t.Errorf("channel %s has empty data", ch.ID)
		}
		switch ch.ID {
		case "master":
			hasMaster = true
		case "kick":
			hasKick = true
		case "snare":
			hasSnare = true
		}
	}
	if !hasMaster {
		t.Error("missing master channel")
	}
	if !hasKick {
		t.Error("missing kick channel")
	}
	if !hasSnare {
		t.Error("missing snare channel")
	}

	// Verify sample rate from platform was used
	if result.Metadata.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", result.Metadata.SampleRate)
	}

	// Verify metadata has all channels
	if len(result.Metadata.Channels) != 3 {
		t.Errorf("metadata channels = %d, want 3", len(result.Metadata.Channels))
	}
}

func TestPlatformCaptureDoesNotOverrideGoCapture(t *testing.T) {
	origStart := platformRecordingStart
	origStop := platformRecordingStop
	t.Cleanup(func() {
		platformRecordingStart = origStart
		platformRecordingStop = origStop
	})

	platformRecordingStart = func(instruments []InstrumentMeta) {}
	platformRecordingStop = func() (map[channel.ID][]float64, []float64, int) {
		// Return some data — but it should be ignored since Go mixer has data
		return map[channel.ID][]float64{channel.Of(roster.Kick): {9.9}}, []float64{9.9}, 48000
	}

	instruments := []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}}
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: instruments,
		BPM:         120,
	}

	t.Cleanup(env.RecordingsDir.SetForTest(t.TempDir()))
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	// The desktop mixer's Tap on the audio thread.
	pipe := pipelinePtr.Load()
	if pipe == nil {
		t.Fatal("pipelinePtr should not be nil during recording")
	}
	instBufs := [][]float64{{0.1, 0.2, 0.3, 0.4}}
	workBuf := []float64{0.5, 0.6, 0.7, 0.8}
	pipe.Tap([]channel.ID{channel.Of(roster.Kick)}, []int{0}, instBufs, workBuf, 4)

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	// Should have Go-captured data, not platform data
	if len(result.Channels) != 2 {
		t.Errorf("channels = %d, want 2 (master + kick)", len(result.Channels))
	}

	// Sample rate should NOT be overridden by platform when Go captured data
	if result.Metadata.SampleRate == 48000 {
		t.Error("platform sample rate should not override Go capture sample rate")
	}
}

func TestDesktopRecordingPathConfigurable(t *testing.T) {
	// The stub's contract; real file I/O is tested in the !test build.
	result := &RecordingResult{
		Channels: []EncodedChannel{
			{ID: "master", Name: "Master", Filename: "master.wav", Data: []byte{1, 2, 3}},
			{ID: "kick", Name: "Kick", Filename: "kick.wav", Data: []byte{4, 5, 6}},
		},
		Metadata: SessionMetadata{
			Timestamp: "2026-03-22_150000",
		},
	}

	path, err := SaveRecording(result)
	if err != nil {
		t.Fatalf("SaveRecording: %v", err)
	}
	if path == "" {
		t.Error("SaveRecording returned empty path")
	}
}
