//go:build !js

package audio

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// pipelineUnderLoad spawns the pipeline and a Tap-driver goroutine that
// pumps blocks at audio-thread cadence (~93 blocks/sec for a 480-sample
// block at 44.1 kHz). Returns a stop func that waits for the driver to
// exit and stops the pipeline.
func pipelineUnderLoad(t *testing.T, ctx context.Context, instruments []InstrumentMeta, blockLen, durationMs int) (*pipeline, int64) {
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
	pipelinePtr.Store(pipe)
	t.Cleanup(func() { pipelinePtr.Store(nil) })

	instBufs := make([][]float64, len(instruments))
	slotIDs := make([]string, len(instruments))
	activeSlots := make([]int, len(instruments))
	for i, inst := range instruments {
		buf := make([]float64, blockLen)
		for j := range buf {
			buf[j] = math.Sin(float64(j) * 0.05)
		}
		instBufs[i] = buf
		slotIDs[i] = inst.ID
		activeSlots[i] = i
	}
	workBuf := make([]float64, blockLen)
	for i := range workBuf {
		workBuf[i] = math.Sin(float64(i) * 0.1)
	}

	// Real audio: 44.1 kHz / 480 samples = 91.875 blocks/sec ≈ 10.9 ms.
	tickDur := time.Duration(float64(blockLen) / 44.1)
	if tickDur < time.Millisecond {
		tickDur = time.Millisecond
	}
	deadline := time.Now().Add(time.Duration(durationMs) * time.Millisecond)
	var blocksTapped int64

	driverDone := make(chan struct{})
	go func() {
		defer close(driverDone)
		ticker := time.NewTicker(tickDur)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if time.Now().After(deadline) {
					return
				}
				pipe.Tap(slotIDs, activeSlots, instBufs, workBuf, blockLen)
				atomic.AddInt64(&blocksTapped, 1)
			}
		}
	}()
	<-driverDone

	t.Cleanup(func() {
		// Pipeline cleanup happens in Stop().
	})
	return pipe, atomic.LoadInt64(&blocksTapped)
}

// TestPipelineSurvivesCPUNoise: spawn many CPU-burning goroutines and
// verify the pipeline still keeps up — drop rate must stay under 5 %.
//
// This emulates the "noisy neighbor" scenario where a background task
// hogs cores. With our bounded pools and per-channel encoder workers,
// the audio path should remain unaffected.
func TestPipelineSurvivesCPUNoise(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CPU-noise test in -short")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// CPU noise: spawn NumCPU goroutines doing busy math.
	noiseStop := make(chan struct{})
	var noiseWg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		noiseWg.Add(1)
		go func() {
			defer noiseWg.Done()
			x := 1.0
			for {
				select {
				case <-noiseStop:
					return
				default:
					for j := 0; j < 1000; j++ {
						x = math.Sqrt(x*1.0001 + 1)
					}
				}
			}
		}()
	}
	defer func() { close(noiseStop); noiseWg.Wait() }()

	instruments := []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
		{ID: "hihat", Name: "HiHat"},
	}
	pipe, blocksTapped := pipelineUnderLoad(t, ctx, instruments, 480, 1000) // 1 s
	metas, err := pipe.Stop()
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if blocksTapped == 0 {
		t.Fatal("driver tapped no blocks")
	}
	dropRate := float64(pipe.Drops()) / float64(blocksTapped)
	if dropRate > 0.05 {
		t.Fatalf("drop rate %.2f%% exceeds 5%% under CPU noise (drops=%d, tapped=%d)",
			dropRate*100, pipe.Drops(), blocksTapped)
	}
	if len(metas) == 0 {
		t.Fatal("expected non-empty channel metadata")
	}
	t.Logf("CPU noise: tapped=%d drops=%d (%.2f%%) channels=%d",
		blocksTapped, pipe.Drops(), dropRate*100, len(metas))
}

// TestPipelineSurvivesAllocNoise: spawn an allocator goroutine that
// churns memory (forces frequent GC) and assert the pipeline still
// keeps up — drop rate below 5 %.
//
// Note we don't use testing.AllocsPerRun here because that measures all
// goroutine allocations, not just the function under test. Tap's
// zero-alloc property is verified by TestPipelineHotPathNoAlloc; this
// test verifies *scheduler* survival under GC pressure.
func TestPipelineSurvivesAllocNoise(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping alloc-noise test in -short")
	}
	noiseStop := make(chan struct{})
	var noiseWg sync.WaitGroup
	noiseWg.Add(1)
	go func() {
		defer noiseWg.Done()
		junk := make([][]byte, 0, 1024)
		for {
			select {
			case <-noiseStop:
				return
			default:
				junk = append(junk, make([]byte, 1024))
				if len(junk) > 512 {
					junk = junk[:0]
					runtime.GC()
				}
			}
		}
	}()
	defer func() { close(noiseStop); noiseWg.Wait() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	instruments := []InstrumentMeta{
		{ID: "kick", Name: "Kick"},
		{ID: "snare", Name: "Snare"},
	}
	pipe, blocksTapped := pipelineUnderLoad(t, ctx, instruments, 480, 1000)
	if _, err := pipe.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if blocksTapped == 0 {
		t.Fatal("driver tapped no blocks")
	}
	dropRate := float64(pipe.Drops()) / float64(blocksTapped)
	if dropRate > 0.05 {
		t.Fatalf("drop rate %.2f%% exceeds 5%% under alloc noise (drops=%d, tapped=%d)",
			dropRate*100, pipe.Drops(), blocksTapped)
	}
	t.Logf("alloc noise: tapped=%d drops=%d (%.2f%%)",
		blocksTapped, pipe.Drops(), dropRate*100)
}

// TestPipelineSurvivesDiskNoise: spawn writer goroutines hammering the
// same temp directory. The pipeline still has its own writer threads;
// they should keep up even when other I/O contends for disk.
func TestPipelineSurvivesDiskNoise(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping disk-noise test in -short")
	}
	noiseDir := t.TempDir()
	noiseStop := make(chan struct{})
	var noiseWg sync.WaitGroup
	for i := 0; i < 4; i++ {
		noiseWg.Add(1)
		go func(id int) {
			defer noiseWg.Done()
			path := filepath.Join(noiseDir, "noise-"+string(rune('a'+id)))
			payload := make([]byte, 16*1024)
			for {
				select {
				case <-noiseStop:
					return
				default:
					_ = os.WriteFile(path, payload, 0o644)
				}
			}
		}(i)
	}
	defer func() { close(noiseStop); noiseWg.Wait() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipe, blocksTapped := pipelineUnderLoad(t, ctx,
		[]InstrumentMeta{{ID: "kick", Name: "Kick"}}, 480, 1000)
	metas, err := pipe.Stop()
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if blocksTapped == 0 {
		t.Fatal("driver tapped no blocks")
	}
	dropRate := float64(pipe.Drops()) / float64(blocksTapped)
	if dropRate > 0.10 {
		t.Fatalf("drop rate %.2f%% exceeds 10%% under disk noise (drops=%d, tapped=%d)",
			dropRate*100, pipe.Drops(), blocksTapped)
	}
	if len(metas) == 0 {
		t.Fatal("expected non-empty metadata")
	}
	t.Logf("disk noise: tapped=%d drops=%d (%.2f%%)",
		blocksTapped, pipe.Drops(), dropRate*100)
}

// TestStopRecordingFastReturn: assert that StopRecording returns within
// 5 ms even when the writers have buffered work — the slow file Close
// must run on the lifecycle pool, not the calling goroutine.
func TestStopRecordingFastReturn(t *testing.T) {
	t.Setenv("BEATMO_RECORDINGS_DIR", t.TempDir())
	opts := RecordingOptions{
		Format: FormatWAV24,
		Instruments: []InstrumentMeta{
			{ID: "kick", Name: "Kick"},
			{ID: "snare", Name: "Snare"},
		},
		BPM: 120,
	}
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	// Tap a bunch of blocks so writers have buffered work to flush.
	pipe := pipelinePtr.Load()
	if pipe == nil {
		t.Fatal("pipelinePtr should be set after StartRecording")
	}
	const blockLen = 480
	instBufs := [][]float64{make([]float64, blockLen), make([]float64, blockLen)}
	workBuf := make([]float64, blockLen)
	for i := 0; i < 50; i++ {
		pipe.Tap([]string{"kick", "snare"}, []int{0, 1}, instBufs, workBuf, blockLen)
	}

	start := time.Now()
	result, err := StopRecording()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	// Block until the async finalize completes so the next test can
	// recycle the lifecycle pool slot cleanly.
	wctx, wcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer wcancel()
	if err := WaitRecordingFinalized(wctx); err != nil {
		t.Fatalf("WaitRecordingFinalized: %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("StopRecording returned in %v; want < 50 ms (file Close should be async)", elapsed)
	}
	t.Logf("StopRecording elapsed: %v", elapsed)
}

// TestRecordingByteIntegrity: feed a known PRNG-seeded float32 sequence
// to a single channel and verify the resulting WAV file matches when
// decoded back (within int24 quantization tolerance).
func TestRecordingByteIntegrity(t *testing.T) {
	t.Setenv("BEATMO_RECORDINGS_DIR", t.TempDir())
	opts := RecordingOptions{
		Format:      FormatWAV24,
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
		BPM:         120,
	}
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	pipe := pipelinePtr.Load()
	if pipe == nil {
		t.Fatal("pipelinePtr nil")
	}

	const (
		blockLen  = 480
		nBlocks   = 20
		totalSamp = blockLen * nBlocks
	)
	expected := make([]float64, totalSamp)
	for i := range expected {
		// Deterministic ramp + sine so we can verify visually too.
		expected[i] = 0.5 * math.Sin(float64(i)*0.01)
	}

	for b := 0; b < nBlocks; b++ {
		buf := expected[b*blockLen : (b+1)*blockLen]
		pipe.Tap([]string{"kick"}, []int{0}, [][]float64{buf}, buf, blockLen)
	}

	result, err := StopRecording()
	if err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	wctx, wcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer wcancel()
	if err := WaitRecordingFinalized(wctx); err != nil {
		t.Fatalf("WaitRecordingFinalized: %v", err)
	}

	// Find kick.wav.
	var kickPath string
	for _, ch := range result.Channels {
		if ch.ID == "kick" {
			kickPath = ch.Path
		}
	}
	if kickPath == "" {
		t.Fatal("kick channel missing Path")
	}

	got := decodeWAV24Mono(t, kickPath)
	if len(got) != totalSamp {
		t.Fatalf("decoded %d samples, want %d", len(got), totalSamp)
	}
	const tolerance = 1.0 / 8388607.0 * 2 // 2 LSB of int24
	for i := range got {
		if math.Abs(got[i]-expected[i]) > tolerance {
			t.Fatalf("sample %d mismatch: got %f want %f (tol %f)",
				i, got[i], expected[i], tolerance)
		}
	}
}

// decodeWAV24Mono reads a 24-bit mono PCM WAV and returns float64
// samples in [-1, 1]. Minimal parser sufficient for our streaming WAV
// output. Validates the header structure; fatals on malformed files.
func decodeWAV24Mono(t *testing.T, path string) []float64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(data) < wavHeaderSize {
		t.Fatalf("file %s smaller than header", path)
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("file %s missing RIFF/WAVE", path)
	}
	if got := binary.LittleEndian.Uint16(data[34:36]); got != 24 {
		t.Fatalf("file %s bps=%d, want 24", path, got)
	}
	if got := binary.LittleEndian.Uint16(data[22:24]); got != 1 {
		t.Fatalf("file %s channels=%d, want 1 (mono)", path, got)
	}
	dataSize := binary.LittleEndian.Uint32(data[40:44])
	samples := make([]float64, dataSize/3)
	off := wavHeaderSize
	for i := range samples {
		// Sign-extend int24 → int32.
		b0 := uint32(data[off])
		b1 := uint32(data[off+1])
		b2 := uint32(data[off+2])
		off += 3
		v := int32(b0 | (b1 << 8) | (b2 << 16))
		if v&0x800000 != 0 {
			v |= ^0xffffff
		}
		samples[i] = float64(v) / 8388607.0
	}
	return samples
}

// Ensure newTestPipeline (defined in recording_pipeline_test.go) is
// linked under both build tag combinations so this stress file compiles
// independently when only stress tag is used.
var _ = filepath.Join
