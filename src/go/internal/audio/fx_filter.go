//go:build test || js

package audio

// filter wraps a biquad filter with user-facing parameters for LP/HP/BP modes.
// Mode 0=lowpass, 1=highpass, 2=bandpass.
type filter struct {
	mode   float64 // 0=LP, 1=HP, 2=BP
	cutoff float64 // Hz
	q      float64 // resonance
	mix    float64 // wet/dry

	sr int
	bq *biquad
}

func newFilter(sr int, params map[string]float64) *filter {
	f := &filter{sr: sr}
	f.mode = clampf(params["mode"], 0, 2)
	f.cutoff = clampf(params["cutoff"], 20, 20000)
	f.q = clampf(params["q"], 0.1, 10)
	f.mix = clampf(params["mix"], 0, 1)
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

func (f *filter) ProcessSample(x float64) float64 {
	if f.bq == nil {
		return x
	}
	wet := f.bq.ProcessSample(x)
	return x*(1-f.mix) + wet*f.mix
}

func (f *filter) Reset() {
	if f.bq != nil {
		f.bq.x1, f.bq.x2, f.bq.y1, f.bq.y2 = 0, 0, 0, 0
	}
}

func (f *filter) SetParam(name string, value float64) {
	switch name {
	case "mode":
		f.mode = clampf(value, 0, 2)
		f.rebuildBiquad()
	case "cutoff":
		f.cutoff = clampf(value, 20, 20000)
		f.rebuildBiquad()
	case "q":
		f.q = clampf(value, 0.1, 10)
		f.rebuildBiquad()
	case "mix":
		f.mix = clampf(value, 0, 1)
	}
}
