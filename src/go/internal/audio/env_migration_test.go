package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The EQ-side entry point applies no volume, so env.BypassEQ's effect is observable directly.
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

// Read through internal/env, not an init-time os.Getenv cache, which made the flag untestable without a subprocess.
func TestBypassEQHonorsEnvVar(t *testing.T) {
	restore := env.BypassEQ.SetForTest(true)
	defer restore()
	if !env.BypassEQ.Get() {
		t.Fatalf("precondition: BypassEQ.Get()=false after SetForTest(true)")
	}
	// With EQ bypassed a non-flat curve leaves the signal unchanged.
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
