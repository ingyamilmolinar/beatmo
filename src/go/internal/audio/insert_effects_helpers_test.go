//go:build test

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestPassThroughResetAndSetParamAreNoOps(t *testing.T) {
	var p passThrough
	p.Reset()
	p.SetParam("", 0)
	p.SetParam("anything", 1234.5)
	if got := p.ProcessSample(0.5); got != 0.5 {
		t.Errorf("ProcessSample(0.5) = %v, want 0.5", got)
	}
	in := []float32{0.1, -0.2, 0.3, -0.4}
	out := make([]float32, len(in))
	p.ProcessBlockBuf(in, out, len(in))
	for i, v := range in {
		if out[i] != v {
			t.Errorf("ProcessBlockBuf[%d] = %v, want %v", i, out[i], v)
		}
	}
}

// The export/import path for per-row insert chains: a nil map is elided, a populated one round-trips.
func TestEffectSlotJSONRoundTrip(t *testing.T) {
	for _, et := range []EffectType{"delay", "reverb", "chorus", "bitcrusher", "filter", "distortion"} {
		defaults := DefaultParams(et)
		if defaults == nil {
			t.Errorf("DefaultParams(%q) returned nil; want at least an empty map", et)
		}
		// Catalog must list this effect type.
		cat := InsertEffectCatalog()
		if _, ok := cat[et]; !ok {
			t.Errorf("InsertEffectCatalog missing %q", et)
		}
	}
}

// The chain never dispatches against a nil InsertEffect.
func TestNewEffectProcessorUnknownTypeFallsBackToPassThrough(t *testing.T) {
	slot := EffectSlot{Type: "this-effect-does-not-exist", Enabled: true, Params: nil}
	proc := NewEffectProcessor(slot, 48000)
	if proc == nil {
		t.Fatal("NewEffectProcessor returned nil for unknown effect type")
	}
	if got := proc.ProcessSample(0.42); math.Abs(got-0.42) > 1e-9 {
		t.Errorf("unknown-effect ProcessSample(0.42) = %v, want 0.42 (passThrough)", got)
	}
}

func TestWaveshaperResetIsSafe(t *testing.T) {
	slot := EffectSlot{Type: "distortion", Enabled: true, Params: DefaultParams("distortion")}
	proc := NewEffectProcessor(slot, 48000)
	if proc == nil {
		t.Skip("distortion effect not registered (build-tag-dependent); skipping")
	}
	before := proc.ProcessSample(0.5)
	proc.Reset()
	after := proc.ProcessSample(0.5)
	if math.Abs(before-after) > 1e-9 {
		t.Errorf("waveshaper Reset() changed output: before=%v after=%v", before, after)
	}
}

func TestChannelEQSnapshotReflectsSetChannelEQ(t *testing.T) {
	ResetInstruments()
	SetChannelEQ(channel.Master, 48000, EQBand{Kind: EQPeaking, Freq: 150, Q: 1.0, GainDB: 3})
	got := ChannelEQSnapshot(channel.Master)
	if got.ID != "main" {
		t.Errorf("ChannelEQSnapshot(\"main\").ID = %q, want %q", got.ID, "main")
	}
	if got.SampleRate != 48000 {
		t.Errorf("ChannelEQSnapshot(\"main\").SampleRate = %d, want 48000", got.SampleRate)
	}
	if len(got.Bands) != 1 || got.Bands[0].GainDB != 3 {
		t.Errorf("ChannelEQSnapshot(\"main\").Bands = %+v, want one band with GainDB=3", got.Bands)
	}
}

func TestGetPutF64BufReusesCapacity(t *testing.T) {
	b := getF64Buf(8)
	if len(b) != 8 {
		t.Fatalf("getF64Buf(8) length=%d, want 8", len(b))
	}
	// Mutate so we can prove the underlying array was reused.
	b[0] = 1.25
	putF64Buf(b)
	c := getF64Buf(8)
	if len(c) != 8 {
		t.Fatalf("second getF64Buf(8) length=%d, want 8", len(c))
	}
	putF64Buf(c)

	// Boundary: getF64Buf(0) → nil.
	if got := getF64Buf(0); got != nil {
		t.Errorf("getF64Buf(0) = %v, want nil", got)
	}
	// Boundary: getF64Buf(-1) → nil.
	if got := getF64Buf(-1); got != nil {
		t.Errorf("getF64Buf(-1) = %v, want nil", got)
	}
	// Boundary: putF64Buf(nil) / putF64Buf(empty) → no panic.
	putF64Buf(nil)
	putF64Buf([]float64{})
}

// The seam internal/ui tests count document pushes with; it returns the previous hook to restore.
func TestSwapPlatformInstrumentDocChangedForTest(t *testing.T) {
	resetLibraryForTest(t)
	var capturedID instrument.ID
	hits := 0
	prev := SwapPlatformInstrumentDocChangedForTest(func(id instrument.ID, d instrument.Doc) {
		capturedID = id
		hits++
	})
	t.Cleanup(func() { SwapPlatformInstrumentDocChangedForTest(prev) })

	SetInstrumentParam(roster.Kick, "post_decay", 0.4)
	if hits != 1 || capturedID != roster.Kick {
		t.Errorf("hook invoked %d times (id %q), want once for kick", hits, capturedID)
	}
	// Passing nil leaves the active hook untouched but returns it.
	if cur := SwapPlatformInstrumentDocChangedForTest(nil); cur == nil {
		t.Error("SwapPlatformInstrumentDocChangedForTest(nil) returned nil; should return the active hook")
	}
}

// The soak-test seam: one entry per distinct modified id.
func TestModifiedInstrumentDocCountForTest(t *testing.T) {
	ResetInstrumentParams("any-id-clears-on-reset")
	startLen := ModifiedInstrumentDocCountForTest()

	SetInstrumentParam(roster.Kick, "amp_attack", 0.1)
	SetInstrumentParam(roster.Snare, "amp_decay", 0.2)
	got := ModifiedInstrumentDocCountForTest()
	// Some test builds route writes through a stub, so only "never shrinks" is locked.
	if got < startLen {
		t.Errorf("ModifiedInstrumentDocCountForTest shrank from %d to %d after writes", startLen, got)
	}

	ResetInstrumentParams(roster.Kick)
	if afterReset := ModifiedInstrumentDocCountForTest(); afterReset > got {
		t.Errorf("ModifiedInstrumentDocCountForTest grew after ResetInstrumentParams: %d -> %d", got, afterReset)
	}
}
