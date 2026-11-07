//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Returns the ones it refused because they had already started.
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

// Voices starting at/after t are spared. t goes to a sample position through playParamsIdentity's delay math.
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
