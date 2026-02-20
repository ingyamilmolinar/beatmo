//go:build !test && !js

package audio

// antiPopVoice wraps any Voice, applying a linear fade-in over the first
// fadeInSamples and a quadratic fade-out over fadeOutSamples when
// RequestStop() is called. This prevents audible clicks/pops from abrupt
// signal transitions (non-zero → zero).
//
// The wrapper is transparent: it delegates Sample() to the inner voice and
// multiplies by an envelope. The voice reports done=true only after the
// fade-out completes, so the mixer naturally removes it.
type antiPopVoice struct {
	inner          Voice
	pos            int
	fadeInSamples  int
	fadeOutSamples int
	stopping       bool
	fadeOutPos     int
	done           bool
}

// Default fade durations at 44100 Hz:
//
//	fadeIn:  5ms  ≈ 220 samples
//	fadeOut: 20ms ≈ 882 samples
const (
	antiPopFadeInMs  = 5
	antiPopFadeOutMs = 20
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
	}
}

// RequestStop begins the fade-out. The voice will report done after
// fadeOutSamples have been produced.
func (a *antiPopVoice) RequestStop() {
	if !a.stopping {
		a.stopping = true
		a.fadeOutPos = 0
	}
}

// IsStopping returns true if a fade-out is in progress.
func (a *antiPopVoice) IsStopping() bool {
	return a.stopping
}

// SampleBlock implements BlockVoice for antiPopVoice. If the inner voice
// supports BlockVoice, it uses the bulk path and applies envelope in-place.
func (a *antiPopVoice) SampleBlock(dst []float64) (int, bool) {
	if a.done {
		return 0, true
	}

	// Try bulk path on inner voice.
	if bv, ok := a.inner.(BlockVoice); ok {
		n, innerDone := bv.SampleBlock(dst)
		// Apply envelope in-place.
		for i := 0; i < n; i++ {
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
				t := 1.0 - float64(a.fadeOutPos)/float64(a.fadeOutSamples)
				dst[i] *= t * t
				a.fadeOutPos++
			}
			a.pos++
		}
		if innerDone {
			a.done = true
		}
		return n, a.done
	}

	// Fallback: sample-by-sample.
	for i := range dst {
		val, done := a.Sample()
		if done {
			return i, true
		}
		dst[i] = val
	}
	// Probe: voice may be exactly exhausted at block boundary.
	if _, done := a.Sample(); done {
		a.done = true
		return len(dst), true
	}
	return len(dst), false
}

func (a *antiPopVoice) Sample() (float64, bool) {
	if a.done {
		return 0, true
	}

	val, innerDone := a.inner.Sample()

	// If inner voice finished naturally, apply a quick micro-fade on the
	// last sample to avoid a pop (the inner voice already decayed, but
	// just in case).
	if innerDone {
		a.done = true
		return 0, true
	}

	// Fade-in envelope: linear ramp 0→1 over fadeInSamples.
	if a.pos < a.fadeInSamples {
		env := float64(a.pos) / float64(a.fadeInSamples)
		val *= env
	}
	a.pos++

	// Fade-out envelope: quadratic curve 1→0 for a smoother tail.
	if a.stopping {
		if a.fadeOutPos >= a.fadeOutSamples {
			a.done = true
			return 0, true
		}
		t := 1.0 - float64(a.fadeOutPos)/float64(a.fadeOutSamples)
		val *= t * t // quadratic fade
		a.fadeOutPos++
	}

	return val, false
}
