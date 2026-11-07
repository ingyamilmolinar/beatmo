package audio

import (
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

var debugChannelCount int64

type Processor interface {
	ProcessSample(float64) float64
}

// When EVERY processor in a channel implements it, one cgo call per effect per block replaces per-sample calls.
type BlockProcessor interface {
	Processor
	ProcessBlockBuf(in []float32, out []float32, samples int)
}

type Channel struct {
	id     channel.ID
	parent *Channel
	vol    uint64 // atomic float64 bits

	// The insert chain's ChainMemorySec as atomic float64 bits: how long the channel holds a sound with silent output.
	memory atomic.Uint64

	mu sync.RWMutex
	// Split so the mixer can tap post-inserts / pre-EQ (scope.StageInsertFX): input → volume → preEQ tap → inserts →
	// tap → eqProcs → output. `processors` stays the concatenation for the ProcessSample / ProcessBlock paths.
	inserts       []Processor
	eqProcs       []Processor
	processors    []Processor // = append(inserts, eqProcs...); used by legacy paths
	preEQAnalyzer *Analyzer   // tapped after volume, before inserts

	// The combined caches drive ProcessBlockLocal, the split ones ProcessInsertsBlockLocal / ProcessEQBlockLocal.
	allBlock          bool
	blockProcs        []BlockProcessor
	insertsAllBlock   bool
	insertsBlockProcs []BlockProcessor
	eqAllBlock        bool
	eqBlockProcs      []BlockProcessor

	// The old chain is kept briefly and blended with the new one, so a chain swap (an effect toggle) doesn't click.
	oldProcessors  []Processor
	xfadePos       int
	xfadeLen       int
	samplesThruOld uint32 // atomic: >0 if audio was processed with current chain
}

func newChannel(id channel.ID, parent *Channel) *Channel {
	ch := &Channel{id: id, parent: parent}
	ch.SetVolume(1)
	return ch
}

func (c *Channel) ID() channel.ID { return c.id }

func (c *Channel) Volume() float64 {
	return math.Float64frombits(atomic.LoadUint64(&c.vol))
}

func (c *Channel) SetVolume(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 1
	}
	if v < 0 {
		v = 0
	}
	atomic.StoreUint64(&c.vol, math.Float64bits(v))
}

func (c *Channel) ProcessSample(input float64) float64 {
	gain := c.Volume()
	out := input * gain

	// Skip processors (EQ) if bypassing for debugging
	if !env.BypassEQ.Get() {
		c.mu.RLock()
		procs := c.processors
		c.mu.RUnlock()
		for _, p := range procs {
			out = p.ProcessSample(out)
		}
	}

	// Debug logging (every 10000 samples to avoid spam)
	if env.DebugChannel.Get() && debugChannelCount%10000 == 0 {
		log.Tracef("[channel] %s input=%.6f gain=%.6f out=%.6f hasParent=%v",
			c.id, input, gain, out, c.parent != nil)
	}
	debugChannelCount++

	if c.parent != nil {
		return c.parent.ProcessSample(out)
	}
	return out
}

// Chains through parent channels; the lock is taken once per block.
func (c *Channel) ProcessBlock(input, output []float64) {
	gain := c.Volume()

	var procs []Processor
	if !env.BypassEQ.Get() {
		c.mu.RLock()
		procs = c.processors
		c.mu.RUnlock()
	}

	if c.parent != nil {
		for i, x := range input {
			out := x * gain
			for _, p := range procs {
				out = p.ProcessSample(out)
			}
			output[i] += c.parent.ProcessSample(out)
		}
	} else {
		for i, x := range input {
			out := x * gain
			for _, p := range procs {
				out = p.ProcessSample(out)
			}
			output[i] += out
		}
	}
}

// No recursion to the parent: instrument and master channels must each see one coherent stream, or the shared
// biquad state is corrupted.
func (c *Channel) ProcessSampleLocal(input float64) float64 {
	gain := c.Volume()
	out := input * gain

	// Mark that audio has been processed with the current chain.
	atomic.StoreUint32(&c.samplesThruOld, 1)

	if !env.BypassEQ.Get() {
		c.mu.RLock()
		procs := c.processors
		oldProcs := c.oldProcessors
		xfadePos := c.xfadePos
		xfadeLen := c.xfadeLen
		c.mu.RUnlock()

		if oldProcs != nil && xfadePos < xfadeLen {
			// Crossfade between old and new processor chains.
			oldOut := out
			for _, p := range oldProcs {
				oldOut = p.ProcessSample(oldOut)
			}
			newOut := out
			for _, p := range procs {
				newOut = p.ProcessSample(newOut)
			}
			t := float64(xfadePos) / float64(xfadeLen)
			out = oldOut*(1-t) + newOut*t

			c.mu.Lock()
			c.xfadePos++
			if c.xfadePos >= c.xfadeLen {
				c.oldProcessors = nil
			}
			c.mu.Unlock()
		} else {
			for _, p := range procs {
				out = p.ProcessSample(out)
			}
		}
	}

	return out
}

// No recursion to the parent; processors are snapshotted once per block. When all implement BlockProcessor, each
// effect runs once per block over ping-pong float32 buffers instead of once per sample.
func (c *Channel) ProcessBlockLocal(input, output []float64) {
	gain := c.Volume() // atomic read, no lock needed

	var procs []Processor
	c.mu.RLock()
	var allBlock bool
	var blockProcs []BlockProcessor
	if !env.BypassEQ.Get() {
		procs = c.processors // snapshot slice header (safe: replaceProcessors creates new slice)
		allBlock = c.allBlock
		blockProcs = c.blockProcs
	}
	preEQ := c.preEQAnalyzer
	c.mu.RUnlock()

	n := len(input)

	if allBlock && n > 0 {
		// Optimized block path: apply volume, then ping-pong through block processors.
		buf0 := blockBufPool.get(n)
		buf1 := blockBufPool.get(n)
		defer blockBufPool.put(buf0)
		defer blockBufPool.put(buf1)

		// Fill buf0 with gain-scaled input.
		for i, x := range input {
			buf0[i] = float32(x * gain)
		}

		// Tap pre-EQ signal.
		if preEQ != nil {
			preEQ.ProcessBlock(buf0, n)
		}

		// Ping-pong through block processors.
		src, dst := buf0, buf1
		for _, bp := range blockProcs {
			bp.ProcessBlockBuf(src, dst, n)
			src, dst = dst, src
		}

		// Accumulate result (now in src) into output.
		for i := 0; i < n; i++ {
			output[i] += float64(src[i])
		}
	} else {
		// Per-sample fallback.
		for i, x := range input {
			out := x * gain
			if preEQ != nil {
				preEQ.ProcessSample(out)
			}
			for _, p := range procs {
				out = p.ProcessSample(out)
			}
			output[i] += out
		}
	}
}

// Volume, the pre-EQ tap and the inserts, accumulated into output, so the mixer can tap the intermediate buffer as
// StageInsertFX before ProcessEQBlockLocal. No parent recursion; output is read-modify-write.
func (c *Channel) ProcessInsertsBlockLocal(input, output []float64) {
	gain := c.Volume()

	var procs []Processor
	c.mu.RLock()
	var allBlock bool
	var blockProcs []BlockProcessor
	if !env.BypassEQ.Get() {
		procs = c.inserts
		allBlock = c.insertsAllBlock
		blockProcs = c.insertsBlockProcs
	}
	preEQ := c.preEQAnalyzer
	c.mu.RUnlock()

	n := len(input)
	if n == 0 {
		return
	}

	if len(procs) > 0 && allBlock {
		buf0 := blockBufPool.get(n)
		buf1 := blockBufPool.get(n)
		defer blockBufPool.put(buf0)
		defer blockBufPool.put(buf1)

		for i, x := range input {
			buf0[i] = float32(x * gain)
		}
		if preEQ != nil {
			preEQ.ProcessBlock(buf0, n)
		}
		src, dst := buf0, buf1
		for _, bp := range blockProcs {
			bp.ProcessBlockBuf(src, dst, n)
			src, dst = dst, src
		}
		for i := 0; i < n; i++ {
			output[i] += float64(src[i])
		}
		return
	}

	// Per-sample fallback.
	for i, x := range input {
		out := x * gain
		if preEQ != nil {
			preEQ.ProcessSample(out)
		}
		for _, p := range procs {
			out = p.ProcessSample(out)
		}
		output[i] += out
	}
}

// Volume was already applied upstream; no parent recursion. An empty eqProcs copy-accumulates; output is
// read-modify-write.
func (c *Channel) ProcessEQBlockLocal(input, output []float64) {
	var procs []Processor
	c.mu.RLock()
	var allBlock bool
	var blockProcs []BlockProcessor
	if !env.BypassEQ.Get() {
		procs = c.eqProcs
		allBlock = c.eqAllBlock
		blockProcs = c.eqBlockProcs
	}
	c.mu.RUnlock()

	n := len(input)
	if n == 0 {
		return
	}

	if len(procs) == 0 {
		for i := 0; i < n; i++ {
			output[i] += input[i]
		}
		return
	}

	if allBlock {
		buf0 := blockBufPool.get(n)
		buf1 := blockBufPool.get(n)
		defer blockBufPool.put(buf0)
		defer blockBufPool.put(buf1)

		for i, x := range input {
			buf0[i] = float32(x)
		}
		src, dst := buf0, buf1
		for _, bp := range blockProcs {
			bp.ProcessBlockBuf(src, dst, n)
			src, dst = dst, src
		}
		for i := 0; i < n; i++ {
			output[i] += float64(src[i])
		}
		return
	}

	// Per-sample fallback.
	for i, x := range input {
		out := x
		for _, p := range procs {
			out = p.ProcessSample(out)
		}
		output[i] += out
	}
}

// blockBufPool is a simple pool for reusable float32 block buffers.
var blockBufPool = newBlockPool()

type blockPool struct {
	pool [][]float32
}

func newBlockPool() *blockPool { return &blockPool{} }

func (p *blockPool) get(n int) []float32 {
	for i, b := range p.pool {
		if cap(b) >= n {
			p.pool = append(p.pool[:i], p.pool[i+1:]...)
			return b[:n]
		}
	}
	return make([]float32, n)
}

func (p *blockPool) put(b []float32) {
	p.pool = append(p.pool, b)
}

// Caller must hold c.mu.
func (c *Channel) cacheBlockCapability() {
	c.allBlock, c.blockProcs = computeBlockCache(c.processors)
	c.insertsAllBlock, c.insertsBlockProcs = computeBlockCache(c.inserts)
	c.eqAllBlock, c.eqBlockProcs = computeBlockCache(c.eqProcs)
}

// An empty slice also returns (false, nil): callers check len first, so it only means "no fast path".
func computeBlockCache(procs []Processor) (bool, []BlockProcessor) {
	if len(procs) == 0 {
		return false, nil
	}
	bp := make([]BlockProcessor, len(procs))
	for i, p := range procs {
		b, ok := p.(BlockProcessor)
		if !ok {
			return false, nil
		}
		bp[i] = b
	}
	return true, bp
}

// About 5 ms.
const channelXfadeSamples = 220

// inserts run first, eqProcs after; the split lets the mixer tap between them, while c.processors keeps the
// concatenation for the legacy paths.
func (c *Channel) replaceProcessors(inserts, eqProcs []Processor) {
	c.mu.Lock()
	// Only when audio actually ran through the current chain: no crossfade for setup before playback or for rapid
	// changes with no audio in between.
	if atomic.LoadUint32(&c.samplesThruOld) > 0 {
		c.oldProcessors = c.processors
		c.xfadePos = 0
		c.xfadeLen = channelXfadeSamples
	}
	c.inserts = append([]Processor(nil), inserts...)
	c.eqProcs = append([]Processor(nil), eqProcs...)
	c.processors = append(append([]Processor(nil), c.inserts...), c.eqProcs...)
	atomic.StoreUint32(&c.samplesThruOld, 0)
	c.cacheBlockCapability()
	c.mu.Unlock()
	chanMgr.refreshProcChannels()
}

// For AddChannelProcessor, which knows no insert/EQ split.
func (c *Channel) addEQProcessor(p Processor) {
	c.mu.Lock()
	c.eqProcs = append(c.eqProcs, p)
	c.processors = append(append([]Processor(nil), c.inserts...), c.eqProcs...)
	c.cacheBlockCapability()
	c.mu.Unlock()
	chanMgr.refreshProcChannels()
}

type channelManager struct {
	mu          sync.RWMutex
	channels    map[channel.ID]*Channel
	instruments map[channel.ID]*Channel
	main        *Channel

	// Channels whose chain holds a processor: the mixer runs each every block, voice or not.
	procChannels atomic.Pointer[[]*Channel]
}

// Never call it holding a Channel's mu or cm.mu.
func (cm *channelManager) refreshProcChannels() {
	cm.mu.RLock()
	var out []*Channel
	for id, ch := range cm.channels {
		if id == channel.Master {
			continue
		}
		ch.mu.RLock()
		has := len(ch.inserts)+len(ch.eqProcs) > 0
		ch.mu.RUnlock()
		if has {
			out = append(out, ch)
		}
	}
	cm.mu.RUnlock()
	cm.procChannels.Store(&out)
}

func newChannelManager() *channelManager {
	cm := &channelManager{}
	cm.reset() // initializes maps and main channel
	return cm
}

func (cm *channelManager) reset() {
	cm.mu.Lock()
	cm.channels = make(map[channel.ID]*Channel)
	cm.instruments = make(map[channel.ID]*Channel)
	cm.main = newChannel(channel.Master, nil)
	cm.main.SetVolume(FreshSessionMasterVolume)
	cm.channels[channel.Master] = cm.main
	cm.mu.Unlock()
	cm.procChannels.Store(&[]*Channel{}) // fresh channels hold no processor
}

// The zero ID names no channel: it gets a detached channel nothing reads, so an edit addressed to it is dropped
// instead of landing on the master bus.
func (cm *channelManager) ensureChannel(id channel.ID) *Channel {
	if id.IsZero() {
		return newChannel(id, nil)
	}
	cm.mu.RLock()
	ch, ok := cm.channels[id]
	cm.mu.RUnlock()
	if ok {
		return ch
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if ch, ok = cm.channels[id]; ok {
		return ch
	}
	parent := cm.main
	if id == channel.Master {
		parent = nil
	}
	ch = newChannel(id, parent)
	cm.channels[id] = ch
	if parent == nil {
		cm.main = ch
	}
	return ch
}

func (cm *channelManager) ensureInstrumentChannel(id channel.ID) *Channel {
	if id.IsZero() {
		return newChannel(id, nil) // no channel: detached, see ensureChannel
	}
	cm.mu.RLock()
	ch, ok := cm.instruments[id]
	cm.mu.RUnlock()
	if ok {
		return ch
	}
	created := false
	cm.mu.Lock()
	if ch, ok = cm.instruments[id]; !ok {
		if existing, ok := cm.channels[id]; ok {
			ch = existing
		} else {
			ch = newChannel(id, cm.main)
			cm.channels[id] = ch
		}
		cm.instruments[id] = ch
		created = true
	}
	cm.mu.Unlock()
	if created {
		platformChannelVolumeChanged(id, ch.Volume())
	}
	return ch
}

// The master bus for a voice no instrument owns.
func (cm *channelManager) channelForInstrument(id channel.ID) *Channel {
	if id.IsZero() {
		return cm.main
	}
	cm.mu.RLock()
	ch, ok := cm.instruments[id]
	cm.mu.RUnlock()
	if ok {
		return ch
	}
	return cm.ensureInstrumentChannel(id)
}

func (cm *channelManager) setVolume(id channel.ID, vol float64) *Channel {
	ch := cm.ensureChannel(id)
	ch.SetVolume(vol)
	return ch
}

// Without building it: a channel that does not exist yet is at unity.
func (cm *channelManager) volume(id channel.ID) float64 {
	cm.mu.RLock()
	ch, ok := cm.channels[id]
	cm.mu.RUnlock()
	if !ok {
		return 1
	}
	return ch.Volume()
}

// None is rebuilt: a channel exists from its first use on.
func (cm *channelManager) resetInstruments() {
	cm.mu.Lock()
	main := cm.main
	cm.channels = map[channel.ID]*Channel{channel.Master: main}
	cm.instruments = map[channel.ID]*Channel{}
	cm.mu.Unlock()
	cm.refreshProcChannels()
}

var chanMgr = newChannelManager()

func init() {
	platformChannelVolumeChanged(channel.Master, chanMgr.main.Volume())
}

func ChannelVolume(id channel.ID) float64 {
	return chanMgr.volume(id)
}

func SetChannelVolume(id channel.ID, vol float64) {
	ch := chanMgr.setVolume(id, vol)
	platformChannelVolumeChanged(ch.id, ch.Volume())
}

// Headroom for the loudness-normalised instruments; also what a project without master_volume gets.
const FreshSessionMasterVolume = 0.5

// The one clamp both platforms' master gain receives.
func SetMainVolume(vol float64) {
	if vol > MaxMasterVolume {
		vol = MaxMasterVolume
	}
	ch := chanMgr.setVolume(channel.Master, vol)
	platformChannelVolumeChanged(ch.id, ch.Volume())
}

func MainVolume() float64 {
	return chanMgr.volume(channel.Master)
}

func InstrumentChannel(id channel.ID) *Channel {
	ch := chanMgr.ensureInstrumentChannel(id)
	platformChannelVolumeChanged(ch.id, ch.Volume())
	return ch
}

func resetChannels() {
	chanMgr.reset()
	platformChannelVolumeChanged(channel.Master, chanMgr.main.Volume())
	resetAnalyzers()
}

func resetInstrumentChannels() {
	chanMgr.resetInstruments()
	platformChannelVolumeChanged(channel.Master, chanMgr.main.Volume())
	resetAnalyzers()
}

func channelForInstrument(id channel.ID) *Channel {
	return chanMgr.channelForInstrument(id)
}

// Replaces only the EQ-side chain; the inserts are untouched.
func SetChannelProcessors(id channel.ID, procs ...Processor) {
	ch := chanMgr.ensureChannel(id)
	ch.replaceProcessors(nil, procs)
}

// AddChannelProcessor appends a processor to the existing chain.
func AddChannelProcessor(id channel.ID, p Processor) {
	ch := chanMgr.ensureChannel(id)
	ch.addEQProcessor(p)
}

// A no-op where Go mixes.
var platformChannelVolumeChanged = func(channel.ID, float64) {}

// Import clears per-instrument EQ first, so an instrument flat in the new project does not inherit the old curve.
func InstrumentChannelIDs() []channel.ID {
	chanMgr.mu.RLock()
	defer chanMgr.mu.RUnlock()
	out := make([]channel.ID, 0, len(chanMgr.instruments))
	for id := range chanMgr.instruments {
		out = append(out, id)
	}
	slices.SortFunc(out, channel.Compare)
	return out
}
