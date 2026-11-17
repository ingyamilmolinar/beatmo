package audio

import (
	"math"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// residency.go — which channels keep running (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §1.2). The UI declares the instruments its rows play (the held set). A
// channel outside it stops running — its stages are reset and no longer
// processed every block — only once nothing in it can still sound: no voice,
// and input and output below ReleaseQuietDB for ReleaseHoldSec plus the
// chain's memory (the longest a stage can hold a sound before it reappears at
// the output, a delay's echo). A voice or a re-hold wakes it, as fresh as a
// channel that never ran. Its settings are never touched.

const (
	ReleaseQuietDB = -90.0
	ReleaseHoldSec = 0.25
)

// releaseQuietAmp is ReleaseQuietDB as an amplitude.
var releaseQuietAmp = math.Pow(10, ReleaseQuietDB/20)

// effectMemorySec is the longest a stage holds a sound inside it (fx_host.c's
// buffers): its output can be silent that long and still bring the sound back.
// An effect not listed has no buffer: whatever it holds shows at its output.
var effectMemorySec = map[EffectType]float64{
	EffectDelay:      1.0,  // time ≤ 1000 ms
	EffectPitchShift: 0.15, // window buffer
	EffectReverb:     0.1,  // longest comb + both allpasses ≈ 54 ms
	EffectChorus:     0.04,
	EffectFlanger:    0.02,
	EffectTape:       0.01,
}

// ChainMemorySec is how long the enabled effects of a chain, in series, can
// hold a sound with a silent output.
func ChainMemorySec(slots []EffectSlot) float64 {
	sec := 0.0
	for _, s := range slots {
		if s.Enabled {
			sec += effectMemorySec[s.Type]
		}
	}
	return sec
}

// heldChannels is the held set; nil until a circuit declares one (offline
// tools, tests): then every channel is held.
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

// ResetHeldInstruments returns to "no circuit declared a held set": every
// channel is held.
func ResetHeldInstruments() { heldChannels.Store(nil) }

// ChannelHeld reports whether a row plays the channel's instrument.
func ChannelHeld(id channel.ID) bool {
	p := heldChannels.Load()
	return p == nil || (*p)[id]
}

// platformHeldChanged hands the held set to a platform that releases its own
// resources (the browser's WebAudio chains).
var platformHeldChanged = func([]instrument.ID) {}

// residencyState is what one mixer knows about one channel's residency.
type residencyState struct {
	quietFrames int  // frames its input and output have stayed below the threshold
	released    bool // its stages stopped running
}

// trackQuiet counts the frames a channel's input and output have stayed below
// the release threshold.
func (r *residencyState) trackQuiet(in, out []float64) {
	for i := range in {
		if math.Abs(in[i]) > releaseQuietAmp || math.Abs(out[i]) > releaseQuietAmp {
			r.quietFrames = 0
			return
		}
	}
	r.quietFrames += len(in)
}
