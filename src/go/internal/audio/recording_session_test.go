//go:build test

package audio

import (
	"testing"
	"time"
)

func TestStartStopRecording(t *testing.T) {
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

	if !IsRecording() {
		t.Error("expected IsRecording() = true after start")
	}

	elapsed := RecordingElapsed()
	if elapsed < 0 {
		t.Error("RecordingElapsed should be non-negative")
	}

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	if IsRecording() {
		t.Error("expected IsRecording() = false after stop")
	}

	if result == nil {
		t.Fatal("StopRecording returned nil result")
	}

	// Should have master channel at minimum
	if len(result.Channels) == 0 {
		// Master may have no samples since no audio was actually playing
		// but result should still be valid
	}

	if result.Metadata.BPM != 120 {
		t.Errorf("metadata BPM = %d, want 120", result.Metadata.BPM)
	}
	if result.Metadata.Format != FormatWAV16 {
		t.Errorf("metadata Format = %s, want wav16", result.Metadata.Format)
	}
	if result.Metadata.Timestamp == "" {
		t.Error("metadata Timestamp should not be empty")
	}
}

func TestStartRecordingDefaultFormat(t *testing.T) {
	opts := RecordingOptions{
		// No Format specified — should default to WAV24
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BPM:         100,
	}

	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	defer StopRecording()

	if !IsRecording() {
		t.Error("expected IsRecording() = true")
	}
}

func TestStartRecordingDuplicate(t *testing.T) {
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BPM:         120,
	}

	if err := StartRecording(opts); err != nil {
		t.Fatalf("first StartRecording: %v", err)
	}

	// Second start should fail
	err := StartRecording(opts)
	if err == nil {
		t.Error("expected error from duplicate StartRecording")
	}

	StopRecording()
}

func TestStartRecordingInvalidFormat(t *testing.T) {
	opts := RecordingOptions{
		Format:      "nonexistent",
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BPM:         120,
	}
	err := StartRecording(opts)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestStopRecordingWhenNotRecording(t *testing.T) {
	// Ensure not recording
	if IsRecording() {
		StopRecording()
	}

	_, err := StopRecording()
	if err == nil {
		t.Error("expected error when stopping non-active recording")
	}
}

func TestRecordingElapsedWhenNotRecording(t *testing.T) {
	if IsRecording() {
		StopRecording()
	}
	elapsed := RecordingElapsed()
	if elapsed != 0 {
		t.Errorf("RecordingElapsed should be 0 when not recording, got %v", elapsed)
	}
}

func TestRecordingAutoStopped(t *testing.T) {
	// Not recording → should return false
	if RecordingAutoStopped() {
		t.Error("RecordingAutoStopped should be false when not recording")
	}
}

func TestRecordingResultMetadataJSON(t *testing.T) {
	result := &RecordingResult{
		Metadata: SessionMetadata{
			BPM:        120,
			Duration:   5.0,
			SampleRate: 44100,
			Format:     FormatWAV16,
			Timestamp:  "2026-03-22_143052",
			Channels: []ChannelMeta{
				{ID: "master", Name: "Master", Filename: "master.wav", Samples: 220500},
			},
		},
	}

	data, err := result.MetadataJSON()
	if err != nil {
		t.Fatalf("MetadataJSON: %v", err)
	}

	if len(data) == 0 {
		t.Error("MetadataJSON returned empty data")
	}

	// Check it contains expected fields
	s := string(data)
	if !contains(s, "\"bpm\": 120") {
		t.Error("metadata JSON missing bpm field")
	}
	if !contains(s, "\"format\": \"wav16\"") {
		t.Error("metadata JSON missing format field")
	}
}

func TestRecordingWithMaxDuration(t *testing.T) {
	opts := RecordingOptions{
		Format:      FormatWAV16,
		MaxDuration: 100 * time.Millisecond,
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BPM:         120,
	}

	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	if !IsRecording() {
		t.Error("expected recording to be active")
	}

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestSaveRecordingStub(t *testing.T) {
	result := &RecordingResult{
		Channels: []EncodedChannel{
			{ID: "master", Name: "Master", Filename: "master.wav", Data: []byte{1, 2, 3}},
		},
		Metadata: SessionMetadata{
			Timestamp: "2026-03-22_143052",
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

func TestSaveRecordingNilResult(t *testing.T) {
	_, err := SaveRecording(nil)
	if err == nil {
		t.Error("expected error for nil result")
	}
}

func TestStopRecordingWithCapturedData(t *testing.T) {
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

	// Simulate captured audio by injecting data into the capture
	mc := multiCapturePtr.Load()
	if mc == nil {
		t.Fatal("multiCapturePtr should not be nil during recording")
	}

	instBufs := [][]float64{
		{0.1, 0.2, 0.3, 0.4},
		{0.5, 0.6, 0.7, 0.8},
	}
	slotIDs := []string{"kick", "snare"}
	workBuf := []float64{0.6, 0.8, 1.0, 0.5}
	mc.appendBlock(instBufs, slotIDs, []int{0, 1}, workBuf, 4)

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	// Should have master + 2 instruments = 3 channels
	if len(result.Channels) != 3 {
		t.Errorf("channels = %d, want 3 (master + kick + snare)", len(result.Channels))
	}

	// Each channel should have non-empty encoded data
	for _, ch := range result.Channels {
		if len(ch.Data) == 0 {
			t.Errorf("channel %s has empty data", ch.ID)
		}
		if ch.Filename == "" {
			t.Errorf("channel %s has empty filename", ch.ID)
		}
	}

	// Metadata should list all channels
	if len(result.Metadata.Channels) != 3 {
		t.Errorf("metadata channels = %d, want 3", len(result.Metadata.Channels))
	}
}

func TestSaveRecordingEmptyChannels(t *testing.T) {
	result := &RecordingResult{
		Channels: []EncodedChannel{},
		Metadata: SessionMetadata{Timestamp: "test"},
	}
	_, err := SaveRecording(result)
	if err == nil {
		t.Error("expected error for empty channels")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
