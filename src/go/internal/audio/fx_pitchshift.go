//go:build test || js

package audio

import "math"

// pitchShift implements a dual delay-line pitch shifter (Eventide-style).
// Two read heads traverse a circular buffer at a rate that differs from the
// write rate. The rate difference creates pitch shift. Triangular cross-fading
// between the two heads (offset by half a window) hides wrap-around
// discontinuities. All parameters are smoothed to prevent clicks on change.
type pitchShift struct {
	pitch  smoothParam // semitones -24..+24
	mix    smoothParam // 0=dry, 1=wet
	window smoothParam // window size in ms (20-100)

	sr            int
	buf           []float64
	writePos      int
	headA         float64
	headB         float64
	windowSamples float64
}

func newPitchShift(sr int, params map[string]float64) *pitchShift {
	p := &pitchShift{sr: sr}
	p.pitch = newSmoothParam(clampf(params["pitch"], -24, 24), sr, defaultSmoothTimeMs)
	p.mix = newSmoothParam(clampf(params["mix"], 0, 1), sr, defaultSmoothTimeMs)
	win := params["window"]
	if win == 0 {
		win = 50
	}
	p.window = newSmoothParam(clampf(win, 20, 100), sr, defaultSmoothTimeMs)

	// Buffer: 120ms of headroom to cover max 100ms window.
	bufLen := int(0.12 * float64(sr))
	if bufLen < 64 {
		bufLen = 64
	}
	p.buf = make([]float64, bufLen)

	p.recalc()
	// headA starts at 0, headB at half-window offset.
	p.headA = 0
	p.headB = p.windowSamples / 2
	return p
}

func (p *pitchShift) recalc() {
	sr := float64(p.sr)
	if sr <= 0 {
		sr = 44100
	}
	p.windowSamples = p.window.value() * 0.001 * sr
	if p.windowSamples < 1 {
		p.windowSamples = 1
	}
	// Ensure buffer is large enough.
	needed := int(p.windowSamples*2) + 4
	if needed > len(p.buf) {
		newBuf := make([]float64, needed)
		copy(newBuf, p.buf)
		p.buf = newBuf
	}
}

// bufReadLerp reads from a circular buffer at a fractional position using
// linear interpolation.
func bufReadLerp(buf []float64, bufLen int, pos float64) float64 {
	for pos < 0 {
		pos += float64(bufLen)
	}
	idx := int(pos) % bufLen
	frac := pos - math.Floor(pos)
	next := (idx + 1) % bufLen
	return buf[idx]*(1-frac) + buf[next]*frac
}

func (p *pitchShift) ProcessSample(x float64) float64 {
	bufLen := len(p.buf)
	if bufLen == 0 {
		return x
	}

	pitch := p.pitch.tick()
	mix := p.mix.tick()
	winMs := p.window.tick()

	// Recompute window samples each tick (cheap, keeps it smooth).
	sr := float64(p.sr)
	if sr <= 0 {
		sr = 44100
	}
	p.windowSamples = winMs * 0.001 * sr
	if p.windowSamples < 1 {
		p.windowSamples = 1
	}

	// Write input to circular buffer.
	p.buf[p.writePos] = x
	p.writePos = (p.writePos + 1) % bufLen

	// Rate difference: shift = 2^(semitones/12).
	// step = 1.0 - shift: how fast read heads drift relative to write head.
	shift := math.Pow(2.0, pitch/12.0)
	step := 1.0 - shift

	// Update head positions (fractional, within window range).
	p.headA += step
	p.headB += step

	ws := p.windowSamples

	// Wrap heads within [0, windowSamples).
	p.headA = math.Mod(p.headA, ws)
	if p.headA < 0 {
		p.headA += ws
	}
	p.headB = math.Mod(p.headB, ws)
	if p.headB < 0 {
		p.headB += ws
	}

	// Triangular cross-fade: peaks at center of window, zero at edges.
	alphaA := 1.0 - math.Abs(2.0*p.headA/ws-1.0)
	alphaB := 1.0 - math.Abs(2.0*p.headB/ws-1.0)

	// Read from circular buffer using linear interpolation.
	readA := bufReadLerp(p.buf, bufLen, float64(p.writePos)-p.headA)
	readB := bufReadLerp(p.buf, bufLen, float64(p.writePos)-p.headB)

	// Normalize cross-fade weights.
	sumAlpha := alphaA + alphaB
	var wet float64
	if sumAlpha > 0 {
		wet = (alphaA*readA + alphaB*readB) / sumAlpha
	}

	return x*(1-mix) + wet*mix
}

func (p *pitchShift) Reset() {
	for i := range p.buf {
		p.buf[i] = 0
	}
	p.writePos = 0
	p.headA = 0
	p.headB = p.windowSamples / 2
}

func (p *pitchShift) SetParam(name string, value float64) {
	switch name {
	case "pitch":
		p.pitch.set(clampf(value, -24, 24))
	case "mix":
		p.mix.set(clampf(value, 0, 1))
	case "window":
		p.window.set(clampf(value, 20, 100))
		p.recalc()
	}
}
