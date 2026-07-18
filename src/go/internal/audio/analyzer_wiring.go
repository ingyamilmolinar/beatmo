//go:build !js

package audio

// analyzer_wiring.go — the single place that wires a channel-analyzer tap into
// a channel's processor chain, plus observability for how many are wired.
//
// WHY THIS EXISTS (do not inline it back into EnableChannelAnalyzer):
// every *Analyzer sitting in a channel's eqProcs runs a full FFT on the AUDIO
// RENDERING THREAD each time its window fills, and it is created enabled. The
// original EnableChannelAnalyzer did NewAnalyzer + AddChannelProcessor, and
// AddChannelProcessor appends with no dedupe — so each call left another live
// FFT on the channel forever. SetAnalyzerEnabled could not switch them off
// either: it only reaches analyzerRegistry.m[id], i.e. the newest instance, so
// every predecessor was orphaned in the chain, enabled and unreachable.
//
// That mattered because setEQActiveChannel re-issues EnableChannelAnalyzer on
// EVERY channel focus, and EnsureAnalyzersForTab re-issues it for master + every
// row instrument on EVERY qualifying tab transition. Soloing a row auto-focuses
// its channel (reconcileChannelWithAudibleSet), so each solo toggle permanently
// added FFT load to the audio thread — the opposite of what soloing should do,
// since it strictly removes scheduled voices. The JS path already guards this;
// see the "Idempotent" comment in src/js/audio.js's enableChannelAnalyzer.
//
// The contract mirrors JS: reuse the analyzer already wired into the channel
// when its window matches; otherwise replace it. Reuse is decided from the
// CHANNEL CHAIN, never from analyzerRegistry — SetChannelEQ →
// RebuildChannelWithEQ rebuilds eqProcs from scratch and drops the analyzers
// while leaving the registry entry dangling, and applyMasterEQ/applyRowEQ rely
// on the next Enable call re-wiring a dropped analyzer.

// effectiveAnalyzerWindow returns the window size NewAnalyzer would actually
// allocate for the requested size, so callers can compare "already wired at this
// size" without constructing an Analyzer.
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
func lookupChannel(id string) *Channel {
	chanMgr.mu.RLock()
	defer chanMgr.mu.RUnlock()
	if ch := chanMgr.channels[id]; ch != nil {
		return ch
	}
	return chanMgr.instruments[id]
}

// eqAnalyzer returns the analyzer currently wired into the channel's eqProcs,
// or nil when none is wired.
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

// replaceEQAnalyzer drops every *Analyzer currently in the channel's eqProcs and
// appends an in their place, keeping the chain to at most one analyzer tap.
// Mirrors addEQProcessor's bookkeeping (canonical processors list + block
// capability cache).
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

// wireChannelAnalyzer is the shared idempotent implementation behind
// EnableChannelAnalyzer on both the real and stubbed builds. It returns the
// analyzer wired into the channel for id, always enabled.
//
// Enabling on the reuse path is what keeps this a pure de-duplication: the old
// append-always code handed back a freshly constructed analyzer, and NewAnalyzer
// starts enabled, so every Enable* call used to yield an enabled tap. Callers
// depend on that — applyMasterEQ/applyRowEQ re-issue Enable* purely to keep
// waveform/spectrum live after SetChannelEQ rebuilds the chain, and never call
// SetAnalyzerEnabled themselves. Reusing a tap that setEQActiveChannel had
// disabled would silently leave those panels dead.
func wireChannelAnalyzer(id string, window int) *Analyzer {
	ch := chanMgr.ensureChannel(id)
	if cur := ch.eqAnalyzer(); cur != nil && len(cur.window) == effectiveAnalyzerWindow(window) {
		// Already wired at this size — reuse it rather than stacking another
		// FFT onto the audio thread.
		cur.SetEnabled(true)
		return cur
	}
	an := NewAnalyzer(window)
	ch.replaceEQAnalyzer(an)
	return an
}

// ChannelAnalyzerCount reports how many live Analyzer taps sit in the channel's
// processing chain. Each one costs a full FFT on the audio thread per window, so
// this is a direct proxy for that channel's analyzer CPU load. Returns 0 for an
// unknown channel and never creates one, so it is safe in assertions.
func ChannelAnalyzerCount(id string) int {
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
