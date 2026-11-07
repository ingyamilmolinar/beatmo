//go:build !test && !js

package audio

// A melodic voice honours a duration below 1 by sounding for less time, not by playing faster, which transposed it.
type gateVoice struct {
	inner Voice
	limit int
	fade  int
	pos   int
}

// Long enough not to click, short against the shortest templated note (a sixteenth at 200 BPM is 75 ms).
const gateFadeMs = GateFadeMs // chain_spec.go

func newGateVoice(inner Voice, limit, sampleRate int) *gateVoice {
	fade := sampleRate * gateFadeMs / 1000
	if fade > limit/2 {
		fade = limit / 2
	}
	if fade < 1 {
		fade = 1
	}
	return &gateVoice{inner: inner, limit: limit, fade: fade}
}

func (g *gateVoice) Sample() (float64, bool) {
	if g.pos >= g.limit {
		return 0, true
	}
	s, done := g.inner.Sample()
	if done {
		g.pos = g.limit
		return 0, true
	}
	if left := g.limit - g.pos; left <= g.fade {
		s *= float64(left) / float64(g.fade)
	}
	g.pos++
	return s, false
}
