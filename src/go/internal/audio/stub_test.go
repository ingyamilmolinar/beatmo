//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestStubPlayDoesNotPanic(t *testing.T) {
	// Play with no optional args.
	Play(roster.Kick)
	// Play with a 'when' argument.
	Play(roster.Snare, 0.5)

	// PlayVol with and without when.
	PlayVol(roster.Kick, 0.8)
	PlayVol(roster.Hihat, 0.5, 1.0)

	// PlayParams with and without when.
	PlayParams(roster.Tom, 0.9, 1.0, 0.5)
	PlayParams(roster.Clap, 0.7, 0.8, 0.3, 2.0)

	// PlayParamsAt (fixed arity, no varargs).
	PlayParamsAt(roster.Kick, 1.0, 0.0, 1.0, 0.0)
}

func TestStubStopHook(t *testing.T) {
	var called instrument.ID
	SetStopHook(func(id instrument.ID) { called = id })
	t.Cleanup(func() { SetStopHook(nil) })

	Stop(roster.Kick)
	if called != roster.Kick {
		t.Fatalf("expected stop hook called with %q, got %q", "kick", called)
	}
}

func TestStubNowOverride(t *testing.T) {
	// Default returns 0.
	if v := Now(); v != 0 {
		t.Fatalf("expected Now()=0, got %v", v)
	}

	restore := SetNowForTest(func() float64 { return 42.5 })
	if v := Now(); v != 42.5 {
		t.Fatalf("expected Now()=42.5, got %v", v)
	}

	restore()
	if v := Now(); v != 0 {
		t.Fatalf("expected Now()=0 after restore, got %v", v)
	}
}

func TestStubSampleRate(t *testing.T) {
	if sr := SampleRate(); sr != 44100 {
		t.Fatalf("expected SampleRate()=44100, got %d", sr)
	}
}

func TestStubRegisterAndInstruments(t *testing.T) {
	ResetInstruments()
	t.Cleanup(ResetInstruments)

	Register("custom", nil)

	found := false
	for _, id := range Instruments() {
		if id == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'custom' in Instruments(), got %v", Instruments())
	}
}

func TestStubChannelVolumeRoundTrip(t *testing.T) {
	ResetInstruments()
	t.Cleanup(ResetInstruments)

	InstrumentChannel(channel.Of(roster.Kick)) // ensure channel exists
	SetChannelVolume(channel.Of(roster.Kick), 0.7)
	got := ChannelVolume(channel.Of(roster.Kick))
	if got != 0.7 {
		t.Fatalf("expected ChannelVolume=0.7, got %v", got)
	}
}

func TestStubInsertEffectLifecycle(t *testing.T) {
	ResetInstruments()
	ClearAllInsertEffects()
	t.Cleanup(func() {
		ClearAllInsertEffects()
		ResetInstruments()
	})

	// Add first effect.
	idx := AddInsertEffect(roster.Kick, EffectDistortion, nil)
	if idx != 0 {
		t.Fatalf("expected first slot index=0, got %d", idx)
	}
	effects := GetInsertEffects(roster.Kick)
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(effects))
	}
	if effects[0].Type != EffectDistortion {
		t.Fatalf("expected distortion, got %v", effects[0].Type)
	}
	if !effects[0].Enabled {
		t.Fatal("expected effect to be enabled by default")
	}

	// Toggle off.
	ToggleInsertEffect(roster.Kick, 0, false)
	effects = GetInsertEffects(roster.Kick)
	if effects[0].Enabled {
		t.Fatal("expected effect to be disabled after toggle")
	}

	// Add second effect.
	idx2 := AddInsertEffect(roster.Kick, EffectDelay, nil)
	if idx2 != 1 {
		t.Fatalf("expected second slot index=1, got %d", idx2)
	}
	effects = GetInsertEffects(roster.Kick)
	if len(effects) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(effects))
	}

	// Move: swap order (move index 0 to index 1).
	MoveInsertEffect(roster.Kick, 0, 1)
	effects = GetInsertEffects(roster.Kick)
	if len(effects) != 2 {
		t.Fatalf("expected 2 effects after move, got %d", len(effects))
	}
	// After moving distortion from 0 to 1, delay should be at 0.
	if effects[0].Type != EffectDelay {
		t.Fatalf("expected delay at index 0 after move, got %v", effects[0].Type)
	}
	if effects[1].Type != EffectDistortion {
		t.Fatalf("expected distortion at index 1 after move, got %v", effects[1].Type)
	}

	// Remove first effect (now delay at index 0).
	RemoveInsertEffect(roster.Kick, 0)
	effects = GetInsertEffects(roster.Kick)
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect after remove, got %d", len(effects))
	}
	if effects[0].Type != EffectDistortion {
		t.Fatalf("expected remaining effect to be distortion, got %v", effects[0].Type)
	}
}

func TestNewEQProcessorAllMuted(t *testing.T) {
	p := NewEQProcessor(44100,
		EQBand{Muted: true, GainDB: 0},
		EQBand{Muted: true, GainDB: 0},
	)
	// All bands muted → silenceProcessor.
	if _, ok := p.(*silenceProcessor); !ok {
		t.Fatalf("expected silenceProcessor when all bands muted, got %T", p)
	}
	// Output should be zero.
	out := p.ProcessSample(1.0)
	if out != 0 {
		t.Fatalf("expected 0 from silenceProcessor, got %f", out)
	}
}

func TestNewEQProcessorSomeMuted(t *testing.T) {
	p := NewEQProcessor(44100,
		EQBand{Muted: false, GainDB: 0},
		EQBand{Muted: true, GainDB: 0},
	)
	// One band muted → multibandProcessor.
	if _, ok := p.(*multibandProcessor); !ok {
		t.Fatalf("expected multibandProcessor when some bands muted, got %T", p)
	}
}

func TestNewEQProcessorGainOnly(t *testing.T) {
	p := NewEQProcessor(44100,
		EQBand{Muted: false, GainDB: 0},
		EQBand{Muted: false, GainDB: 6},
	)
	// No muting → gainProcessor.
	gp, ok := p.(*gainProcessor)
	if !ok {
		t.Fatalf("expected gainProcessor when no bands muted, got %T", p)
	}
	// 0dB * 6dB = product of linear gains
	if gp.gain < 1.0 {
		t.Fatalf("expected gain > 1.0 for +6dB band, got %f", gp.gain)
	}
}

func TestNewEQProcessorGainFloor(t *testing.T) {
	p := NewEQProcessor(44100,
		EQBand{Muted: false, GainDB: -200},
	)
	gp, ok := p.(*gainProcessor)
	if !ok {
		t.Fatalf("expected gainProcessor, got %T", p)
	}
	if gp.gain < 0.0001 {
		t.Fatalf("expected gain floor of 0.0001, got %f", gp.gain)
	}
}

func TestMultibandProcessBlockBuf(t *testing.T) {
	p := NewEQProcessor(44100,
		EQBand{Muted: false, GainDB: 0},
		EQBand{Muted: true, GainDB: 0},
	)
	mb, ok := p.(*multibandProcessor)
	if !ok {
		t.Fatalf("expected multibandProcessor, got %T", p)
	}
	in := make([]float32, 256)
	out := make([]float32, 256)
	for i := range in {
		in[i] = 0.5
	}
	mb.ProcessBlockBuf(in, out, 256)
	// At least some output should be non-zero (unmuted band passes signal).
	hasNonZero := false
	for _, v := range out {
		if v != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("expected non-zero output from multibandProcessor with one unmuted band")
	}
}

func TestNearestPow2(t *testing.T) {
	cases := []struct{ in, want int }{
		{63, 64},
		{64, 64},
		{65, 64},
		{128, 128},
		{1, 1},
		{1000, 1024},
	}
	for _, tc := range cases {
		got := nearestPow2(tc.in)
		if got != tc.want {
			t.Errorf("nearestPow2(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestAnalyzerProcessBlockEmpty(t *testing.T) {
	a := NewAnalyzer(64)
	// n=0 should be a no-op.
	a.ProcessBlock(nil, 0)
	snap := a.Snapshot()
	if snap.RMS != 0 {
		t.Fatalf("expected RMS=0 after empty ProcessBlock, got %f", snap.RMS)
	}
}

func TestAnalyzerDisabledCompute(t *testing.T) {
	a := NewAnalyzer(64)
	a.SetEnabled(false)
	// Feed enough samples to fill the window.
	buf := make([]float32, 64)
	for i := range buf {
		buf[i] = 1.0
	}
	a.ProcessBlock(buf, 64)
	snap := a.Snapshot()
	if snap.RMS != 0 {
		t.Fatalf("expected RMS=0 when analyzer disabled, got %f", snap.RMS)
	}
	if snap.Peak != 0 {
		t.Fatalf("expected Peak=0 when analyzer disabled, got %f", snap.Peak)
	}
}

func TestSilenceProcessorBlockBuf(t *testing.T) {
	s := &silenceProcessor{}
	in := make([]float32, 64)
	out := make([]float32, 64)
	for i := range in {
		in[i] = 0.5
	}
	for i := range out {
		out[i] = 99.0 // fill with non-zero to prove it gets zeroed
	}
	s.ProcessBlockBuf(in, out, 64)
	for i, v := range out {
		if v != 0 {
			t.Fatalf("expected out[%d]=0, got %f", i, v)
		}
	}
}

func TestStubSetBPMFunc(t *testing.T) {
	var recorded int
	origFunc := BPMFuncForTest()
	SetBPMFuncForTest(func(bpm int) { recorded = bpm })
	t.Cleanup(func() { SetBPMFuncForTest(origFunc) })

	SetBPM(140)
	if recorded != 140 {
		t.Fatalf("expected SetBPMFunc called with 140, got %d", recorded)
	}
}
