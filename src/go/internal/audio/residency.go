package audio

import (
	"math"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A channel no row plays stops running only once nothing in it can sound: no voice, and input and output quiet for
// ReleaseHoldSec plus its chain's memory (a delay's echo). A voice or a re-hold wakes it fresh; settings are never touched.

const (
	ReleaseQuietDB = -90.0
	ReleaseHoldSec = 0.25
)

// releaseQuietAmp is ReleaseQuietDB as an amplitude.
var releaseQuietAmp = math.Pow(10, ReleaseQuietDB/20)

// Its output can be silent that long and still bring the sound back. An unlisted effect has no buffer.
var effectMemorySec = map[EffectType]float64{
	EffectDelay:      1.0,  // time ≤ 1000 ms
	EffectPitchShift: 0.15, // window buffer
	EffectReverb:     0.1,  // longest comb + both allpasses ≈ 54 ms
	EffectChorus:     0.04,
	EffectFlanger:    0.02,
	EffectTape:       0.01,
}

// The enabled effects in series.
func ChainMemorySec(slots []EffectSlot) float64 {
	sec := 0.0
	for _, s := range slots {
		if s.Enabled {
			sec += effectMemorySec[s.Type]
		}
	}
	return sec
}

// nil until a circuit declares one (offline tools, tests): then every channel is held.
var heldChannels atomic.Pointer[map[channel.ID]bool]

// SetHeldInstruments declares the instruments the rows play.
func SetHeldInstruments(ids []instrument.ID) {
	held := make(map[channel.ID]bool, len(ids))
	for _, id := range ids {
		held[channel.Of(id)] = true
	}
	heldChannels.Store(&held)
	platformHeldChanged(ids)
}

// Every channel is held again.
func ResetHeldInstruments() { heldChannels.Store(nil) }

// ChannelHeld reports whether a row plays the channel's instrument.
func ChannelHeld(id channel.ID) bool {
	p := heldChannels.Load()
	return p == nil || (*p)[id]
}

// For a platform that releases its own resources (the browser's WebAudio chains).
var platformHeldChanged = func([]instrument.ID) {}

// residencyState is what one mixer knows about one channel's residency.
type residencyState struct {
	quietFrames int  // frames its input and output have stayed below the threshold
	released    bool // its stages stopped running
}

func (r *residencyState) trackQuiet(in, out []float64) {
	for i := range in {
		if math.Abs(in[i]) > releaseQuietAmp || math.Abs(out[i]) > releaseQuietAmp {
			r.quietFrames = 0
			return
		}
	}
	r.quietFrames += len(in)
}
