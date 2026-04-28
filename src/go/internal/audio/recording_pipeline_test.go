//go:build !js

package audio

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func newTestPipeline(t *testing.T, instruments []InstrumentMeta) *pipeline {
	t.Helper()
	dir := t.TempDir()
	pipe, err := newPipeline(pipelineConfig{
		OutputDir:     dir,
		SampleRate:    44100,
		Format:        FormatWAV24,
		Instruments:   instruments,
		BlockCapacity: 4096,
		PoolSize:      32,
		QueueDepth:    64,
	})
	if err != nil {
		t.Fatalf("newPipeline: %v", err)
	}
	return pipe
}

// TestPipelineHotPathNoAlloc verifies that the audio-thread Tap call does
// not allocate on the steady-state hot path. This is the central
// performance invariant — any allocation in Tap regresses the desktop
// recording stutter fix.
func TestPipelineHotPathNoAlloc(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	})
	t.Cleanup(func() { pipe.Stop() })

	const blockLen = 512
	instBufs := [][]float64{
		make([]float64, blockLen),
		make([]float64, blockLen),
	}
	for i := range instBufs[0] {
		instBufs[0][i] = float64(i) * 0.0001
		instBufs[1][i] = float64(i) * -0.0001
	}
	workBuf := make([]float64, blockLen)
	for i := range workBuf {
		workBuf[i] = float64(i) * 0.00005
	}
	slotIDs := []string{"kick", "snare"}
	activeSlots := []int{0, 1}

	// Warm pool, workers, scratch buffers.
	for i := 0; i < 8; i++ {
		pipe.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
	}
	// Let the workers drain so blocks return to the pool before the alloc
	// run — otherwise we measure pool-empty allocations from the hot path.
	for pipe.dropsTotal.Load() == 0 && len(pipe.blockPool) < 16 {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}

	allocs := testing.AllocsPerRun(200, func() {
		pipe.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
		// Drain pressure between iterations.
		runtime.Gosched()
	})
	if allocs > 0 {
		t.Fatalf("Tap allocates %.2f times per call; want 0", allocs)
	}
}

// TestPipelineDropsOnBackpressure forces the per-channel queue and the
// block pool to saturate, then verifies drops are counted (not queued).
func TestPipelineDropsOnBackpressure(t *testing.T) {
	dir := t.TempDir()
	pipe, err := newPipeline(pipelineConfig{
		OutputDir:     dir,
		SampleRate:    44100,
		Format:        FormatWAV24,
		Instruments:   []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BlockCapacity: 64,
		PoolSize:      2, // tiny pool — saturate quickly
		QueueDepth:    1, // tiny queue
	})
	if err != nil {
		t.Fatalf("newPipeline: %v", err)
	}
	t.Cleanup(func() { pipe.Stop() })

	blockLen := 64
	instBufs := [][]float64{make([]float64, blockLen)}
	workBuf := make([]float64, blockLen)
	slotIDs := []string{"kick"}
	activeSlots := []int{0}

	// Hammer Tap without giving workers time to drain.
	for i := 0; i < 1000; i++ {
		pipe.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
	}

	if pipe.Drops() == 0 {
		t.Fatal("expected non-zero drop count under saturation")
	}
}

// TestPipelineWritesWAVFiles end-to-end test: tap a few blocks of a tone
// and verify per-channel WAV files exist on disk after Stop.
func TestPipelineWritesWAVFiles(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	})

	blockLen := 256
	instBufs := [][]float64{
		make([]float64, blockLen),
		make([]float64, blockLen),
	}
	for i := range instBufs[0] {
		instBufs[0][i] = 0.5
		instBufs[1][i] = 0.25
	}
	workBuf := make([]float64, blockLen)
	for i := range workBuf {
		workBuf[i] = 0.75
	}
	slotIDs := []string{"kick", "snare"}
	activeSlots := []int{0, 1}

	for i := 0; i < 4; i++ {
		pipe.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
	}

	metas, err := pipe.Stop()
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if len(metas) == 0 {
		t.Fatal("Stop returned no channel metadata")
	}

	for _, ch := range pipe.Channels() {
		if ch.Path == "" {
			t.Errorf("channel %s missing Path", ch.ID)
			continue
		}
		info, err := os.Stat(ch.Path)
		if err != nil {
			t.Errorf("stat %s: %v", ch.Path, err)
			continue
		}
		if info.Size() <= int64(wavHeaderSize) {
			t.Errorf("%s file size %d <= header size %d", ch.Path, info.Size(), wavHeaderSize)
		}
		if filepath.Base(ch.Path) != ch.Filename {
			t.Errorf("filename mismatch: %s vs %s", ch.Filename, filepath.Base(ch.Path))
		}
	}
}

// TestPipelineUnknownInstrumentDropped: tap with a slotID not in the
// pre-registered channel list. The block must be silently ignored
// (counted as a no-op, no panic, no allocation explosion).
func TestPipelineUnknownInstrumentDropped(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	blockLen := 64
	instBufs := [][]float64{make([]float64, blockLen)}
	workBuf := make([]float64, blockLen)
	// "snare" is not registered — should be silently skipped.
	pipe.Tap([]string{"snare"}, []int{0}, instBufs, workBuf, blockLen)

	// Nothing should have been written to the kick channel either, since
	// the only active slot was snare.
	if pipe.channels["kick"].count.Load() != 0 {
		t.Fatal("kick should not have received any samples")
	}
}

// TestPipelineStopAfterStopReturnsError: calling Stop twice should error.
func TestPipelineStopAfterStopReturnsError(t *testing.T) {
	pipe := newTestPipeline(t, nil)
	if _, err := pipe.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if _, err := pipe.Stop(); err == nil {
		t.Fatal("second Stop should error")
	}
}
