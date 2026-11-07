//go:build !test && !js

package audio

// Linear fade-in and fade-out against clicks from abrupt transitions; the browser bakes the same fade-in and ramps its
// gain over the same frames, so a stop sounds the same on both. Done only after the fade-out, so the mixer removes it.
type antiPopVoice struct {
	inner          Voice
	pos            int
	fadeInSamples  int
	fadeOutSamples int
	stopping       bool
	fadeOutPos     int
	stopAt         int // voice sample the fade-out starts at; -1 = none pending
	done           bool
}

// Fade durations (chain_spec.go); at 48 kHz 240 and 960 samples.
const (
	antiPopFadeInMs  = FadeInMs   // chain_spec.go: the browser bakes the same fade
	antiPopFadeOutMs = StopFadeMs // chain_spec.go
)

func newAntiPopVoice(inner Voice, sr int) *antiPopVoice {
	fadeIn := sr * antiPopFadeInMs / 1000
	fadeOut := sr * antiPopFadeOutMs / 1000
	if fadeIn < 1 {
		fadeIn = 1
	}
	if fadeOut < 1 {
		fadeOut = 1
	}
	return &antiPopVoice{
		inner:          inner,
		fadeInSamples:  fadeIn,
		fadeOutSamples: fadeOut,
		stopAt:         -1,
	}
}

// Done after fadeOutSamples.
func (a *antiPopVoice) RequestStop() {
	if !a.stopping {
		a.stopping = true
		a.fadeOutPos = 0
	}
}

// On the voice's own sample, so a lane stop lands exactly, not on the containing mixer block. A past pos stops now.
func (a *antiPopVoice) RequestStopAt(pos int) {
	if a.stopping {
		return
	}
	if pos <= a.pos {
		a.RequestStop()
		return
	}
	if a.stopAt < 0 || pos < a.stopAt {
		a.stopAt = pos
	}
}

// IsStopping returns true if a fade-out is in progress.
func (a *antiPopVoice) IsStopping() bool {
	return a.stopping
}

func (a *antiPopVoice) SampleBlock(dst []float64) (int, bool) {
	if a.done {
		return 0, true
	}

	// Try bulk path on inner voice.
	if bv, ok := a.inner.(BlockVoice); ok {
		n, innerDone := bv.SampleBlock(dst)
		// Apply envelope in-place.
		for i := 0; i < n; i++ {
			if a.stopAt >= 0 && a.pos >= a.stopAt {
				a.stopAt = -1
				a.RequestStop()
			}
			// Fade-in.
			if a.pos < a.fadeInSamples {
				dst[i] *= float64(a.pos) / float64(a.fadeInSamples)
			}
			// Fade-out.
			if a.stopping {
				if a.fadeOutPos >= a.fadeOutSamples {
					a.done = true
					return i, true
				}
				dst[i] *= 1.0 - float64(a.fadeOutPos)/float64(a.fadeOutSamples)
				a.fadeOutPos++
			}
			a.pos++
		}
		if innerDone {
			a.done = true
		}
		return n, a.done
	}

	// Never probe for the end with an extra Sample: that drops a sample; the next block reports done.
	for i := range dst {
		val, done := a.Sample()
		if done {
			return i, true
		}
		dst[i] = val
	}
	return len(dst), false
}

func (a *antiPopVoice) Sample() (float64, bool) {
	if a.done {
		return 0, true
	}

	val, innerDone := a.inner.Sample()

	// A micro-fade on the last sample in case the inner voice ends above zero.
	if innerDone {
		a.done = true
		return 0, true
	}

	if a.stopAt >= 0 && a.pos >= a.stopAt {
		a.stopAt = -1
		a.RequestStop()
	}
	// Fade-in envelope: linear ramp 0→1 over fadeInSamples.
	if a.pos < a.fadeInSamples {
		env := float64(a.pos) / float64(a.fadeInSamples)
		val *= env
	}
	a.pos++

	// The browser's gain ramp is the same line.
	if a.stopping {
		if a.fadeOutPos >= a.fadeOutSamples {
			a.done = true
			return 0, true
		}
		val *= 1.0 - float64(a.fadeOutPos)/float64(a.fadeOutSamples)
		a.fadeOutPos++
	}

	return val, false
}
