package audio

import (
	"log"
	"math"
	"os"
	"sync"
	"sync/atomic"
)

const mainChannelID = "main"

// bypassEQ skips all EQ/processor processing when set via BYPASS_EQ=1.
// Use this to isolate whether distortion comes from EQ processing.
var bypassEQ = os.Getenv("BYPASS_EQ") == "1"

// debugChannelProc logs channel processing values when set via DEBUG_CHANNEL=1.
var debugChannelProc = os.Getenv("DEBUG_CHANNEL") == "1"
var debugChannelCount int64

func init() {
	if bypassEQ {
		log.Println("[AUDIO DEBUG] BYPASS_EQ=1 - Skipping all EQ/processor processing")
	}
	if debugChannelProc {
		log.Println("[AUDIO DEBUG] DEBUG_CHANNEL=1 - Logging channel processing values")
	}
}

// Processor represents a placeholder audio processor in the channel chain.
// Future implementations can provide EQ, effects, etc.
type Processor interface {
	ProcessSample(float64) float64
}

// BlockProcessor is an optional interface for processors that support
// efficient block-based processing (e.g., C effects via CGo).
// When ALL processors in a channel implement BlockProcessor, the channel
// can use an optimized path with one CGo call per effect per block instead
// of per-sample calls.
type BlockProcessor interface {
	Processor
	ProcessBlockBuf(in []float32, out []float32, samples int)
}

type Channel struct {
	id     string
	parent *Channel
	vol    uint64 // atomic float64 bits
	panVal uint64 // atomic float64 bits: -1 (left) to +1 (right), 0 = center

	mu            sync.RWMutex
	processors    []Processor
	preEQAnalyzer *Analyzer // tapped after volume, before EQ processors

	// Cached block-processing capability (recomputed when processors change).
	allBlock   bool             // true when all processors implement BlockProcessor
	blockProcs []BlockProcessor // pre-cast processors (nil if !allBlock or no processors)

	// Crossfade state: when processors are replaced, the old chain is kept
	// briefly and blended with the new chain to avoid clicks from abrupt
	// chain swaps (e.g., effect toggle on/off).
	oldProcessors  []Processor
	xfadePos       int
	xfadeLen       int
	samplesThruOld uint32 // atomic: >0 if audio was processed with current chain
}

func newChannel(id string, parent *Channel) *Channel {
	ch := &Channel{id: id, parent: parent}
	ch.SetVolume(1)
	return ch
}

func (c *Channel) ID() string { return c.id }

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

// Pan returns the stereo pan position: -1 (left) to +1 (right), 0 = center.
func (c *Channel) Pan() float64 {
	return math.Float64frombits(atomic.LoadUint64(&c.panVal))
}

// SetPan sets the stereo pan position: -1 (left) to +1 (right), 0 = center.
func (c *Channel) SetPan(v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	if v < -1 {
		v = -1
	}
	if v > 1 {
		v = 1
	}
	atomic.StoreUint64(&c.panVal, math.Float64bits(v))
}

// PanGains returns the left and right gain factors for equal-power panning.
func (c *Channel) PanGains() (gainL, gainR float64) {
	pan := c.Pan()
	angle := (pan + 1) * math.Pi * 0.25
	return math.Cos(angle), math.Sin(angle)
}

func (c *Channel) ProcessSample(input float64) float64 {
	gain := c.Volume()
	out := input * gain

	// Skip processors (EQ) if bypassing for debugging
	if !bypassEQ {
		c.mu.RLock()
		procs := c.processors
		c.mu.RUnlock()
		for _, p := range procs {
			out = p.ProcessSample(out)
		}
	}

	// Debug logging (every 10000 samples to avoid spam)
	if debugChannelProc && debugChannelCount%10000 == 0 {
		log.Printf("[CHANNEL %s] input=%.6f gain=%.6f out=%.6f hasParent=%v",
			c.id, input, gain, out, c.parent != nil)
	}
	debugChannelCount++

	if c.parent != nil {
		return c.parent.ProcessSample(out)
	}
	return out
}

// ProcessBlock processes input samples and adds results to output.
// Chains through parent channels. Hoists lock outside the sample loop.
func (c *Channel) ProcessBlock(input, output []float64) {
	gain := c.Volume()

	var procs []Processor
	if !bypassEQ {
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

// ProcessSampleLocal applies this channel's volume and EQ processors but does
// NOT recurse to the parent channel. Used by the 3-phase mixer so that
// instrument channels and the master channel are processed separately with
// coherent signal streams (avoiding shared biquad state corruption).
func (c *Channel) ProcessSampleLocal(input float64) float64 {
	gain := c.Volume()
	out := input * gain

	// Mark that audio has been processed with the current chain.
	atomic.StoreUint32(&c.samplesThruOld, 1)

	if !bypassEQ {
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

// ProcessBlockLocal applies this channel's processing to each input sample and
// accumulates results into output. Does NOT recurse to the parent channel.
// Hoists the RWMutex read-lock outside the sample loop so processors are
// snapshotted once per block instead of once per sample.
//
// When ALL processors implement BlockProcessor, uses an optimized path that
// calls each effect's block method with ping-pong float32 buffers, reducing
// CGo overhead from (samples * effects) to just (effects) calls per block.
func (c *Channel) ProcessBlockLocal(input, output []float64) {
	gain := c.Volume() // atomic read, no lock needed

	var procs []Processor
	c.mu.RLock()
	var allBlock bool
	var blockProcs []BlockProcessor
	if !bypassEQ {
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

// cacheBlockCapability recomputes the allBlock / blockProcs cache.
// Caller must hold c.mu.
func (c *Channel) cacheBlockCapability() {
	if len(c.processors) == 0 {
		c.allBlock = false
		c.blockProcs = nil
		return
	}
	bp := make([]BlockProcessor, len(c.processors))
	for i, p := range c.processors {
		if b, ok := p.(BlockProcessor); ok {
			bp[i] = b
		} else {
			c.allBlock = false
			c.blockProcs = nil
			return
		}
	}
	c.allBlock = true
	c.blockProcs = bp
}

// channelXfadeSamples is the crossfade duration when the processor chain is
// replaced, preventing clicks from abrupt chain swaps (e.g., effect toggle).
// ~5ms at 44100 Hz.
const channelXfadeSamples = 220

func (c *Channel) replaceProcessors(list []Processor) {
	c.mu.Lock()
	// Only crossfade if audio was actually processed with the current chain.
	// This avoids crossfading when effects are configured before playback starts,
	// or when multiple chain changes happen in rapid succession without audio.
	if atomic.LoadUint32(&c.samplesThruOld) > 0 {
		c.oldProcessors = c.processors
		c.xfadePos = 0
		c.xfadeLen = channelXfadeSamples
	}
	c.processors = append([]Processor(nil), list...)
	atomic.StoreUint32(&c.samplesThruOld, 0)
	c.cacheBlockCapability()
	c.mu.Unlock()
}

func (c *Channel) addProcessor(p Processor) {
	c.mu.Lock()
	c.processors = append(c.processors, p)
	c.cacheBlockCapability()
	c.mu.Unlock()
}

type channelManager struct {
	mu          sync.RWMutex
	channels    map[string]*Channel
	instruments map[string]*Channel
	main        *Channel
}

func newChannelManager() *channelManager {
	cm := &channelManager{}
	cm.reset() // initializes maps and main channel
	return cm
}

func (cm *channelManager) reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.channels = make(map[string]*Channel)
	cm.instruments = make(map[string]*Channel)
	cm.main = newChannel(mainChannelID, nil)
	cm.channels[mainChannelID] = cm.main
}

func (cm *channelManager) ensureChannel(id string) *Channel {
	if id == "" {
		id = mainChannelID
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
	if id == mainChannelID {
		parent = nil
	}
	ch = newChannel(id, parent)
	cm.channels[id] = ch
	if parent == nil {
		cm.main = ch
	}
	return ch
}

func (cm *channelManager) ensureInstrumentChannel(id string) *Channel {
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

func (cm *channelManager) channelForInstrument(id string) *Channel {
	if id == "" {
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

func (cm *channelManager) setVolume(id string, vol float64) *Channel {
	ch := cm.ensureChannel(id)
	ch.SetVolume(vol)
	return ch
}

func (cm *channelManager) volume(id string) float64 {
	ch := cm.ensureChannel(id)
	return ch.Volume()
}

func (cm *channelManager) renameInstrument(oldID, newID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	ch, ok := cm.instruments[oldID]
	if !ok {
		if _, exists := cm.instruments[newID]; !exists {
			cm.instruments[newID] = newChannel(newID, cm.main)
			cm.channels[newID] = cm.instruments[newID]
		}
		delete(cm.instruments, oldID)
		return
	}
	delete(cm.channels, oldID)
	delete(cm.instruments, oldID)
	ch.id = newID
	cm.channels[newID] = ch
	cm.instruments[newID] = ch
}

func (cm *channelManager) resetInstruments(ids []string) {
	cm.mu.Lock()
	main := cm.main
	cm.channels = map[string]*Channel{mainChannelID: main}
	cm.instruments = make(map[string]*Channel, len(ids))
	cm.mu.Unlock()
	for _, id := range ids {
		cm.ensureInstrumentChannel(id)
	}
}

var chanMgr = newChannelManager()

func init() {
	platformChannelVolumeChanged(mainChannelID, chanMgr.main.Volume())
}

func ChannelVolume(id string) float64 {
	return chanMgr.volume(id)
}

func SetChannelVolume(id string, vol float64) {
	ch := chanMgr.setVolume(id, vol)
	platformChannelVolumeChanged(ch.id, ch.Volume())
}

func SetMainVolume(vol float64) {
	ch := chanMgr.setVolume(mainChannelID, vol)
	platformChannelVolumeChanged(ch.id, ch.Volume())
}

func MainVolume() float64 {
	return chanMgr.volume(mainChannelID)
}

// SetChannelPan sets the stereo pan for an instrument channel.
func SetChannelPan(id string, pan float64) {
	ch := chanMgr.ensureChannel(id)
	ch.SetPan(pan)
	platformChannelPanChanged(ch.id, ch.Pan())
}

// ChannelPan returns the current pan position for a channel.
func ChannelPan(id string) float64 {
	ch := chanMgr.ensureChannel(id)
	return ch.Pan()
}

func InstrumentChannel(id string) *Channel {
	ch := chanMgr.ensureInstrumentChannel(id)
	platformChannelVolumeChanged(ch.id, ch.Volume())
	return ch
}

func resetChannels() {
	chanMgr.reset()
	platformChannelVolumeChanged(mainChannelID, chanMgr.main.Volume())
	resetAnalyzers()
}

func resetInstrumentChannels(ids []string) {
	chanMgr.resetInstruments(ids)
	platformChannelVolumeChanged(mainChannelID, chanMgr.main.Volume())
	for _, id := range ids {
		platformChannelVolumeChanged(id, chanMgr.volume(id))
	}
	resetAnalyzers()
}

func renameInstrumentChannel(oldID, newID string) {
	chanMgr.renameInstrument(oldID, newID)
	platformChannelVolumeChanged(newID, chanMgr.volume(newID))
}

func channelForInstrument(id string) *Channel {
	return chanMgr.channelForInstrument(id)
}

// SetChannelProcessors replaces the processor chain for the given channel ID.
func SetChannelProcessors(id string, procs ...Processor) {
	ch := chanMgr.ensureChannel(id)
	ch.replaceProcessors(procs)
}

// AddChannelProcessor appends a processor to the existing chain.
func AddChannelProcessor(id string, p Processor) {
	ch := chanMgr.ensureChannel(id)
	ch.addProcessor(p)
}

// platformChannelVolumeChanged is implemented per-platform to propagate volume
// changes down to the audio backend (e.g., JS GainNodes). On platforms where
// mixing is performed in Go, it may be a no-op.
var platformChannelVolumeChanged = func(string, float64) {}

// platformChannelPanChanged is implemented per-platform to propagate pan
// changes to the audio backend (e.g., JS StereoPannerNode).
var platformChannelPanChanged = func(string, float64) {}
