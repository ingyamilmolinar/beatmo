//go:build !test && !js

package audio

import (
	"sync"

	"github.com/ebitengine/oto/v3"
)

const blockSize = 64 // Process 64 samples at a time (~1.45ms at 44.1kHz)

// mixer mixes multiple voices into a single PCM stream.
type mixer struct {
	mu     sync.Mutex
	voices []*voiceState
	pos    int
	player *oto.Player

	// Pre-allocated work buffers for block processing
	workBuf   []float64 // Mixed output samples
	voiceTemp []float64 // Single voice block buffer

	// Separate pending queue to reduce lock contention
	pendingMu  sync.Mutex
	pendingAdd []*voiceState
}

type voiceState struct {
	start int
	id    string
	v     Voice
	ch    *Channel
}

func newMixer(c *oto.Context) *mixer {
	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
	}
	p := c.NewPlayer(m)
	p.SetBufferSize(bufferSizeBytes10ms)
	p.Play()
	m.player = p
	return m
}

// Schedule adds a voice to start after delaySamples have elapsed.
// Uses a separate pending queue to minimize contention with Read().
func (m *mixer) Schedule(id string, v Voice, delaySamples int) {
	ch := channelForInstrument(id) // Lookup OUTSIDE any lock
	vs := &voiceState{start: m.pos + delaySamples, id: id, v: v, ch: ch}

	m.pendingMu.Lock()
	m.pendingAdd = append(m.pendingAdd, vs)
	m.pendingMu.Unlock()
}

