//go:build !test && !js

package audio

import (
	"sync"

	"github.com/ebitengine/oto/v3"
)

const blockSize = 64 // Process 64 samples at a time (~1.45ms at 44.1kHz)

// mixer mixes multiple voices into a single PCM stream.
// Uses 3-phase processing to avoid shared biquad state corruption:
//
//	Phase 1: Render voices + headroom → per-instrument buffers (no EQ)
//	Phase 2: Per-instrument channel EQ → masterBuf
//	Phase 3: Master channel EQ → workBuf
type mixer struct {
	mu     sync.Mutex
	voices []*voiceState
	pos    int
	player *oto.Player

	// Pre-allocated work buffers for block processing
	workBuf   []float64 // Final mixed output (after master EQ)
	voiceTemp []float64 // Single voice render buffer
	masterBuf []float64 // Pre-master-EQ accumulation buffer

	// Per-instrument accumulation buffers indexed by instrument slot.
	instBufs [][]float64

	// Instrument ID → stable slot index, populated at Schedule() time.
	instSlots   map[string]int
	instSlotIDs []string // reverse: slot → ID (for channel lookups)

	// Active instrument slots in current block (avoids iterating full list).
	activeSlots []int

	// Separate pending queue to reduce lock contention
	pendingMu  sync.Mutex
	pendingAdd []*voiceState
}

type voiceState struct {
	start int
	id    string
	slot  int // index into mixer.instBufs (pre-resolved at Schedule time)
	v     Voice
	ch    *Channel
}

func newMixer(c *oto.Context) *mixer {
	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		instSlots: make(map[string]int),
	}
	p := c.NewPlayer(m)
	p.SetBufferSize(bufferSizeBytes10ms)
	p.Play()
	m.player = p
	return m
}

// Schedule adds a voice to start after delaySamples have elapsed.
// Uses a separate pending queue to minimize contention with Read().
// Wraps the voice in antiPopVoice for click-free start/stop.
func (m *mixer) Schedule(id string, v Voice, delaySamples int) {
	// In test voice mode, replace all synth voices with simple sine
	if testVoiceMode {
		v = newTestSineVoice()
	}

	// Notify analyzer of trigger with raw buffer before any wrapping.
	if analyzerSvc != nil {
		if cv, ok := v.(*cVoice); ok {
			analyzerSvc.NotifyTrigger(id, cv.buf)
		}
	}

	// Wrap in anti-pop envelope for click-free fade-in/out.
	v = newAntiPopVoice(v, sampleRate)

	ch := channelForInstrument(id) // Lookup OUTSIDE any lock
	slot := m.instrumentSlot(id)
	vs := &voiceState{start: m.pos + delaySamples, id: id, slot: slot, v: v, ch: ch}

	m.pendingMu.Lock()
	m.pendingAdd = append(m.pendingAdd, vs)
	m.pendingMu.Unlock()
}

// instrumentSlot returns a stable integer index for the given instrument ID,
// allocating a new slot if this is the first time we see this instrument.
func (m *mixer) instrumentSlot(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx, ok := m.instSlots[id]; ok {
		return idx
	}
	if m.instSlots == nil {
		m.instSlots = make(map[string]int)
	}
	idx := len(m.instSlotIDs)
	m.instSlots[id] = idx
	m.instSlotIDs = append(m.instSlotIDs, id)
	m.instBufs = append(m.instBufs, make([]float64, blockSize))
	return idx
}

// testVoiceMode is defined in engine_stop.go
