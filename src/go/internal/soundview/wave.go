package soundview

import "math"

// From a rising zero crossing, whole periods of the pitch at T0, or a fixed 20 ms when unpitched (PeriodSec 0).
// Samples is a read-only view of the analysed PCM.
type WaveWindow struct {
	Samples   []float32
	T0        float64
	PeriodSec float64
}

const waveFixedSec = 0.02

// 25 % into a sustained note, or just after a percussive note's loudest moment (a quarter into its tail no longer shows
// its shape).
func (a *Analysis) WaveMoment() float64 {
	aud := a.AudibleSec()
	if aud <= 0 {
		return 0
	}
	t := 0.25 * aud
	env := a.Envelope()
	if a.AttackSec() < 0.03 && env.RMSAt(t) < env.MaxRMS()-12 {
		best := 0
		for i, v := range env.RMSdB {
			if v > env.RMSdB[best] {
				best = i
			}
		}
		t = (float64(best)+0.5)*env.HopSec + 0.01
	}
	return t
}

// Wave returns `cycles` periods (or 20 ms when unpitched) at WaveMoment.
func (a *Analysis) Wave(cycles int) WaveWindow {
	return a.WaveAt(a.WaveMoment(), cycles)
}

// WaveAt is Wave at an explicit moment t (ghost renders share the current
// note's moment, so the close-ups compare like with like).
func (a *Analysis) WaveAt(t float64, cycles int) WaveWindow {
	if len(a.pcm) == 0 || cycles <= 0 {
		return WaveWindow{}
	}
	i0 := int(t * float64(a.sr))
	i0 = max(1, min(len(a.pcm)-1, i0))
	period := 0.0
	if hz := a.Pitch().HzAt(t); hz > 0 {
		period = 1 / hz
	}
	span := waveFixedSec
	if period > 0 {
		span = float64(cycles) * period
		// Align to the first rising zero crossing within one period.
		lim := min(len(a.pcm)-1, i0+int(period*float64(a.sr))+1)
		for i := i0; i < lim; i++ {
			if a.pcm[i-1] < 0 && a.pcm[i] >= 0 {
				i0 = i
				break
			}
		}
	}
	n := int(math.Round(span * float64(a.sr)))
	end := min(len(a.pcm), i0+n)
	return WaveWindow{Samples: a.pcm[i0:end], T0: float64(i0) / float64(a.sr), PeriodSec: period}
}
