package audio

import (
	"math"
	"testing"
)

// mockBlockProcessor implements both Processor and BlockProcessor.
// Multiplies every sample by gain.
type mockBlockProcessor struct {
	gain float64
}

func (m *mockBlockProcessor) ProcessSample(x float64) float64 {
	return x * m.gain
}

func (m *mockBlockProcessor) ProcessBlockBuf(in, out []float32, samples int) {
	for i := 0; i < samples; i++ {
		out[i] = in[i] * float32(m.gain)
	}
}

// mockSampleOnlyProcessor implements only Processor (not BlockProcessor).
type mockSampleOnlyProcessor struct {
	gain float64
}

func (m *mockSampleOnlyProcessor) ProcessSample(x float64) float64 {
	return x * m.gain
}

func TestProcessBlockLocalAllBlockProcessors(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("bp-all-block", nil)
	ch.SetVolume(1.0)
	ch.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 0.5},
		&mockBlockProcessor{gain: 0.8},
	})

	input := make([]float64, 64)
	output := make([]float64, 64)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	expected := 1.0 * 0.5 * 0.8
	for i, v := range output {
		if math.Abs(v-expected) > 0.001 {
			t.Errorf("sample[%d] = %f, want %f", i, v, expected)
			break
		}
	}
}

func TestProcessBlockLocalMixedProcessors(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("bp-mixed", nil)
	ch.SetVolume(1.0)
	ch.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 0.5},
		&mockSampleOnlyProcessor{gain: 0.8}, // forces per-sample fallback
	})

	input := make([]float64, 64)
	output := make([]float64, 64)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	expected := 1.0 * 0.5 * 0.8
	for i, v := range output {
		if math.Abs(v-expected) > 0.001 {
			t.Errorf("sample[%d] = %f, want %f", i, v, expected)
			break
		}
	}
}

func TestProcessBlockLocalNoProcessors(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("bp-none", nil)
	ch.SetVolume(0.75)
	ch.replaceProcessors(nil)

	input := make([]float64, 32)
	output := make([]float64, 32)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	for i, v := range output {
		if math.Abs(v-0.75) > 0.001 {
			t.Errorf("sample[%d] = %f, want 0.75 (volume only)", i, v)
			break
		}
	}
}

func TestBlockBufPoolGetPut(t *testing.T) {
	pool := newBlockPool()
	buf := pool.get(128)
	if len(buf) != 128 {
		t.Fatalf("pool.get(128) len = %d, want 128", len(buf))
	}
	ptr := &buf[0]
	pool.put(buf)

	// get again should reuse the same backing array
	buf2 := pool.get(128)
	if &buf2[0] != ptr {
		t.Error("pool.get after put did not reuse buffer")
	}
}

func TestBlockBufPoolMultipleBuffers(t *testing.T) {
	pool := newBlockPool()
	buf1 := pool.get(64)
	buf2 := pool.get(64)
	if &buf1[0] == &buf2[0] {
		t.Error("two concurrent gets returned same buffer")
	}
}

func TestProcessBlockLocalPreEQAnalyzerTap(t *testing.T) {
	withDefaultAudio(t)
	id := "bp-preeq"
	_ = InstrumentChannel(id)
	an := EnablePreEQAnalyzer(id, 64)

	ch := chanMgr.ensureChannel(id)
	ch.SetVolume(0.5)

	input := make([]float64, 64)
	output := make([]float64, 64)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	snap := an.Snapshot()
	// Pre-EQ tap is after volume: 1.0 * 0.5 = 0.5, RMS should be 0.5
	if snap.RMS < 0.4 || snap.RMS > 0.6 {
		t.Errorf("pre-EQ analyzer RMS = %f, want ≈ 0.5", snap.RMS)
	}
}

func TestProcessBlockLocalPingPongCorrectness(t *testing.T) {
	// Verify that block path and per-sample path produce identical output.
	withDefaultAudio(t)
	n := 137 // odd size

	input := make([]float64, n)
	for i := range input {
		input[i] = float64(i+1) * 0.01
	}

	// Block path: all BlockProcessors
	chBlock := newChannel("pp-block", nil)
	chBlock.SetVolume(1.0)
	chBlock.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 0.6},
		&mockBlockProcessor{gain: 0.4},
	})
	outBlock := make([]float64, n)
	chBlock.ProcessBlockLocal(input, outBlock)

	// Per-sample path: all sample-only processors with same gains
	chSample := newChannel("pp-sample", nil)
	chSample.SetVolume(1.0)
	chSample.replaceProcessors([]Processor{
		&mockSampleOnlyProcessor{gain: 0.6},
		&mockSampleOnlyProcessor{gain: 0.4},
	})
	outSample := make([]float64, n)
	chSample.ProcessBlockLocal(input, outSample)

	for i := 0; i < n; i++ {
		if math.Abs(outBlock[i]-outSample[i]) > 1e-6 {
			t.Errorf("sample[%d]: block=%f sample=%f", i, outBlock[i], outSample[i])
			break
		}
	}
}

func TestPassThroughImplementsBlockProcessor(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("bp-passthrough", nil)
	ch.SetVolume(1.0)
	pt := &passThrough{}
	ch.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 0.5},
		pt,
	})

	ch.mu.RLock()
	allBlock := ch.allBlock
	ch.mu.RUnlock()
	if !allBlock {
		t.Fatal("channel with passThrough + mockBlockProcessor should have allBlock=true")
	}

	input := make([]float64, 32)
	output := make([]float64, 32)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	for i, v := range output {
		if math.Abs(v-0.5) > 0.001 {
			t.Errorf("sample[%d] = %f, want 0.5 (passThrough should not alter signal)", i, v)
			break
		}
	}
}

func TestAnalyzerImplementsBlockProcessor(t *testing.T) {
	withDefaultAudio(t)
	id := "bp-analyzer"
	_ = InstrumentChannel(id)
	an := EnableChannelAnalyzer(id, 64)

	// Add a block-capable processor alongside the analyzer.
	ch := chanMgr.ensureChannel(id)
	ch.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 0.5},
		an,
	})

	// Verify allBlock is true (analyzer satisfies BlockProcessor).
	ch.mu.RLock()
	allBlock := ch.allBlock
	ch.mu.RUnlock()
	if !allBlock {
		t.Fatal("channel with Analyzer + mockBlockProcessor should have allBlock=true")
	}

	// Verify output is correct (gain 0.5, analyzer pass-through).
	input := make([]float64, 64)
	output := make([]float64, 64)
	for i := range input {
		input[i] = 1.0
	}
	ch.ProcessBlockLocal(input, output)

	for i, v := range output {
		if math.Abs(v-0.5) > 0.001 {
			t.Errorf("sample[%d] = %f, want 0.5", i, v)
			break
		}
	}

	// Analyzer should have received data.
	snap := an.Snapshot()
	if snap.RMS < 0.1 {
		t.Errorf("analyzer RMS = %f, want > 0.1 (should have received signal)", snap.RMS)
	}
}

func TestAnalyzerDisabledSkipsCompute(t *testing.T) {
	withDefaultAudio(t)
	id := "bp-analyzer-disabled"
	_ = InstrumentChannel(id)
	an := EnableChannelAnalyzer(id, 64)

	// Disable the analyzer.
	an.SetEnabled(false)
	if an.Enabled() {
		t.Fatal("Enabled() should return false after SetEnabled(false)")
	}

	ch := chanMgr.ensureChannel(id)
	ch.SetVolume(1.0)
	ch.replaceProcessors([]Processor{an})

	// Feed signal through — audio should still pass.
	input := make([]float64, 128)
	output := make([]float64, 128)
	for i := range input {
		input[i] = 0.9
	}
	ch.ProcessBlockLocal(input, output)

	for i, v := range output {
		if math.Abs(v-0.9) > 0.001 {
			t.Errorf("sample[%d] = %f, want 0.9 (audio should pass through even when disabled)", i, v)
			break
		}
	}

	// Snapshot should have stale/zero data since compute was skipped.
	snap := an.Snapshot()
	if snap.RMS > 0.01 {
		t.Errorf("analyzer RMS = %f, want ≈ 0 (compute should be skipped when disabled)", snap.RMS)
	}

	// Re-enable and verify compute resumes.
	an.SetEnabled(true)
	output2 := make([]float64, 128)
	ch.ProcessBlockLocal(input, output2)
	snap2 := an.Snapshot()
	if snap2.RMS < 0.1 {
		t.Errorf("analyzer RMS = %f, want > 0.1 after re-enabling", snap2.RMS)
	}
}

func TestSetAnalyzerEnabledPackageLevel(t *testing.T) {
	withDefaultAudio(t)
	id := "bp-analyzer-pkg"
	_ = InstrumentChannel(id)
	an := EnableChannelAnalyzer(id, 64)
	pre := EnablePreEQAnalyzer(id, 64)

	// Both should start enabled.
	if !an.Enabled() || !pre.Enabled() {
		t.Fatal("analyzers should be enabled by default")
	}

	// Disable via package-level API.
	SetAnalyzerEnabled(id, false)
	if an.Enabled() || pre.Enabled() {
		t.Error("SetAnalyzerEnabled(false) should disable both analyzers")
	}

	// Re-enable.
	SetAnalyzerEnabled(id, true)
	if !an.Enabled() || !pre.Enabled() {
		t.Error("SetAnalyzerEnabled(true) should re-enable both analyzers")
	}
}

func TestProcessBlockLocalOddBlockSize(t *testing.T) {
	withDefaultAudio(t)
	ch := newChannel("bp-odd", nil)
	ch.SetVolume(1.0)
	ch.replaceProcessors([]Processor{
		&mockBlockProcessor{gain: 2.0},
	})

	n := 137
	input := make([]float64, n)
	output := make([]float64, n)
	for i := range input {
		input[i] = 0.3
	}
	ch.ProcessBlockLocal(input, output)

	for i, v := range output {
		if math.Abs(v-0.6) > 0.001 {
			t.Errorf("sample[%d] = %f, want 0.6", i, v)
			break
		}
	}
}
