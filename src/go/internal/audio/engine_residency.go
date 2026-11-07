//go:build !test && !js

package audio

import "math"

// residencyOf is this mixer's residency state for ch.
func (m *mixer) residencyOf(ch *Channel) *residencyState {
	r := m.resState[ch]
	if r == nil {
		if m.resState == nil {
			m.resState = map[*Channel]*residencyState{}
		}
		r = &residencyState{}
		m.resState[ch] = r
	}
	return r
}

// Quiet for its hold plus its chain's memory; its stages are reset, so a voice or a re-hold wakes a fresh channel.
func (m *mixer) releaseIfSilent(ch *Channel) {
	if ChannelHeld(ch.id) {
		return
	}
	r := m.residencyOf(ch)
	need := (ReleaseHoldSec + math.Float64frombits(ch.memory.Load())) * float64(sampleRate)
	if float64(r.quietFrames) < need {
		return
	}
	ch.resetProcessorState()
	r.released, r.quietFrames = true, 0
}
