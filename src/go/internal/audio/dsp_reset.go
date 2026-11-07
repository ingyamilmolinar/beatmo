package audio

// Every stage between voice and speaker carries state (biquad history, delay lines, LFO phase), right during playback
// and wrong across a DISCONTINUITY: a load or a second offline render must start from silence, or the same circuit
// sounds different depending on what played before. ResetDSPState is the one place that clears it; never mid-stream.

// A Processor that does not implement it is stateless by construction and needs no reset.
type Resettable interface {
	Reset()
}

// Clears what the filters REMEMBER, never what they are set to. It MUTATES in place, so only at a quiescent point
// (playback stopped, or an offline mixer no audio thread reads); a live chain changes click-free through
// replaceProcessors instead.
func ResetDSPState() {
	for _, ch := range chanMgr.allChannels() {
		ch.resetProcessorState()
	}
	platformResetDSPState()
}

// The browser's C effects and EQ in the worklet; desktop's C state belongs to the Go processors.
var platformResetDSPState = func() {}

// allChannels returns every channel the manager holds, master included.
func (cm *channelManager) allChannels() []*Channel {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	out := make([]*Channel, 0, len(cm.channels))
	for _, ch := range cm.channels {
		out = append(out, ch)
	}
	return out
}

// The crossfade hides a click on an effect toggle; at a discontinuity there is none, and the retired chain's state
// would leak into the new stream.
func (c *Channel) resetProcessorState() {
	c.mu.Lock()
	procs := append([]Processor(nil), c.processors...)
	old := c.oldProcessors
	c.oldProcessors = nil
	c.xfadePos = 0
	c.xfadeLen = 0
	c.mu.Unlock()

	for _, p := range procs {
		if r, ok := p.(Resettable); ok {
			r.Reset()
		}
	}
	// replaceProcessors may hand the same instances back to a later chain.
	for _, p := range old {
		if r, ok := p.(Resettable); ok {
			r.Reset()
		}
	}
}
