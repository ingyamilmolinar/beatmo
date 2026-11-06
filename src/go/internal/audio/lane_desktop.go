//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// CancelVoices silently removes lane voices that have not started rendering.
// CancelVoices silently removes not-yet-started lane voices and returns the
// ones it refused because they had already started.
func CancelVoices(ids []uint64) (started []uint64) {
	if mix != nil {
		return mix.cancelVoices(ids)
	}
	return nil
}

// CancelAllLaneFuture removes every lane voice that has not started.
func CancelAllLaneFuture() {
	if mix != nil {
		mix.cancelAllLaneFuture()
	}
}

// StopLaneAt fades, at audio time t (the Now() clock), the lane voices of
// inst that started before t; voices starting at/after t are spared. t is
// converted to a mixer sample position with the same delay math
// playParamsIdentity uses for a note's start.
func StopLaneAt(inst instrument.ID, t float64) {
	if mix == nil {
		return
	}
	d := t - Now()
	if d < 0 {
		d = 0
	}
	mix.mu.Lock()
	at := mix.pos + int(d*float64(sampleRate))
	mix.mu.Unlock()
	mix.stopLaneAtSample(channel.Of(inst), at)
}
