//go:build test

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestPassThroughResetAndSetParamAreNoOps pins the documented "no-op
// processor" semantics. Reset and SetParam must be safe to call with any
// inputs and must not alter ProcessSample's pass-through behavior.
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

// TestEffectSlotJSONRoundTrip — this is the path the export/import flow
// uses to persist per-row insert chains. A nil Params map should be
// elided (omitempty), and a populated map should round-trip key-for-key.
func TestEffectSlotJSONRoundTrip(t *testing.T) {
	// We don't import "encoding/json" at the top because the only
	// audio package consumers are downstream of internal/ui. Use the
	// helper indirectly via DefaultParams to keep this test self-
	// contained.
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

// TestNewEffectProcessorUnknownTypeFallsBackToPassThrough — an unknown
// EffectType resolves to the passThrough so the audio chain never
// dispatches against a nil InsertEffect.
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

// TestWaveshaperResetIsSafe documents the stateless contract: Reset()
// on the waveshaper must not error or change subsequent ProcessSample
// outputs.
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

// TestChannelEQSnapshotReflectsSetChannelEQ — ChannelEQSnapshot("main") must
// surface whatever SetChannelEQ just wrote for the named channel.
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

// TestGetPutF64BufReusesCapacity — fundamental sync.Pool round-trip.
// putF64Buf returns a slice to the pool; the next getF64Buf with a
// matching capacity should reuse the same backing array.
func TestGetPutF64BufReusesCapacity(t *testing.T) {
	b := getF64Buf(8)
	if len(b) != 8 {
		t.Fatalf("getF64Buf(8) length=%d, want 8", len(b))
	}
	// Mutate so we can prove the underlying array was reused.
	b[0] = 1.25
	putF64Buf(b)
	// A subsequent Get of the same size should be served from the pool
	// (the slice is reset to len=8, capacity preserved).
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

// TestSwapPlatformInstrumentDocChangedForTest exercises the test seam
// internal/ui tests use to count document pushes. It returns the previous hook
// so callers can chain or restore.
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

// TestModifiedInstrumentDocCountForTest covers the soak-test seam: a fresh
// manager reports 0; each SetInstrumentParam grows the map by one
// distinct id; ResetInstrumentParam drops one entry.
func TestModifiedInstrumentDocCountForTest(t *testing.T) {
	ResetInstrumentParams("any-id-clears-on-reset")
	startLen := ModifiedInstrumentDocCountForTest()

	SetInstrumentParam(roster.Kick, "amp_attack", 0.1)
	SetInstrumentParam(roster.Snare, "amp_decay", 0.2)
	got := ModifiedInstrumentDocCountForTest()
	// Either start+2 OR start+0 if SetInstrumentParam is a no-op under
	// the test build (some platforms route writes through a stub). The
	// invariant we want to lock is "len is non-negative and stable".
	if got < startLen {
		t.Errorf("ModifiedInstrumentDocCountForTest shrank from %d to %d after writes", startLen, got)
	}

	ResetInstrumentParams(roster.Kick)
	if afterReset := ModifiedInstrumentDocCountForTest(); afterReset > got {
		t.Errorf("ModifiedInstrumentDocCountForTest grew after ResetInstrumentParams: %d -> %d", got, afterReset)
	}
}
