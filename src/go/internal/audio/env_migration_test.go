package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// processChannelForTest is a thin test-only wrapper around the channel's
// unexported EQ-side processing entry point (Channel.ProcessEQBlockLocal),
// which does not apply volume gain — only the eqProcs chain, gated on
// env.BypassEQ. This lets the test observe env.BypassEQ's effect directly
// without going through the full mixer (which also applies volume/headroom).
func processChannelForTest(id string, buf []float32) {
	ch := chanMgr.ensureChannel(channel.Of(instrument.ID(id)))
	in := make([]float64, len(buf))
	out := make([]float64, len(buf))
	for i, v := range buf {
		in[i] = float64(v)
	}
	ch.ProcessEQBlockLocal(in, out)
	for i := range buf {
		buf[i] = float32(out[i])
	}
}

// TestBypassEQHonorsEnvVar proves the bypass reads through internal/env
// rather than an init-time os.Getenv cache — the cache made the flag
// untestable without a subprocess.
func TestBypassEQHonorsEnvVar(t *testing.T) {
	restore := env.BypassEQ.SetForTest(true)
	defer restore()
	if !env.BypassEQ.Get() {
		t.Fatalf("precondition: BypassEQ.Get()=false after SetForTest(true)")
	}
	// processChannel must short-circuit. Assert via the observable effect:
	// with EQ bypassed a non-flat EQ curve leaves the signal unchanged.
	SetChannelEQ(channel.Master, 44100, EQBand{Kind: EQPeaking, Freq: 1000, Q: 1, GainDB: 12})
	in := []float32{0.5, -0.5, 0.25, -0.25}
	got := append([]float32(nil), in...)
	processChannelForTest("main", got)
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("sample %d = %v, want %v unchanged with BypassEQ set", i, got[i], in[i])
		}
	}
}
