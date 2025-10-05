package audio

import (
	"math"
	"sync"
	"sync/atomic"
)

const mainChannelID = "main"

// Processor represents a placeholder audio processor in the channel chain.
// Future implementations can provide EQ, effects, etc.
type Processor interface {
	ProcessSample(float64) float64
}

type Channel struct {
	id     string
	parent *Channel
	vol    uint64 // atomic float64 bits

	mu         sync.RWMutex
	processors []Processor
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

func (c *Channel) ProcessSample(input float64) float64 {
	gain := c.Volume()
	out := input * gain
	c.mu.RLock()
	procs := c.processors
	c.mu.RUnlock()
	for _, p := range procs {
		out = p.ProcessSample(out)
	}
	if c.parent != nil {
		return c.parent.ProcessSample(out)
	}
	return out
}

func (c *Channel) replaceProcessors(list []Processor) {
	c.mu.Lock()
	c.processors = append([]Processor(nil), list...)
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
		ch = newChannel(id, cm.main)
		cm.channels[id] = ch
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

func InstrumentChannel(id string) *Channel {
	ch := chanMgr.ensureInstrumentChannel(id)
	platformChannelVolumeChanged(ch.id, ch.Volume())
	return ch
}

func resetChannels() {
	chanMgr.reset()
	platformChannelVolumeChanged(mainChannelID, chanMgr.main.Volume())
}

func resetInstrumentChannels(ids []string) {
	chanMgr.resetInstruments(ids)
	platformChannelVolumeChanged(mainChannelID, chanMgr.main.Volume())
	for _, id := range ids {
		platformChannelVolumeChanged(id, chanMgr.volume(id))
	}
}

func renameInstrumentChannel(oldID, newID string) {
	chanMgr.renameInstrument(oldID, newID)
	platformChannelVolumeChanged(newID, chanMgr.volume(newID))
}

func channelForInstrument(id string) *Channel {
	return chanMgr.channelForInstrument(id)
}

// platformChannelVolumeChanged is implemented per-platform to propagate volume
// changes down to the audio backend (e.g., JS GainNodes). On platforms where
// mixing is performed in Go, it may be a no-op.
var platformChannelVolumeChanged = func(string, float64) {}
