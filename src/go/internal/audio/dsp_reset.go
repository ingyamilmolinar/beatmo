package audio

// DSP state reset.
//
// Every stage between a voice and the speaker carries state: EQ and crossover
// biquads hold two samples of history, insert effects hold delay lines and LFO
// phase. That state is correct
// and wanted DURING playback — it is what makes a filter a filter and a tremolo
// a tremolo.
//
// It is wrong across a DISCONTINUITY. Loading a project, or rendering the same
// circuit offline a second time, must start from silence: otherwise the same
// circuit with the same instruments produces different audio depending on what
// was playing beforehand. The tremolo is the clearest case — its LFO phase
// decides the gain of the very first sample, so a replay that inherits a
// mid-cycle phase modulates every sample differently.
//
// ResetDSPState is the single place that clears all of it. Call it at a
// discontinuity, never mid-stream.

// Resettable is implemented by any Processor that carries state which must be
// cleared at a playback discontinuity. Insert effects (fx_*.go, insert_fx_c.go)
// already implement it; biquad and the C EQ (cEQ) implement it for this
// purpose.
//
// A Processor that does NOT implement it is stateless by construction
// (passThrough, silenceProcessor, gainProcessor) and needs no reset.
type Resettable interface {
	Reset()
}

// ResetDSPState clears every stateful DSP node in the mix graph: each channel's
// insert-effect chain and EQ (including the master channel's EQ and any
// crossfade in progress).
//
// Configuration is untouched — cutoffs, gains, effect params and volumes all
// survive. This resets WHAT THE FILTERS REMEMBER, not what they are set to.
//
// Callers: project import (a document swap is a discontinuity — the new circuit
// must not inherit the previous one's delay tail or LFO phase) and every
// offline render pass (so a render is a pure function of its inputs).
//
// CONCURRENCY. This MUTATES processors in place rather than replacing them, so
// it must only be called at a quiescent point — with playback stopped (Import
// stops the sequencer for the duration) or on an offline mixer no audio thread
// is reading. It is not safe to call mid-stream; the click-free way to change a
// live chain is replaceProcessors, which swaps in fresh instances and
// crossfades. Mutating here is the right trade at a discontinuity: there is no
// click to hide, and allocating a whole new chain to zero four floats per
// biquad would be waste.
func ResetDSPState() {
	for _, ch := range chanMgr.allChannels() {
		ch.resetProcessorState()
	}
	platformResetDSPState()
}

// platformResetDSPState clears the DSP state a platform keeps outside Go: in
// the browser, every C effect and EQ in the beatmo-dsp worklet (JS
// resetDSPState). Desktop's C state belongs to the Go processors reset above.
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

// resetProcessorState clears this channel's processors and abandons any
// in-progress chain crossfade. The crossfade blends a retired chain into the
// live one to hide a click on an effect toggle; at a discontinuity there is no
// click to hide and the retired chain's state would leak into the new stream.
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
	// The retired chain is dropped above, but reset it too: replaceProcessors
	// may hand the same processor instances back to a later chain.
	for _, p := range old {
		if r, ok := p.(Resettable); ok {
			r.Reset()
		}
	}
}
