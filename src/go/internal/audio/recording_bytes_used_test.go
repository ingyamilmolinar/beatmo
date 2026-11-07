//go:build !js

package audio

import (
	"context"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// From per-channel sample counts × bytes per sample, so the perf snapshot needs no hot-path counter.
func TestPipelineStatsBytesUsed(t *testing.T) {
	t.Cleanup(env.RecordingsDir.SetForTest(t.TempDir()))

	if err := StartRecording(RecordingOptions{
		Format: FormatWAV24,
		Instruments: []InstrumentMeta{
			{ID: channel.Of(roster.Kick), Name: "Kick"},
		},
		BPM: 120,
	}); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	t.Cleanup(func() {
		if IsRecording() {
			_, _ = StopRecording()
		}
		// Finalize writes asynchronously on the lifecycle pool; wait for it before TempDir's RemoveAll (LIFO cleanup order),
		// or it trips "directory not empty".
		wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer wcancel()
		if err := WaitRecordingFinalized(wctx); err != nil {
			t.Errorf("WaitRecordingFinalized: %v", err)
		}
	})

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
	slotIDs := []channel.ID{channel.Of(roster.Kick)}
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
	// 3 bytes per sample (WAV24), master + 1 instrument.
	const wantPerChannel = blocks * blockLen * 3
	const wantTotal = wantPerChannel * 2
	// The worker may be mid-write, so only a sane range is checked.
	if stats.BytesUsed <= 0 {
		t.Errorf("BytesUsed=%d, want > 0", stats.BytesUsed)
	}
	if stats.BytesUsed > int64(wantTotal*2) {
		t.Errorf("BytesUsed=%d, want <= %d (sanity)", stats.BytesUsed, wantTotal*2)
	}
	t.Logf("BytesUsed=%d (expected ~%d for %d blocks × %d samples × 3 bytes × 2 channels)",
		stats.BytesUsed, wantTotal, blocks, blockLen)
}
