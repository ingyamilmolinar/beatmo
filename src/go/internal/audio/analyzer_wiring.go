//go:build !js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/channel"

// Not inlined into EnableChannelAnalyzer: every analyzer in eqProcs runs an FFT on the AUDIO THREAD and focus changes
// re-enable on every call, so appending stacks FFT load. Reuse is judged from the CHANNEL CHAIN, never analyzerRegistry,
// which RebuildChannelWithEQ leaves dangling.

// Lets callers compare "already wired at this size" without constructing an Analyzer.
func effectiveAnalyzerWindow(window int) int {
	if window < 64 {
		window = 64
	}
	if window > 8192 {
		window = 8192
	}
	return nearestPow2(window)
}

// lookupChannel returns the channel for id without creating one.
func lookupChannel(id channel.ID) *Channel {
	chanMgr.mu.RLock()
	defer chanMgr.mu.RUnlock()
	if ch := chanMgr.channels[id]; ch != nil {
		return ch
	}
	return chanMgr.instruments[id]
}

func (c *Channel) eqAnalyzer() *Analyzer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.eqProcs {
		if an, ok := p.(*Analyzer); ok {
			return an
		}
	}
	return nil
}

// At most one analyzer tap; mirrors addEQProcessor's bookkeeping (processor list + block-capability cache).
func (c *Channel) replaceEQAnalyzer(an *Analyzer) {
	c.mu.Lock()
	kept := c.eqProcs[:0:0]
	for _, p := range c.eqProcs {
		if _, isAnalyzer := p.(*Analyzer); isAnalyzer {
			continue
		}
		kept = append(kept, p)
	}
	c.eqProcs = append(kept, an)
	c.processors = append(append([]Processor(nil), c.inserts...), c.eqProcs...)
	c.cacheBlockCapability()
	c.mu.Unlock()
}

// Always enabled, as the old append-always code's fresh analyzer was: applyMasterEQ/applyRowEQ re-enable only to keep
// panels live after a chain rebuild, so reusing a tap focus had disabled would leave them dead.
func wireChannelAnalyzer(id channel.ID, window int) *Analyzer {
	ch := chanMgr.ensureChannel(id)
	if cur := ch.eqAnalyzer(); cur != nil && len(cur.window) == effectiveAnalyzerWindow(window) {
		// Reused rather than stacking another FFT onto the audio thread.
		cur.SetEnabled(true)
		return cur
	}
	an := NewAnalyzer(window)
	ch.replaceEQAnalyzer(an)
	return an
}

// Each tap costs an FFT per window on the audio thread. 0 for an unknown channel, never creating one.
func ChannelAnalyzerCount(id channel.ID) int {
	ch := lookupChannel(id)
	if ch == nil {
		return 0
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	n := 0
	for _, p := range ch.processors {
		if _, ok := p.(*Analyzer); ok {
			n++
		}
	}
	return n
}
