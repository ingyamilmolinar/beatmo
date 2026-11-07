//go:build !test && !js

// A biquad is stateful and needs one continuous stream, so the desktop mixer runs in three phases: voices +
// headroom into per-instrument buffers (no EQ), each channel's inserts and EQ on its sum into the master, then the
// master EQ → soft clip → capture → int16. The TEST_* / BYPASS_* / DEBUG_* env vars isolate one stage each.
package audio

import (
	"math"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

var debugMixerCount int64
var testTonePhase float64 = 0

func init() {
	if env.TestTone.Get() {
		log.Warnf("[audio] debug mode BEATMO_TEST_TONE: outputting 440Hz test tone (bypassing all synth)")
	}
	if env.TestVoice.Get() {
		log.Warnf("[audio] debug mode BEATMO_TEST_VOICE: replacing all synth with simple sine voices")
	}
	if env.BypassChannelProc.Get() {
		log.Warnf("[audio] debug mode BYPASS_CHANNEL_PROC: skipping all channel processing (volume, EQ)")
	}
	if env.AudioClipDebug.Get() {
		log.Warnf("[audio] debug mode AUDIO_CLIP_DEBUG: logging clipping events")
	}
	if env.DebugMixer.Get() {
		log.Warnf("[audio] debug mode DEBUG_MIXER: logging mixer workBuf values")
	}
	if env.BypassHeadroom.Get() {
		log.Warnf("[audio] debug mode BYPASS_HEADROOM: skipping 0.25 headroom attenuation")
	}
	if env.TestRawVoice.Get() {
		log.Warnf("[audio] debug mode BEATMO_TEST_RAW_VOICE: outputting first voice raw samples (no processing)")
	}
	if env.TestRawVoiceFull.Get() {
		log.Warnf("[audio] debug mode BEATMO_TEST_RAW_VOICE_FULL: outputting first voice at FULL amplitude")
	}
	if env.TestRawVoiceNoScale.Get() {
		log.Warnf("[audio] debug mode BEATMO_TEST_RAW_VOICE_NOSCALE: outputting first voice with NO scaling")
	}
	if env.SingleVoice.Get() {
		log.Warnf("[audio] debug mode BEATMO_SINGLE_VOICE: limiting mixer to ONE voice at a time")
	}
}

// A pre-rendered float32 buffer, like cVoice, to match the C synth path.
type testSineVoice struct {
	buf []float32
	pos int
}

func newTestSineVoice() *testSineVoice {
	sr := sampleRate
	duration := sr / 4 // 0.25 seconds
	buf := make([]float32, duration)

	freq := 220.0
	amp := float32(0.5)

	for i := 0; i < duration; i++ {
		phase := 2 * math.Pi * freq * float64(i) / float64(sr)
		sample := amp * float32(math.Sin(phase))

		// Apply simple envelope
		env := float32(1.0)
		if i < 100 {
			env = float32(i) / 100.0 // attack
		} else if i > duration-500 {
			env = float32(duration-i) / 500.0 // release
		}
		buf[i] = sample * env
	}

	return &testSineVoice{buf: buf, pos: 0}
}

func (v *testSineVoice) Sample() (float64, bool) {
	if v.pos >= len(v.buf) {
		return 0, true
	}
	f := float64(v.buf[v.pos])
	v.pos++
	return f, false
}

// SampleBlock implements BlockVoice for testSineVoice (bulk float32→float64).
func (v *testSineVoice) SampleBlock(dst []float64) (int, bool) {
	remaining := len(v.buf) - v.pos
	if remaining <= 0 {
		return 0, true
	}
	n := len(dst)
	if n > remaining {
		n = remaining
	}
	for j := 0; j < n; j++ {
		dst[j] = float64(v.buf[v.pos+j])
	}
	v.pos += n
	return n, v.pos >= len(v.buf)
}

var clipSampleCount atomic.Int64
var clipBlockCount atomic.Int64
var totalBlockCount atomic.Int64

// Output capture for comparing Go vs JS audio output
var (
	outputCaptureEnabled bool
	outputCaptureBuf     []float64
	outputCaptureMu      sync.Mutex
)

func StartOutputCapture() {
	outputCaptureMu.Lock()
	outputCaptureBuf = outputCaptureBuf[:0]
	outputCaptureEnabled = true
	outputCaptureMu.Unlock()
	log.Debugf("[capture] started capturing output")
}

// A copy, post-soft-clip and pre-int16.
func StopOutputCapture() []float64 {
	outputCaptureMu.Lock()
	outputCaptureEnabled = false
	result := make([]float64, len(outputCaptureBuf))
	copy(result, outputCaptureBuf)
	outputCaptureMu.Unlock()
	log.Debugf("[capture] stopped: captured %d samples (%.3f seconds)",
		len(result), float64(len(result))/float64(sampleRate))
	return result
}

// GetOutputCapture returns the current capture buffer without stopping.
func GetOutputCapture() []float64 {
	outputCaptureMu.Lock()
	result := make([]float64, len(outputCaptureBuf))
	copy(result, outputCaptureBuf)
	outputCaptureMu.Unlock()
	return result
}

// ClearOutputCapture clears the capture buffer without stopping capture.
func ClearOutputCapture() {
	outputCaptureMu.Lock()
	outputCaptureBuf = outputCaptureBuf[:0]
	outputCaptureMu.Unlock()
}

// IsOutputCaptureEnabled returns whether output capture is currently enabled.
func IsOutputCaptureEnabled() bool {
	outputCaptureMu.Lock()
	defer outputCaptureMu.Unlock()
	return outputCaptureEnabled
}

// OutputCaptureSampleRate returns the sample rate used for capture (same as audio output).
func OutputCaptureSampleRate() int {
	return sampleRate
}

// EQ filters need it for correct frequencies.
func SampleRate() int {
	return sampleRate
}

func (m *mixer) Stop(id channel.ID) {
	m.mu.Lock()
	for i := 0; i < len(m.voices); i++ {
		if m.voices[i].id == id {
			// A graceful fade: the voice reports done after it, and processBlock removes it.
			if ap, ok := m.voices[i].v.(*antiPopVoice); ok {
				ap.RequestStop()
			} else {
				// Non-wrapped voice: remove immediately (legacy path).
				m.voices[i] = m.voices[len(m.voices)-1]
				m.voices = m.voices[:len(m.voices)-1]
				i--
			}
		}
	}
	m.mu.Unlock()
}

// Block-based to keep lock overhead low.
func (m *mixer) Read(p []byte) (int, error) {
	samples := len(p) / 2

	// Drain pending voices with quick lock
	m.pendingMu.Lock()
	if len(m.pendingAdd) > 0 {
		m.mu.Lock()
		m.voices = append(m.voices, m.pendingAdd...)
		m.mu.Unlock()
		m.pendingAdd = m.pendingAdd[:0]
	}
	m.pendingMu.Unlock()

	// Process in blocks
	offset := 0
	for offset < samples {
		blockLen := blockSize
		if offset+blockLen > samples {
			blockLen = samples - offset
		}

		// Lock ONCE for entire block
		m.mu.Lock()
		m.processBlock(offset, blockLen, p)
		m.mu.Unlock()

		offset += blockLen
	}
	return len(p), nil
}

// TEST_TONE=1: a pure 440 Hz sine bypassing all synthesis, kept out of the hot kernel. m.workBuf must already be
// allocated.
func (m *mixer) processTestTone(offset, blockLen int, p []byte) {
	sr := float64(sampleRate)
	freq := 440.0
	amp := 0.2 // -14dB to avoid clipping
	for i := 0; i < blockLen; i++ {
		m.workBuf[i] = amp * math.Sin(2*math.Pi*freq*testTonePhase/sr)
		testTonePhase++
		if testTonePhase >= sr {
			testTonePhase -= sr
		}
	}
	// Skip to int16 conversion (no headroom needed for test tone)
	for i := 0; i < blockLen; i++ {
		sum := m.workBuf[i]
		if sum > 1 {
			sum = 1
		} else if sum < -1 {
			sum = -1
		}
		v := int16(math.Round(sum * 32767))
		outIdx := (offset + i) * 2
		p[outIdx] = byte(v)
		p[outIdx+1] = byte(v >> 8)
	}
	m.pos += blockLen
	updateDeviceClock(int64(m.pos))
}

// TEST_RAW_VOICE=1: voice 0 with no mixing, EQ or headroom, isolating the voice buffer. len(m.voices) > 0.
func (m *mixer) processRawVoice(offset, blockLen int, p []byte) {
	vs := m.voices[0]
	startPos := m.pos + offset
	if startPos >= vs.start {
		for i := 0; i < blockLen; i++ {
			val, done := vs.v.Sample()
			if done {
				// Voice finished, output silence for rest
				for j := i; j < blockLen; j++ {
					outIdx := (offset + j) * 2
					p[outIdx] = 0
					p[outIdx+1] = 0
				}
				// Remove voice
				m.voices = m.voices[1:]
				break
			}
			// Scale by 0.2 to match TEST_TONE amplitude (or full amp/no scale)
			var sum float64
			if env.TestRawVoiceNoScale.Get() {
				// Direct value, no multiplication at all
				sum = val
			} else {
				scale := 0.2
				if env.TestRawVoiceFull.Get() {
					scale = 1.0
				}
				sum = val * scale
			}

			// Debug: log sample values periodically
			if env.DebugMixer.Get() && i == 0 && debugMixerCount%100 == 0 {
				log.Tracef("[raw-voice] val=%.6f sum=%.6f noscale=%v", val, sum, env.TestRawVoiceNoScale.Get())
			}
			if sum > 1 {
				sum = 1
			} else if sum < -1 {
				sum = -1
			}
			v := int16(math.Round(sum * 32767))
			outIdx := (offset + i) * 2
			p[outIdx] = byte(v)
			p[outIdx+1] = byte(v >> 8)
		}
	} else {
		// Voice hasn't started yet, output silence
		for i := 0; i < blockLen; i++ {
			outIdx := (offset + i) * 2
			p[outIdx] = 0
			p[outIdx+1] = 0
		}
	}
	m.pos += blockLen
	updateDeviceClock(int64(m.pos))
}

// Caller holds m.mu: it mutates voices, laneStops and the resampler state.
func (m *mixer) processBlock(offset, blockLen int, p []byte) {
	// Each buffer separately, for test fixtures that build mixer{} directly with only some allocated.
	if m.workBuf == nil {
		m.workBuf = make([]float64, blockSize)
	}
	if m.voiceTemp == nil {
		m.voiceTemp = make([]float64, blockSize)
	}
	if m.masterBuf == nil {
		m.masterBuf = make([]float64, blockSize)
	}
	if m.postFXBuf == nil {
		m.postFXBuf = make([]float64, blockSize)
	}
	if m.postEQBuf == nil {
		m.postEQBuf = make([]float64, blockSize)
	}
	if m.instSlots == nil {
		m.instSlots = make(map[channel.ID]int)
	}

	// TEST_TONE mode: output pure 440Hz sine wave, bypassing all synth.
	if env.TestTone.Get() {
		m.processTestTone(offset, blockLen, p)
		return
	}

	if env.TestRawVoice.Get() && len(m.voices) > 0 {
		m.processRawVoice(offset, blockLen, p)
		return
	}

	// Zero output buffers
	for i := 0; i < blockLen; i++ {
		m.workBuf[i] = 0
		m.masterBuf[i] = 0
	}

	// Reset active instrument tracking
	m.activeSlots = m.activeSlots[:0]

	// Audio-lane StopLaneAt events due within this block.
	m.applyLaneStops(m.pos + offset + blockLen)

	// === PHASE 1: Render voices + headroom → per-instrument buffers (NO EQ) ===
	i := 0
	voicesProcessed := 0
	for i < len(m.voices) {
		// SINGLE_VOICE mode: only process one voice per block
		if env.SingleVoice.Get() && voicesProcessed >= 1 {
			break
		}

		vs := m.voices[i]
		blockStart := m.pos + offset

		// A voice starts on its own sample, inside the block if that is where it falls.
		if vs.start >= blockStart+blockLen {
			i++
			continue
		}
		startOff := max(0, vs.start-blockStart)

		if !vs.emitted && vs.row >= 0 {
			publishEmitReport(vs.row, vs.abs, int64(blockStart+startOff))
			vs.emitted = true
		}
		if vs.laneCV != nil {
			m.laneStarted(vs)
		}

		done := m.renderVoiceIntoInstBuf(vs, startOff, blockLen)
		voicesProcessed++
		if done {
			// Swap with last and truncate (no allocation)
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices = m.voices[:len(m.voices)-1]
			// Don't increment i - check swapped element
		} else {
			i++
		}
	}

	// Push per-instrument buffers to analyzer (memcopy only, no computation).
	if analyzerSvc != nil {
		for _, slot := range m.activeSlots {
			analyzerSvc.PushInstBuf(slot, m.instBufs[slot][:blockLen])
		}
	}

	// Scope tap: post-headroom per-instrument buffers (before EQ/inserts).
	if scopeSvc != nil {
		for _, slot := range m.activeSlots {
			id := m.instSlotIDs[slot]
			scopeSvc.PushSamples(scope.StageAntiPop, id, m.instBufs[slot][:blockLen])
		}
	}
	if exportSvc != nil {
		for _, slot := range m.activeSlots {
			exportSvc.PushSamples(scope.StageAntiPop, m.instSlotIDs[slot], m.instBufs[slot][:blockLen])
		}
	}

	// Two sub-passes so the scope can compare between insert FX and EQ: inserts → postFXBuf (StageInsertFX), then EQ →
	// postEQBuf (StageEQ).
	if !env.BypassChannelProc.Get() {
		at := m.pos + offset
		m.mixedChans = m.mixedChans[:0]
		for _, slot := range m.activeSlots {
			id := m.instSlotIDs[slot]
			ch := channelForInstrument(id)
			if m.residency {
				m.residencyOf(ch).released = false // a voice wakes it
			}
			m.mixedChans = append(m.mixedChans, ch)
			m.mixChannel(id, ch, m.instBufs[slot][:blockLen], at)
		}
		// A channel with an effect or EQ runs every block, voice or not, so tails ring out and LFO phase advances exactly as
		// in the browser's worklet, which processes every quantum.
		if pcs := chanMgr.procChannels.Load(); pcs != nil {
			for _, ch := range *pcs {
				if slices.Contains(m.mixedChans, ch) {
					continue
				}
				if m.residency && m.residencyOf(ch).released {
					if !ChannelHeld(ch.id) {
						continue
					}
					m.residencyOf(ch).released = false // held again
				}
				if len(m.zeroBuf) < blockLen {
					m.zeroBuf = make([]float64, blockSize)
				}
				m.mixChannel(ch.id, ch, m.zeroBuf[:blockLen], at)
				if m.residency {
					m.releaseIfSilent(ch)
				}
			}
		}
	} else {
		// Bypass: direct sum from instrument buffers
		for _, slot := range m.activeSlots {
			buf := m.instBufs[slot]
			for j := 0; j < blockLen; j++ {
				m.masterBuf[j] += buf[j]
			}
		}
	}

	// === PHASE 3: Master channel processing → workBuf ===
	mainCh := chanMgr.main
	if !env.BypassChannelProc.Get() {
		mainCh.ProcessBlockLocal(m.masterBuf[:blockLen], m.workBuf[:blockLen])
	} else {
		for j := 0; j < blockLen; j++ {
			m.workBuf[j] += m.masterBuf[j]
		}
	}

	// Push master output to analyzer (memcopy only, no computation).
	if analyzerSvc != nil {
		analyzerSvc.PushMasterBuf(m.workBuf[:blockLen])
	}
	// The Levels tab's LUFS-S follows the master.
	FeedMasterLUFS(m.workBuf[:blockLen])

	// Scope tap: the master after its EQ, before the soft clip.
	if scopeSvc != nil {
		scopeSvc.PushSamples(scope.StageMaster, channel.Master, m.workBuf[:blockLen])
	}
	if exportSvc != nil {
		exportSvc.PushSamples(scope.StageMaster, channel.Master, m.workBuf[:blockLen])
	}

	// Debug: log mixer workBuf stats
	if env.DebugMixer.Get() && debugMixerCount%1000 == 0 {
		var minVal, maxVal float64
		var nonZero int
		for i := 0; i < blockLen; i++ {
			if m.workBuf[i] != 0 {
				nonZero++
			}
			if m.workBuf[i] < minVal {
				minVal = m.workBuf[i]
			}
			if m.workBuf[i] > maxVal {
				maxVal = m.workBuf[i]
			}
		}
		if nonZero > 0 {
			log.Tracef("[mixer] voices=%d insts=%d nonZero=%d min=%.6f max=%.6f",
				len(m.voices), len(m.activeSlots), nonZero, minVal, maxVal)
		}
	}
	debugMixerCount++

	// Diagnostic: count clipping events before hard clamp
	var clippedInBlock int
	if env.AudioClipDebug.Get() {
		for i := 0; i < blockLen; i++ {
			if m.workBuf[i] > 1 || m.workBuf[i] < -1 {
				clippedInBlock++
			}
		}
		if clippedInBlock > 0 {
			clipSampleCount.Add(int64(clippedInBlock))
			clipBlockCount.Add(1)
		}
		totalBlockCount.Add(1)
		// Log periodically (every 1000 blocks with clipping, ~10 seconds of audio)
		if clippedInBlock > 0 && clipBlockCount.Load()%1000 == 0 {
			log.Warnf("[audio] clipping stats: %d samples clipped in %d blocks (total blocks: %d)",
				clipSampleCount.Load(), clipBlockCount.Load(), totalBlockCount.Load())
		}
	}

	// The same C as the browser's master node.
	softClipBlock(m.workBuf[:blockLen])

	// What the device receives, after the soft clip; the browser's capture taps the same point.
	if outputCaptureEnabled {
		outputCaptureMu.Lock()
		outputCaptureBuf = append(outputCaptureBuf, m.workBuf[:blockLen]...)
		outputCaptureMu.Unlock()
	}

	// Per-instrument and master as the device receives them, written by background workers; zero-cost when not
	// recording, and Tap never blocks or allocates.
	if pipe := pipelinePtr.Load(); pipe != nil {
		pipe.Tap(m.instSlotIDs, m.activeSlots, m.instBufs, m.workBuf[:], blockLen)
	}

	// Rounded, not truncated, to reduce quantisation noise; the soft clip never leaves [-1, 1].
	for i := 0; i < blockLen; i++ {
		sum := m.workBuf[i]
		if sum > 1 {
			sum = 1
		} else if sum < -1 {
			sum = -1
		}
		v := int16(math.Round(sum * 32767))
		outIdx := (offset + i) * 2
		p[outIdx] = byte(v)
		p[outIdx+1] = byte(v >> 8)
	}

	m.pos += blockLen
	updateDeviceClock(int64(m.pos))
}

// in is its voices, or silence; each stage is tapped.
func (m *mixer) mixChannel(id channel.ID, ch *Channel, in []float64, at int) {
	n := len(in)
	for k := 0; k < n; k++ {
		m.postFXBuf[k] = 0
	}
	ch.ProcessInsertsBlockLocal(in, m.postFXBuf[:n])
	// Tap the post-inserts / pre-EQ per-instrument signal.
	if scopeSvc != nil {
		scopeSvc.PushSamples(scope.StageInsertFX, id, m.postFXBuf[:n])
	}
	if exportSvc != nil {
		exportSvc.PushSamples(scope.StageInsertFX, id, m.postFXBuf[:n])
	}
	for k := 0; k < n; k++ {
		m.postEQBuf[k] = 0
	}
	ch.ProcessEQBlockLocal(m.postFXBuf[:n], m.postEQBuf[:n])
	if m.residency {
		m.residencyOf(ch).trackQuiet(in, m.postEQBuf[:n])
	}
	// Tap the post-EQ per-instrument signal.
	if scopeSvc != nil {
		scopeSvc.PushSamples(scope.StageEQ, id, m.postEQBuf[:n])
	}
	if exportSvc != nil {
		exportSvc.PushSamples(scope.StageEQ, id, m.postEQBuf[:n])
	}
	if stageCaptureOn.Load() {
		stageCaptureBlock(id, at, in, m.postFXBuf[:n], m.postEQBuf[:n])
	}
	for k := 0; k < n; k++ {
		m.masterBuf[k] += m.postEQBuf[k]
	}
}

// debugVoiceCount tracks voice sample logging frequency
var debugVoiceCount int64

// Applied per voice BEFORE accumulation, so a dense mix stays under the soft-clip knee.
const mixHeadroom = VoiceHeadroom

// Zeroed on first use within the block.
func (m *mixer) ensureInstBuf(slot, blockLen int) []float64 {
	// Grow instBufs if a slot was allocated after mixer init.
	for slot >= len(m.instBufs) {
		m.instBufs = append(m.instBufs, make([]float64, blockSize))
	}
	buf := m.instBufs[slot]
	if len(buf) < blockLen {
		buf = make([]float64, blockSize)
		m.instBufs[slot] = buf
	}
	for _, a := range m.activeSlots {
		if a == slot {
			return buf // already tracked, buffer already zeroed
		}
	}
	// First voice for this instrument in this block — zero the buffer
	for j := 0; j < blockLen; j++ {
		buf[j] = 0
	}
	m.activeSlots = append(m.activeSlots, slot)
	return buf
}

// Frames [off, blockLen), headroom applied, no channel processing. Returns true when done. Caller holds m.mu.
func (m *mixer) renderVoiceIntoInstBuf(vs *voiceState, off, blockLen int) bool {
	instBuf := m.ensureInstBuf(vs.slot, blockLen)[off:blockLen]
	want := blockLen - off

	var rendered int
	var done bool

	// Try BlockVoice bulk path first.
	if bv, ok := vs.v.(BlockVoice); ok {
		rendered, done = bv.SampleBlock(m.voiceTemp[:want])
	} else {
		// Fallback: sample-by-sample rendering.
		for i := 0; i < want; i++ {
			val, d := vs.v.Sample()
			if d {
				rendered = i
				done = true
				break
			}
			m.voiceTemp[i] = val
		}
		if !done {
			rendered = want
		}
	}

	if rendered > 0 {
		// Debug: log voice sample stats
		if env.DebugMixer.Get() && debugVoiceCount%500 == 0 {
			var minVal, maxVal float64
			for i := 0; i < rendered; i++ {
				if m.voiceTemp[i] < minVal {
					minVal = m.voiceTemp[i]
				}
				if m.voiceTemp[i] > maxVal {
					maxVal = m.voiceTemp[i]
				}
			}
			log.Tracef("[voice] %s min=%.6f max=%.6f bypass=%v block=%v",
				vs.id, minVal, maxVal, env.BypassChannelProc.Get(),
				func() bool { _, ok := vs.v.(BlockVoice); return ok }())
		}
		debugVoiceCount++

		// Apply per-voice headroom BEFORE accumulation to prevent clipping.
		if !env.BypassHeadroom.Get() {
			for i := 0; i < rendered; i++ {
				m.voiceTemp[i] *= mixHeadroom
			}
		}

		// Accumulate into per-instrument buffer (no channel processing here)
		for i := 0; i < rendered; i++ {
			instBuf[i] += m.voiceTemp[i]
		}
	}

	return done
}
