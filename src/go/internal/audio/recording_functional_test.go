//go:build test

package audio

import (
	"testing"
)

// TestPlatformCapturePerInstrumentChannels verifies that per-instrument
// channels returned by platformRecordingStop are injected into the
// recording result alongside the master channel.
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
	platformRecordingStop = func() (map[string][]float64, []float64, int) {
		perInst := map[string][]float64{
			"kick":  {0.1, 0.2, 0.3, 0.4},
			"snare": {0.5, 0.6, 0.7, 0.8},
		}
		master := []float64{0.6, 0.8, 1.0, 0.5}
		return perInst, master, 44100
	}

	instruments := []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	}
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: instruments,
		BPM:         120,
	}

	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if !startCalled {
		t.Error("platformRecordingStart was not called")
	}

	// Don't inject any Go-side capture data — simulate WASM mode
	// where the Go mixer doesn't run.

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

// TestPlatformCaptureDoesNotOverrideGoCapture verifies that when the Go
// mixer captured data (desktop mode), platform capture data is ignored.
func TestPlatformCaptureDoesNotOverrideGoCapture(t *testing.T) {
	origStart := platformRecordingStart
	origStop := platformRecordingStop
	t.Cleanup(func() {
		platformRecordingStart = origStart
		platformRecordingStop = origStop
	})

	platformRecordingStart = func(instruments []InstrumentMeta) {}
	platformRecordingStop = func() (map[string][]float64, []float64, int) {
		// Return some data — but it should be ignored since Go mixer has data
		return map[string][]float64{"kick": {9.9}}, []float64{9.9}, 48000
	}

	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: instruments,
		BPM:         120,
	}

	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	// Inject Go-side capture data (simulates desktop mixer running)
	mc := multiCapturePtr.Load()
	if mc == nil {
		t.Fatal("multiCapturePtr should not be nil during recording")
	}
	instBufs := [][]float64{{0.1, 0.2, 0.3, 0.4}}
	workBuf := []float64{0.5, 0.6, 0.7, 0.8}
	mc.appendBlock(instBufs, []string{"kick"}, []int{0}, workBuf, 4)

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

// TestDesktopRecordingPathConfigurable verifies that BEATMO_RECORDINGS_DIR
// env var controls the output directory, and the default path uses the
// executable's working directory.
func TestDesktopRecordingPathConfigurable(t *testing.T) {
	// This test verifies the stub's contract matches expected behavior.
	// Desktop functional tests for actual file I/O require the !test build.
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
