package songmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestNNLS_RecoversMixture(t *testing.T) {
	// Two orthogonal templates; b is 2*t0 + 3*t1 → recover ~[2,3].
	t0 := []float64{1, 0, 0}
	t1 := []float64{0, 1, 0}
	b := []float64{2, 3, 0}
	x := NNLS([][]float64{t0, t1}, b, 200)
	if math.Abs(x[0]-2) > 0.2 || math.Abs(x[1]-3) > 0.2 {
		t.Errorf("NNLS recovered %v want ~[2,3]", x)
	}
}

func TestNNLS_NonNegative(t *testing.T) {
	t0 := []float64{1, 1, 1}
	b := []float64{-5, -5, -5} // best non-negative fit is ~0
	x := NNLS([][]float64{t0}, b, 100)
	if x[0] < 0 {
		t.Errorf("NNLS returned negative coeff %v", x[0])
	}
}

func TestInstrumentActivity_TracksDisjointStems(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	cfg.HopSec = 0.1
	sr := 44100
	// Two stems with disjoint band energy: low (120 Hz) and high (4000 Hz).
	low := wave.Sine(120, 0.9, 1.0, sr).Samples
	high := wave.Sine(4000, 0.9, 1.0, sr).Samples
	stems := map[string][]float64{"bass": low, "lead": high}
	// Build the real timeline from a low+high mix so both templates are present.
	mix := make([]float64, len(low))
	for i := range mix {
		mix[i] = low[i] + high[i]
	}
	tl := fingerprint.SongTimelineOf(wave.Wave{Samples: mix, SampleRate: sr}, cfg)
	ids, act := InstrumentActivity(tl, stems, sr, cfg)
	if len(ids) != 2 || ids[0] != "bass" || ids[1] != "lead" {
		t.Fatalf("ids=%v want sorted [bass lead]", ids)
	}
	if len(act) == 0 {
		t.Fatal("no activity frames")
	}
	// Both instruments are present in the mix → both should show nonzero activation.
	sum := []float64{0, 0}
	for f := range act {
		for j := range act[f] {
			sum[j] += act[f][j]
		}
	}
	if sum[0] <= 0 || sum[1] <= 0 {
		t.Errorf("expected both stems active; got bass=%.3f lead=%.3f", sum[0], sum[1])
	}
}
