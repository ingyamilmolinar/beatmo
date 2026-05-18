package scope

import (
	"math"
	"testing"
	"time"
)

// Regression guard for the Chn-tab two-trace scenario the user reported:
// with the service scoped to a per-instrument id, pushing per-instrument
// samples for two different stages must yield both TapA and TapB active
// in the same published state. Existing tests cover either (single tap
// + instrument filter) or (two taps + no instrument); none combines both.

func TestServicePerInstrumentBothTapsActive(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})

	svc.SetTapA(StageSynth)
	svc.SetTapB(StageInsertFX)
	svc.SetInstrument("kick")

	go svc.Run()
	defer svc.Stop()

	synthBuf := make([]float64, 256)
	fxBuf := make([]float64, 256)
	for i := range synthBuf {
		synthBuf[i] = math.Sin(2 * math.Pi * float64(i) / 64.0)
		fxBuf[i] = 0.5 * math.Sin(2*math.Pi*float64(i)/32.0)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.PushSamples(StageSynth, "kick", synthBuf)
		svc.PushSamples(StageInsertFX, "kick", fxBuf)
		st := svc.State()
		if st != nil && st.TapA.Active && st.TapB.Active {
			if st.TapA.Stage != StageSynth || st.TapB.Stage != StageInsertFX {
				t.Fatalf("stages: A=%v B=%v; want Synth + InsertFX", st.TapA.Stage, st.TapB.Stage)
			}
			if st.TapA.InstID != "kick" || st.TapB.InstID != "kick" {
				t.Fatalf("instIDs: A=%q B=%q; want both \"kick\"", st.TapA.InstID, st.TapB.InstID)
			}
			// Sanity-check the trace contents differ between A and B so we
			// know the per-stage signals are being kept distinct rather than
			// blurred together.
			if len(st.TapA.Samples) == 0 || len(st.TapB.Samples) == 0 {
				t.Fatalf("empty samples: |A|=%d |B|=%d", len(st.TapA.Samples), len(st.TapB.Samples))
			}
			if approxEqual(st.TapA.Samples, st.TapB.Samples) {
				t.Fatal("TapA and TapB samples are identical — stages were not kept separate")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for both per-instrument taps to go active")
}

// TestServicePerInstrumentDropsMismatchedAndKeepsMaster pins the
// isMasterStage carve-out: when scoped to "kick", per-instrument
// samples for "snare" must be dropped, while master-tagged Master
// samples still pass.
func TestServicePerInstrumentDropsMismatchedAndKeepsMaster(t *testing.T) {
	svc := NewService(Config{MaxWindowMs: 100, SampleRate: 44100})

	svc.SetTapA(StageSynth)
	svc.SetTapB(StageMaster)
	svc.SetInstrument("kick")

	go svc.Run()
	defer svc.Stop()

	buf := make([]float64, 256)
	for i := range buf {
		buf[i] = 0.5
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.PushSamples(StageSynth, "snare", buf) // mismatched per-instrument
		svc.PushSamples(StageMaster, "master", buf)
		st := svc.State()
		if st != nil && st.TapB.Active && st.TapB.Stage == StageMaster {
			if st.TapA.Active {
				t.Fatalf("TapA should NOT be active for mismatched per-instrument samples; got %+v", st.TapA)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for master-bypass tap to go active")
}

// approxEqual returns true when two equal-length slices match within a
// small epsilon. Tests compare per-stage trace contents to ensure the
// service keeps distinct per-stage signals separate.
func approxEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	const eps = 1e-9
	for i := range a {
		if math.Abs(a[i]-b[i]) > eps {
			return false
		}
	}
	return true
}
