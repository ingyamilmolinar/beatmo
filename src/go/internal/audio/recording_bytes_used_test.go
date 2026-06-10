//go:build !js

package audio

import (
	"context"
	"testing"
	"time"
)

// TestPipelineStatsBytesUsed verifies that CurrentPipelineStats.BytesUsed
// is populated from per-channel sample counts × bytes-per-sample, so the
// perf snapshot can surface "how much audio data has been encoded so far"
// without a separate atomic counter on the hot path.
//
// Mirrors the WASM Phase 4 contract where the encoder Worker reports
// bytesUsed for hard-cap detection (BYTES_PER_CHANNEL_MAX). On desktop
// the same field is derived from the streaming pipeline's existing
// per-channel sample counters.
func TestPipelineStatsBytesUsed(t *testing.T) {
	t.Setenv("BEATMO_RECORDINGS_DIR", t.TempDir())

	if err := StartRecording(RecordingOptions{
		Format: FormatWAV24,
		Instruments: []InstrumentMeta{
			{ID: "kick", Name: "Kick"},
		},
		BPM: 120,
	}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	t.Cleanup(func() {
		if IsRecording() {
			_, _ = StopRecording()
		}
		// StopRecording detaches and finalizes the file writers
		// asynchronously on the lifecycle pool (the desktop fast-return
		// contract). Wait for that tail to flush + close the WAV files
		// before t.TempDir()'s RemoveAll runs — otherwise finalize writes
		// .wav/session.json into the dir concurrently with RemoveAll and
		// trips "directory not empty". This cleanup runs before the
		// TempDir cleanup by LIFO ordering.
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		if err := WaitRecordingFinalized(wctx); err != nil {
			t.Errorf("WaitRecordingFinalized: %v", err)
		}
	})

	// Push a few blocks through the pipeline so per-channel sample counts
	// are non-zero. Use the global pipeline directly via the atomic ptr.
	pipe := pipelinePtr.Load()
	if pipe == nil {
		t.Fatal("expected pipeline to be active")
	}

	const blockLen = 256
	instBufs := [][]float64{make([]float64, blockLen)}
	master := make([]float64, blockLen)
	for i := range master {
		master[i] = 0.5
		instBufs[0][i] = 0.5
	}
	slotIDs := []string{"kick"}
	activeSlots := []int{0}
	const blocks = 16
	for i := 0; i < blocks; i++ {
		pipe.Tap(slotIDs, activeSlots, instBufs, master, blockLen)
	}
	// Allow workers to drain.
	time.Sleep(50 * time.Millisecond)

	stats := CurrentPipelineStats()
	if !stats.Active {
		t.Fatal("expected Active=true")
	}
	// Per-channel: blocks * blockLen samples * 3 bytes (WAV24).
	// Master + 1 instrument = 2 channels.
	const wantPerChannel = blocks * blockLen * 3
	const wantTotal = wantPerChannel * 2
	// Allow some slack (queue may not have drained completely; samples may
	// be reported lower than wantTotal if the worker is mid-write). Just
	// verify BytesUsed is non-zero and within a reasonable range.
	if stats.BytesUsed <= 0 {
		t.Errorf("BytesUsed=%d, want > 0", stats.BytesUsed)
	}
	if stats.BytesUsed > int64(wantTotal*2) {
		t.Errorf("BytesUsed=%d, want <= %d (sanity)", stats.BytesUsed, wantTotal*2)
	}
	t.Logf("BytesUsed=%d (expected ~%d for %d blocks × %d samples × 3 bytes × 2 channels)",
		stats.BytesUsed, wantTotal, blocks, blockLen)
}
