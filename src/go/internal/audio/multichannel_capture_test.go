//go:build test

package audio

import (
	"testing"
	"time"
)

func TestMIDINoteForInstrument(t *testing.T) {
	tests := []struct {
		id   string
		want int
	}{
		{"kick", 36},
		{"kick-deep", 35},
		{"snare", 38},
		{"hihat", 42},
		{"clap", 39},
		{"tom", 45},
		{"cowbell", 56},
		{"rimshot", 37},
		{"sidestick", 37},
		{"ride", 51},
		{"crash", 49},
		{"shaker", 70},
		{"unknown-inst", 38}, // default
		{"", 38},             // default
	}
	for _, tt := range tests {
		got := MIDINoteForInstrument(tt.id)
		if got != tt.want {
			t.Errorf("MIDINoteForInstrument(%q) = %d, want %d", tt.id, got, tt.want)
		}
	}
}

func TestAppendMIDIEvent(t *testing.T) {
	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	mc := newMultiChannelCapture(instruments, 44100, 0)

	evt := MIDIEvent{
		Time:    0.5,
		Type:    "note_on",
		Channel: 9,
		Note:    36,
		Velocity: 127,
		InstrID: "kick",
	}
	mc.AppendMIDIEvent(evt)

	if len(mc.midiEvents) != 1 {
		t.Fatalf("midiEvents length = %d, want 1", len(mc.midiEvents))
	}
	if mc.midiEvents[0].Note != 36 {
		t.Errorf("MIDI note = %d, want 36", mc.midiEvents[0].Note)
	}
	if mc.midiEvents[0].InstrID != "kick" {
		t.Errorf("MIDI instrID = %s, want kick", mc.midiEvents[0].InstrID)
	}
}

func TestMultiChannelCaptureBasic(t *testing.T) {
	instruments := []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	}
	mc := newMultiChannelCapture(instruments, 44100, 0)

	// Simulate 2 blocks of audio
	instBufs := [][]float64{
		{0.1, 0.2, 0.3, 0.4}, // slot 0 = kick
		{0.5, 0.6, 0.7, 0.8}, // slot 1 = snare
	}
	slotIDs := []string{"kick", "snare"}
	activeSlots := []int{0, 1}
	workBuf := []float64{0.6, 0.8, 1.0, 1.2}

	mc.appendBlock(instBufs, slotIDs, activeSlots, workBuf, 4)

	// Verify master
	if len(mc.masterCh.Samples) != 4 {
		t.Fatalf("master samples = %d, want 4", len(mc.masterCh.Samples))
	}
	if mc.masterCh.Samples[0] != 0.6 {
		t.Errorf("master[0] = %f, want 0.6", mc.masterCh.Samples[0])
	}

	// Verify per-instrument
	kickCh := mc.channels["kick"]
	if kickCh == nil {
		t.Fatal("kick channel not found")
	}
	if len(kickCh.Samples) != 4 {
		t.Fatalf("kick samples = %d, want 4", len(kickCh.Samples))
	}
	if kickCh.Samples[0] != 0.1 {
		t.Errorf("kick[0] = %f, want 0.1", kickCh.Samples[0])
	}

	snareCh := mc.channels["snare"]
	if snareCh == nil {
		t.Fatal("snare channel not found")
	}
	if snareCh.Samples[0] != 0.5 {
		t.Errorf("snare[0] = %f, want 0.5", snareCh.Samples[0])
	}
}

func TestMultiChannelCaptureDurationLimit(t *testing.T) {
	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	// 10 samples max at 100 Hz = 0.1 seconds
	mc := newMultiChannelCapture(instruments, 100, 100*time.Millisecond)

	instBufs := [][]float64{{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}}
	slotIDs := []string{"kick"}
	workBuf := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}

	// First block: 8 samples
	mc.appendBlock(instBufs, slotIDs, []int{0}, workBuf, 8)
	if mc.totalSamples != 8 {
		t.Fatalf("totalSamples = %d, want 8", mc.totalSamples)
	}

	// Second block: should be truncated to 2 samples (10 - 8 = 2)
	mc.appendBlock(instBufs, slotIDs, []int{0}, workBuf, 8)
	if mc.totalSamples != 10 {
		t.Fatalf("totalSamples = %d, want 10", mc.totalSamples)
	}

	if !mc.isDone() {
		t.Error("expected isDone() = true after reaching max samples")
	}
}

func TestMultiChannelCaptureSnapshot(t *testing.T) {
	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	mc := newMultiChannelCapture(instruments, 44100, 0)

	instBufs := [][]float64{{0.1, 0.2}}
	mc.appendBlock(instBufs, []string{"kick"}, []int{0}, []float64{0.3, 0.4}, 2)

	channels, master := mc.snapshot()

	// Verify snapshot is a copy (modifying original shouldn't affect snapshot)
	mc.masterCh.Samples[0] = 999
	if master.Samples[0] == 999 {
		t.Error("snapshot master is not a copy")
	}

	kickCh := channels["kick"]
	if kickCh == nil {
		t.Fatal("kick not in snapshot")
	}
	if len(kickCh.Samples) != 2 {
		t.Errorf("kick snapshot len = %d, want 2", len(kickCh.Samples))
	}
}

func TestMultiChannelCaptureAppendAfterDone(t *testing.T) {
	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	// 4 samples max
	mc := newMultiChannelCapture(instruments, 100, 40*time.Millisecond)

	instBufs := [][]float64{{0.1, 0.2, 0.3, 0.4, 0.5}}
	slotIDs := []string{"kick"}
	workBuf := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	// Fill to max
	mc.appendBlock(instBufs, slotIDs, []int{0}, workBuf, 4)
	if !mc.isDone() {
		t.Fatal("should be done after reaching max")
	}

	// Subsequent appends should be no-ops
	prevSamples := mc.totalSamples
	mc.appendBlock(instBufs, slotIDs, []int{0}, workBuf, 4)
	if mc.totalSamples != prevSamples {
		t.Errorf("totalSamples changed after done: %d → %d", prevSamples, mc.totalSamples)
	}
}

func TestMultiChannelCaptureDynamicChannel(t *testing.T) {
	// Test channel created on-the-fly when instrument appears after recording start
	instruments := []InstrumentMeta{{ID: "kick", Name: "Kick"}}
	mc := newMultiChannelCapture(instruments, 44100, 0)

	// First block with kick
	instBufs := [][]float64{{0.1, 0.2}, {0.3, 0.4}}
	slotIDs := []string{"kick", "snare"}
	mc.appendBlock(instBufs, slotIDs, []int{0}, []float64{0.5, 0.6}, 2)

	// Second block with snare (new channel)
	mc.appendBlock(instBufs, slotIDs, []int{0, 1}, []float64{0.7, 0.8}, 2)

	// Snare should exist now
	if _, ok := mc.channels["snare"]; !ok {
		t.Error("snare channel should be created dynamically")
	}
}

func TestMultiChannelCaptureAlignment(t *testing.T) {
	instruments := []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	}
	mc := newMultiChannelCapture(instruments, 44100, 0)

	// Block 1: only kick active
	instBufs := [][]float64{{0.1, 0.2}, {0, 0}}
	mc.appendBlock(instBufs, []string{"kick", "snare"}, []int{0}, []float64{0.1, 0.2}, 2)

	// Block 2: only snare active
	mc.appendBlock(instBufs, []string{"kick", "snare"}, []int{1}, []float64{0.3, 0.4}, 2)

	// Both channels should be 4 samples (padded with silence)
	kickCh := mc.channels["kick"]
	snareCh := mc.channels["snare"]

	if len(kickCh.Samples) != 4 {
		t.Errorf("kick samples = %d, want 4", len(kickCh.Samples))
	}
	if len(snareCh.Samples) != 4 {
		t.Errorf("snare samples = %d, want 4", len(snareCh.Samples))
	}

	// Snare should have silence for first 2 samples
	if snareCh.Samples[0] != 0 || snareCh.Samples[1] != 0 {
		t.Error("snare should have silence for first 2 samples")
	}
}
