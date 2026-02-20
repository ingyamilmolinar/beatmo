//go:build test || js

package audio

import "math"

// filter wraps a biquad filter with user-facing parameters for LP/HP/BP modes.
// Mode 0=lowpass, 1=highpass, 2=bandpass.
//
// On cutoff/Q/mode changes, the filter crossfades between the old and new biquad
// outputs over ~5ms to avoid the transient spike from abrupt coefficient changes.
// The mix parameter is smoothed with a one-pole smoother.
type filter struct {
	mode   float64 // 0=LP, 1=HP, 2=BP
	cutoff float64 // Hz
	q      float64 // resonance
	mix    smoothParam

	sr int
	bq *biquad

	// Crossfade state for coefficient changes.
	bqOld    *biquad // previous biquad (nil when not crossfading)
	xfadePos int     // current position in crossfade
	xfadeLen int     // total crossfade in samples (~5ms)
}

func newFilter(sr int, params map[string]float64) *filter {
	f := &filter{sr: sr}
	f.mode = clampf(params["mode"], 0, 2)
	f.cutoff = clampf(params["cutoff"], 20, 20000)
	f.q = clampf(params["q"], 0.1, 10)
	f.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	f.xfadeLen = int(math.Round(float64(sr) * defaultSmoothTimeMs * 0.001))
	if f.xfadeLen < 1 {
		f.xfadeLen = 1
	}
	f.rebuildBiquad()
	return f
}

func (f *filter) eqKind() EQKind {
	switch int(f.mode + 0.5) { // round to nearest int
	case 1:
		return EQHighpass
	case 2:
		return EQBandpass
	default:
		return EQLowpass
	}
}

func (f *filter) rebuildBiquad() {
	f.bq = makeBiquad(f.eqKind(), f.sr, f.cutoff, f.q, 0)
}

// startCrossfade preserves the current biquad and creates a new one,
// then crossfades between them over xfadeLen samples.
func (f *filter) startCrossfade() {
	if f.bq != nil {
		// Clone current biquad with its state.
		old := *f.bq
		f.bqOld = &old
	}
	f.rebuildBiquad()
	f.xfadePos = 0
}

func (f *filter) ProcessSample(x float64) float64 {
	mix := f.mix.tick()

	if f.bq == nil {
		return x
	}

	var wet float64
	if f.bqOld != nil && f.xfadePos < f.xfadeLen {
		// Crossfade: blend old biquad output with new.
		t := float64(f.xfadePos) / float64(f.xfadeLen)
		oldWet := f.bqOld.ProcessSample(x)
		newWet := f.bq.ProcessSample(x)
		wet = oldWet*(1-t) + newWet*t
		f.xfadePos++
		if f.xfadePos >= f.xfadeLen {
			f.bqOld = nil // crossfade complete
		}
	} else {
		wet = f.bq.ProcessSample(x)
	}

	return x*(1-mix) + wet*mix
}

func (f *filter) Reset() {
	if f.bq != nil {
		f.bq.x1, f.bq.x2, f.bq.y1, f.bq.y2 = 0, 0, 0, 0
	}
	f.bqOld = nil
	f.xfadePos = 0
}

func (f *filter) SetParam(name string, value float64) {
	switch name {
	case "mode":
		f.mode = clampf(value, 0, 2)
		f.startCrossfade()
	case "cutoff":
		f.cutoff = clampf(value, 20, 20000)
		f.startCrossfade()
	case "q":
		f.q = clampf(value, 0.1, 10)
		f.startCrossfade()
	case "mix":
		f.mix.set(clampf(value, 0, 1))
	}
}
