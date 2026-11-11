//go:build !test && !js

package audio

import (
	"sync"

	"github.com/ebitengine/oto/v3"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
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
	postFXBuf []float64 // Per-instrument post-inserts/pre-EQ scratch (StageInsertFX tap)
	postEQBuf []float64 // Per-instrument post-EQ scratch (StageEQ tap)

	// Per-instrument accumulation buffers indexed by instrument slot.
	instBufs [][]float64

	// Instrument ID → stable slot index, populated at Schedule() time.
	instSlots   map[channel.ID]int
	instSlotIDs []channel.ID // reverse: slot → channel

	// Active instrument slots in current block (avoids iterating full list).
	activeSlots []int

	// Separate pending queue to reduce lock contention
	pendingMu  sync.Mutex
	pendingAdd []*voiceState

	// laneStops are StopLaneAt events, applied by processBlock when the block
	// containing atSample renders. Guarded by mu.
	laneStops []laneStopEvent

	// mixedChans are the channels mixed this block from voices; zeroBuf is the
	// silence a channel with processors and no voice runs on.
	mixedChans []*Channel
	zeroBuf    []float64

	// residency: release the stages of channels no row plays once they are
	// silent (residency.go). The live mixer does; offline renders never do —
	// a render is a readout of the whole sound, below the threshold included.
	// resState is this mixer's view of each channel (mixer goroutine only).
	residency bool
	resState  map[*Channel]*residencyState
}

// laneStopEvent: at mixer sample atSample, fade the lane voices of inst that
// started before it.
type laneStopEvent struct {
	inst     channel.ID
	atSample int
}

type voiceState struct {
	start    int
	id       channel.ID
	slot     int // index into mixer.instBufs (pre-resolved at Schedule time)
	v        Voice
	ch       *Channel
	row, abs int  // parity identity; (-1,-1) for non-sequencer sounds
	emitted  bool // guard so we publish exactly one report per voice
	// vid identifies an audio-lane voice (0 = preview / UI sound).
	vid uint64
	// laneCV is the raw synth voice of a lane voice whose analyzer / scope
	// trigger is deferred to its first rendered block (it may be committed
	// up to the horizon early). nil once notified, and for non-lane voices.
	laneCV *cVoice
}

func newMixer(c *oto.Context) *mixer {
	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
		residency: true,
	}
	p := c.NewPlayer(m)
	p.SetBufferSize(bufferSizeBytes10ms)
	p.Play()
	m.player = p
	return m
}

// unwrapCVoice traverses voice wrapper layers (scaledVoice, resampleVoice,
// gateVoice, antiPopVoice) to find the underlying *cVoice. Returns nil if no cVoice
// is found in the chain.
func unwrapCVoice(v Voice) *cVoice {
	for {
		switch w := v.(type) {
		case *cVoice:
			return w
		case *scaledVoice:
			v = w.v
		case *resampleVoice:
			v = w.src
		case *gateVoice:
			v = w.inner
		case *antiPopVoice:
			v = w.inner
		default:
			return nil
		}
	}
}

// Schedule adds a voice to start after delaySamples have elapsed.
// Uses a separate pending queue to minimize contention with Read().
// Wraps the voice in antiPopVoice for click-free start/stop.
func (m *mixer) Schedule(id channel.ID, v Voice, delaySamples, row, abs int) {
	m.ScheduleLane(id, v, delaySamples, row, abs, 0)
}

// ScheduleLane is Schedule for an audio-lane voice: vid > 0 makes it
// cancellable before it starts (cancelVoices / cancelAllLaneFuture) and
// defers its analyzer / scope trigger to its first rendered block.
func (m *mixer) ScheduleLane(id channel.ID, v Voice, delaySamples, row, abs int, vid uint64) {
	// In test voice mode, replace all synth voices with simple sine
	if env.TestVoice.Get() {
		v = newTestSineVoice()
	}

	// Unwrap voice wrappers (scaledVoice→resampleVoice→cVoice) to find the
	// underlying cVoice and its raw synth buffer.
	cv := unwrapCVoice(v)
	if vid != 0 {
		// Lane voice: notify when it starts rendering (see laneTriggerLoop).
		v = newAntiPopVoice(v, sampleRate)
		ch := channelForInstrument(id)
		slot := m.instrumentSlot(id)
		m.mu.Lock()
		start := m.pos + delaySamples
		m.mu.Unlock()
		vs := &voiceState{start: start, id: id, slot: slot, v: v, ch: ch, row: row, abs: abs, vid: vid, laneCV: cv}
		if vs.laneCV == nil {
			vs.laneCV = laneNoCV // still report the trigger (test hook / sine voices)
		}
		m.pendingMu.Lock()
		m.pendingAdd = append(m.pendingAdd, vs)
		m.pendingMu.Unlock()
		return
	}
	notifyVoiceTrigger(id, cv)
	// Wrap in anti-pop envelope for click-free fade-in/out.
	v = newAntiPopVoice(v, sampleRate)

	ch := channelForInstrument(id) // Lookup OUTSIDE any lock
	slot := m.instrumentSlot(id)
	m.mu.Lock()
	start := m.pos + delaySamples
	m.mu.Unlock()
	vs := &voiceState{start: start, id: id, slot: slot, v: v, ch: ch, row: row, abs: abs}

	m.pendingMu.Lock()
	m.pendingAdd = append(m.pendingAdd, vs)
	m.pendingMu.Unlock()
}

// notifyVoiceTrigger reports a voice's trigger to the analyzer and pushes its
// raw synth buffer to the scope/export services (A/B pipeline comparison).
func notifyVoiceTrigger(id channel.ID, cv *cVoice) {
	// Notify analyzer of trigger with raw buffer before any wrapping.
	if analyzerSvc != nil && cv != nil {
		analyzerSvc.NotifyTrigger(id, cv.buf)
	}

	// Push raw synth buffer to scope and export services for A/B pipeline comparison.
	// Per-trigger float32→float64 conversion uses a pooled buffer; both consumers
	// copy the contents into their own rings (scope.ringBuf.push appends), so the
	// slice is unreachable as soon as we putF64Buf below. Saves ~80 KB per trigger
	// — the dominant per-Schedule alloc when the synth tab is active.
	if cv != nil && cv.buf != nil && (scopeSvc != nil || exportSvc != nil) {
		f64 := getF64Buf(len(cv.buf))
		for i, s := range cv.buf {
			f64[i] = float64(s)
		}
		if scopeSvc != nil {
			scopeSvc.PushSamples(scope.StageSynth, id, f64)
		}
		if exportSvc != nil {
			exportSvc.PushSamples(scope.StageSynth, id, f64)
		}
		putF64Buf(f64)
	}

}

// instrumentSlot returns a stable integer index for the given instrument ID,
// allocating a new slot if this is the first time we see this instrument.
func (m *mixer) instrumentSlot(id channel.ID) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if idx, ok := m.instSlots[id]; ok {
		return idx
	}
	if m.instSlots == nil {
		m.instSlots = make(map[channel.ID]int)
	}
	idx := len(m.instSlotIDs)
	m.instSlots[id] = idx
	m.instSlotIDs = append(m.instSlotIDs, id)
	m.instBufs = append(m.instBufs, make([]float64, blockSize))
	// Register with the analyzer service so meter bridge can display this instrument.
	if analyzerSvc != nil {
		analyzerSvc.RegisterInstrument(idx, id, id.String())
	}
	return idx
}
