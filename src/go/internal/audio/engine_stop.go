//go:build !test && !js

// ─── 3-Phase Desktop Audio Mixer ────────────────────────────────────────────
//
// The desktop mixer uses a 3-phase block processing design to prevent shared
// biquad EQ state corruption.
//
//	Phase 1: Render voices → per-instrument buffers (NO EQ)
//	         Each voice: voice.Sample() → voiceTemp, apply mixHeadroom (0.18×),
//	         accumulate into instBufs[instrumentID].
//
//	Phase 2: Per-instrument channel EQ → masterBuf
//	         For each active instrument: channel.ProcessBlockLocal(instBuf, masterBuf)
//	         (applies volume + EQ; does NOT recurse to parent channel).
//
//	Phase 3: Master channel EQ → workBuf → soft clip → output capture → int16 (×32767)
//
// WHY 3 PHASES: each biquad filter is stateful (x1, x2, y1, y2) and expects a
// continuous signal stream. The old code processed individual voices through
// shared biquad chains, corrupting filter state and causing audible distortion
// (clicks, noise). The fix sums all voices per-instrument FIRST, then routes
// the coherent sum through EQ. WASM was never affected because WebAudio sums
// voices at the GainNode level before routing through filter nodes.
//
// Debugging workflow (each env var isolates a pipeline stage):
//
//	TEST_TONE=1           → pure 440Hz sine, bypasses all synth (tests Oto/driver)
//	TEST_VOICE=1          → simple 220Hz sine per hit (tests voice/mixer path)
//	BYPASS_CHANNEL_PROC=1 → skip channel processing (isolates mixer vs EQ)
//	AUDIO_CLIP_DEBUG=1    → log clipping events before hard clamp
//	DEBUG_MIXER=1         → log workBuf min/max values periodically
//	BYPASS_HEADROOM=1     → skip 0.25× attenuation (test headroom sufficiency)
//	TEST_RAW_VOICE=1      → output first voice raw (test voice buffer integrity)
//	SINGLE_VOICE=1        → limit to one voice (test multi-voice accumulation)
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

// testSineVoice is a simple 220Hz sine wave voice for debugging.
// Uses a pre-rendered float32 buffer like cVoice to match the C synth path.
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

// StartOutputCapture begins capturing the final mixed audio output.
// Call StopOutputCapture to retrieve the captured samples.
func StartOutputCapture() {
	outputCaptureMu.Lock()
	outputCaptureBuf = outputCaptureBuf[:0]
	outputCaptureEnabled = true
	outputCaptureMu.Unlock()
	log.Debugf("[capture] started capturing output")
}

// StopOutputCapture stops capturing and returns the captured samples.
// Returns a copy of the captured float64 samples (post-headroom, pre-int16 conversion).
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

// SampleRate returns the audio output sample rate for the current platform.
// Use this when configuring EQ filters to ensure correct filter frequencies.
func SampleRate() int {
	return sampleRate
}

func (m *mixer) Stop(id channel.ID) {
	m.mu.Lock()
	for i := 0; i < len(m.voices); i++ {
		if m.voices[i].id == id {
			// Request graceful fade-out instead of immediate removal.
			// The voice will report done=true after the fade completes,
			// and the mixer will naturally remove it in processBlock.
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

// Read implements io.Reader for oto.Player.
// Uses block-based processing to minimize lock overhead.
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

// processBlock processes blockLen samples starting at offset.
// Uses 3-phase processing to avoid shared biquad state corruption:
//
//	Phase 1: Render each voice + headroom → per-instrument buffer (no EQ)
//	Phase 2: Per-instrument channel EQ → masterBuf
//	Phase 3: Master channel EQ → workBuf
//
// This ensures each biquad filter chain receives a coherent, continuous signal
// stream instead of interleaved audio from unrelated voices, which was the root
// cause of desktop audio distortion.
//
// Caller must hold m.mu.
// processTestTone renders a pure 440 Hz sine (TEST_TONE=1), bypassing all synth
// and processing. Extracted verbatim from processBlock's early-return diagnostic
// path so the hot 3-phase kernel isn't cluttered by a mode that never runs in
// production; behaviour is unchanged. Assumes m.workBuf is already allocated
// (processBlock's lazy-init runs before the guard that calls this).
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

// processRawVoice outputs voice[0]'s raw samples with no mixing/EQ/headroom
// (TEST_RAW_VOICE=1), to isolate voice-buffer distortion. Extracted verbatim
// from processBlock's early-return diagnostic path; behaviour is unchanged.
// Caller guarantees len(m.voices) > 0.
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

func (m *mixer) processBlock(offset, blockLen int, p []byte) {
	// Lazily initialize work buffers (needed for tests that create mixer{} directly).
	// Each buffer is checked individually so older test fixtures that pre-allocate
	// some but not all buffers still get the missing ones filled in.
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

	// TEST_RAW_VOICE mode: output raw voice samples with no processing —
	// isolates whether the voice buffer itself causes distortion.
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

		// A voice starts on its own sample, inside the block if that is where
		// it falls (it used to wait for the next block: up to 63 frames late).
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

	// === PHASE 2: Per-instrument channel processing → masterBuf ===
	//
	// Split into two sub-passes so the scope panel can compare the signal
	// between insert FX and EQ (scope.StageInsertFX vs scope.StageEQ).
	//   Phase 2a: instBuf → ProcessInsertsBlockLocal → postFXBuf (tap StageInsertFX)
	//   Phase 2b: postFXBuf → ProcessEQBlockLocal → postEQBuf (tap StageEQ)
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
		// A channel with an effect or an EQ runs every block, voice or not —
		// silence in — so a delay's echoes, a reverb's tail and an EQ's ring
		// play out, and an LFO's phase advances, exactly as in the browser,
		// whose worklet processes every render quantum. (It used to run only
		// while one of its voices played: tails stopped dead at the voice's
		// end and resumed, stale, with the next note.)
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
	// Phase 2: feed the same block through the K-weighted LUFS
	// integrator so the Levels tab's LUFS-S readout follows the master.
	FeedMasterLUFS(m.workBuf[:blockLen])

	// Scope tap: final master output (after master EQ/compressor).
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

	// Master soft clip (mix_dsp.c — the browser's master node runs the same
	// C): identity below the knee, a smooth approach to full scale above.
	softClipBlock(m.workBuf[:blockLen])

	// Capture what the device receives (after the soft clip, before int16):
	// the browser's output capture taps the same point.
	if outputCaptureEnabled {
		outputCaptureMu.Lock()
		outputCaptureBuf = append(outputCaptureBuf, m.workBuf[:blockLen]...)
		outputCaptureMu.Unlock()
	}

	// Recording pipeline tap: stream per-instrument + the master as the device
	// receives it (post soft clip, like the browser's master tap) to disk on
	// background workers. The atomic pointer check is zero-cost when not
	// recording; Tap itself is non-blocking and allocation-free.
	if pipe := pipelinePtr.Load(); pipe != nil {
		pipe.Tap(m.instSlotIDs, m.activeSlots, m.instBufs, m.workBuf, blockLen)
	}

	// Convert to int16. Use math.Round instead of truncation to reduce
	// quantization noise; the soft clip never leaves [-1, 1].
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

// mixChannel runs one instrument channel's inserts and EQ over in (its voices,
// or silence) and adds the result to the master bus, tapping each stage.
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

// mixHeadroom is applied per-voice BEFORE accumulation to prevent clipping.
// -14.9dB headroom (0.18) provides ~3dB more margin than the previous 0.25
// for dense mixes (7+ instruments) while the master compressor normalizes.
// Value lives in chain_spec.go (VoiceHeadroom) as the single source of truth
// shared with the cross-platform parity surface.
const mixHeadroom = VoiceHeadroom

// ensureInstBuf returns the per-instrument buffer for the given slot, zeroing
// it on first use within the current block. Uses O(1) slice indexing.
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

// renderVoiceIntoInstBuf renders the voice into frames [off, blockLen) of
// the block, applies headroom, and accumulates into the per-instrument buffer.
// Does NOT apply any channel/EQ processing — that happens in phase 2/3.
// Uses BlockVoice bulk path when available for reduced function-call overhead.
// Returns true if the voice is done and should be removed.
// Caller must hold m.mu.
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
