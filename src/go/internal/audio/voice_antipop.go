//go:build !test && !js

package audio

// antiPopVoice wraps any Voice, applying a linear fade-in over the first
// fadeInSamples and a linear fade-out over fadeOutSamples once stopped
// (RequestStop now, or RequestStopAt a sample of the voice). This prevents
// audible clicks/pops from abrupt signal transitions (non-zero → zero). The
// browser bakes the same fade-in into the buffer it plays and ramps its gain
// to zero over the same frames (audio.js), so a stop sounds the same on both.
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

// RequestStop begins the fade-out. The voice will report done after
// fadeOutSamples have been produced.
func (a *antiPopVoice) RequestStop() {
	if !a.stopping {
		a.stopping = true
		a.fadeOutPos = 0
	}
}

// RequestStopAt begins the fade-out when the voice reaches its own sample pos
// (counted from its first sample) — a lane stop lands on its exact sample, not
// on the mixer block that contains it. A pos already reached stops now.
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

	// Fallback: sample-by-sample. Never "probe" for the end with an extra
	// Sample: that consumes (drops) a sample; the next block reports done.
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

	// If inner voice finished naturally, apply a quick micro-fade on the
	// last sample to avoid a pop (the inner voice already decayed, but
	// just in case).
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

	// Fade-out envelope: linear 1→0 over fadeOutSamples (the browser's gain
	// ramp is the same line).
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
