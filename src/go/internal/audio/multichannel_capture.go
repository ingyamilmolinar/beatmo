package audio

import (
	"sync"
	"sync/atomic"
	"time"
)

// CaptureChannel holds the accumulated audio samples for one instrument or master.
type CaptureChannel struct {
	ID      string    // instrument ID or "master"
	Name    string    // human-readable name
	Samples []float64 // accumulated post-processing audio
}

// MultiChannelCapture captures audio from all instrument channels and master
// simultaneously during playback. It is designed for minimal hot-path overhead:
// the atomic pointer check is zero-cost when not recording.
type MultiChannelCapture struct {
	mu           sync.Mutex
	channels     map[string]*CaptureChannel // keyed by instrument ID
	masterCh     *CaptureChannel
	sampleRate   int
	startTime    time.Time
	maxSamples   int // 0 = unlimited
	totalSamples int
	done         bool // set when auto-stopped due to maxSamples

	// MIDI event capture (populated by Schedule() when recording)
	midiEvents []MIDIEvent
}

// multiCapturePtr is the global atomic pointer checked on the hot audio path.
// nil = not recording. Non-nil = actively capturing.
var multiCapturePtr atomic.Pointer[MultiChannelCapture]

// newMultiChannelCapture creates a capture with pre-allocated channels.
func newMultiChannelCapture(instruments []InstrumentMeta, sampleRate int, maxDuration time.Duration) *MultiChannelCapture {
	mc := &MultiChannelCapture{
		channels:   make(map[string]*CaptureChannel, len(instruments)+1),
		sampleRate: sampleRate,
		startTime:  time.Now(),
	}

	if maxDuration > 0 {
		mc.maxSamples = int(maxDuration.Seconds() * float64(sampleRate))
	}

	// Pre-allocate channels for each instrument
	estimatedSamples := sampleRate * 30 // pre-allocate ~30s worth
	if mc.maxSamples > 0 && mc.maxSamples < estimatedSamples {
		estimatedSamples = mc.maxSamples
	}

	for _, inst := range instruments {
		mc.channels[inst.ID] = &CaptureChannel{
			ID:      inst.ID,
			Name:    inst.Name,
			Samples: make([]float64, 0, estimatedSamples),
		}
	}

	mc.masterCh = &CaptureChannel{
		ID:      "master",
		Name:    "Master",
		Samples: make([]float64, 0, estimatedSamples),
	}

	return mc
}

// appendBlock is called from processBlock on the hot audio path.
// It appends per-instrument and master audio data for one block.
// instBufs contains post-Phase-2 audio (after channel EQ + insert effects).
// workBuf contains post-Phase-3 audio (after master EQ).
func (mc *MultiChannelCapture) appendBlock(
	instBufs [][]float64, slotIDs []string, activeSlots []int,
	workBuf []float64, blockLen int,
) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.done {
		return
	}

	// Check max duration limit
	remaining := blockLen
	if mc.maxSamples > 0 {
		left := mc.maxSamples - mc.totalSamples
		if left <= 0 {
			mc.done = true
			return
		}
		if remaining > left {
			remaining = left
		}
	}

	// Append per-instrument buffers
	for _, slot := range activeSlots {
		if slot >= len(slotIDs) {
			continue
		}
		id := slotIDs[slot]
		ch, ok := mc.channels[id]
		if !ok {
			// Instrument appeared after recording started; create channel on the fly
			ch = &CaptureChannel{
				ID:      id,
				Name:    id,
				Samples: make([]float64, mc.totalSamples, mc.totalSamples+mc.sampleRate*10),
			}
			mc.channels[id] = ch
		}
		// Pad with silence if this channel was inactive in earlier blocks
		if len(ch.Samples) < mc.totalSamples {
			pad := mc.totalSamples - len(ch.Samples)
			ch.Samples = append(ch.Samples, make([]float64, pad)...)
		}
		ch.Samples = append(ch.Samples, instBufs[slot][:remaining]...)
	}

	// Pad inactive channels with silence to keep alignment
	for _, ch := range mc.channels {
		if len(ch.Samples) < mc.totalSamples+remaining {
			pad := (mc.totalSamples + remaining) - len(ch.Samples)
			ch.Samples = append(ch.Samples, make([]float64, pad)...)
		}
	}

	// Append master buffer
	mc.masterCh.Samples = append(mc.masterCh.Samples, workBuf[:remaining]...)

	mc.totalSamples += remaining

	if mc.maxSamples > 0 && mc.totalSamples >= mc.maxSamples {
		mc.done = true
	}
}

// isDone returns true if the capture has reached its max duration.
func (mc *MultiChannelCapture) isDone() bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return mc.done
}

// snapshot returns a copy of all captured channels (thread-safe).
func (mc *MultiChannelCapture) snapshot() (map[string]*CaptureChannel, *CaptureChannel) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	channels := make(map[string]*CaptureChannel, len(mc.channels))
	for id, ch := range mc.channels {
		samples := make([]float64, len(ch.Samples))
		copy(samples, ch.Samples)
		channels[id] = &CaptureChannel{
			ID:      ch.ID,
			Name:    ch.Name,
			Samples: samples,
		}
	}

	masterSamples := make([]float64, len(mc.masterCh.Samples))
	copy(masterSamples, mc.masterCh.Samples)
	master := &CaptureChannel{
		ID:      mc.masterCh.ID,
		Name:    mc.masterCh.Name,
		Samples: masterSamples,
	}

	return channels, master
}

// AppendMIDIEvent records a MIDI event during capture (called from Schedule).
func (mc *MultiChannelCapture) AppendMIDIEvent(evt MIDIEvent) {
	mc.mu.Lock()
	mc.midiEvents = append(mc.midiEvents, evt)
	mc.mu.Unlock()
}

// InstrumentMeta holds metadata for an instrument being recorded.
type InstrumentMeta struct {
	ID   string
	Name string
}
