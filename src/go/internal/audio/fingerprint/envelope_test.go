package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestLogAttackTime_LongerAttackLargerLAT(t *testing.T) {
	mk := func(attackSec float64) wave.Wave {
		sr := 44100
		n := int(1.0 * float64(sr))
		s := make([]float64, n)
		aN := int(attackSec * float64(sr))
		for i := range s {
			env := 1.0
			if i < aN {
				env = float64(i) / float64(aN)
			}
			s[i] = env * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	var fast, slow Fingerprint
	computeEnvelope(mk(0.005), &fast)
	computeEnvelope(mk(0.2), &slow)
	if !(slow.AttackTimeSec > fast.AttackTimeSec && slow.LogAttackTime > fast.LogAttackTime) {
		t.Errorf("attack fast=%.4f slow=%.4f; LAT fast=%.3f slow=%.3f", fast.AttackTimeSec, slow.AttackTimeSec, fast.LogAttackTime, slow.LogAttackTime)
	}
}

func TestSpectralConvergence_IdentityZero(t *testing.T) {
	a := []float64{1, 2, 3, 4}
	if d := SpectralConvergence(a, a); d > 1e-9 {
		t.Errorf("identity convergence=%.6f want 0", d)
	}
}

// TestLogSpectralDistance_IdentityZeroWithZeros pins the keystone identity
// invariant Distance(fp,fp).Total==0 relies on: LSD(x,x) must be exactly 0 even
// when x contains zero-valued bins (common at the low/high spectrum ends). The
// eps floor is applied symmetrically to both arguments, so the ratio is 1.
func TestLogSpectralDistance_IdentityZeroWithZeros(t *testing.T) {
	x := []float64{0, 0, 1.5, 0, 42, 0, 0.001}
	if d := LogSpectralDistance(x, x); d != 0 {
		t.Errorf("LSD(x,x)=%.9f want exactly 0 (zero bins must not break identity)", d)
	}
}
