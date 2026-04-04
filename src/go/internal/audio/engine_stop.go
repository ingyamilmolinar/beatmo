//go:build !test && !js

// ─── 3-Phase Desktop Audio Mixer ────────────────────────────────────────────
//
// The desktop mixer uses a 3-phase block processing design to prevent shared
// biquad EQ state corruption.
//
//	Phase 1: Render voices → per-instrument buffers (NO EQ)
//	         Each voice: voice.Sample() → voiceTemp, apply mixHeadroom (0.25×),
//	         accumulate into instBufs[instrumentID].
//
//	Phase 2: Per-instrument channel EQ → masterBuf
//	         For each active instrument: channel.ProcessBlockLocal(instBuf, masterBuf)
//	         (applies volume + EQ; does NOT recurse to parent channel).
//
//	Phase 2.5: Send effects (delay + reverb) → masterBuf
//
//	Phase 3: Master channel EQ → workBuf → output capture → hard clamp [-1,1] → int16 (×32767)
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
	"log"
	"math"
	"os"
	"sync"
	"sync/atomic"
)

// Audio clipping diagnostics - enable with AUDIO_CLIP_DEBUG=1
var audioClipDebug = os.Getenv("AUDIO_CLIP_DEBUG") == "1"

// bypassChannelProc skips all channel processing (volume, EQ) when set via BYPASS_CHANNEL_PROC=1.
// Use this to isolate whether distortion comes from channel processing vs mixer/output.
var bypassChannelProc = os.Getenv("BYPASS_CHANNEL_PROC") == "1"

// debugMixer logs mixer workBuf values to help diagnose distortion.
var debugMixer = os.Getenv("DEBUG_MIXER") == "1"
var debugMixerCount int64

// bypassHeadroom skips the 0.25 headroom attenuation when set via BYPASS_HEADROOM=1.
var bypassHeadroom = os.Getenv("BYPASS_HEADROOM") == "1"

// rawVoiceMode outputs only the first voice's raw samples, bypassing all processing.
// Use TEST_RAW_VOICE=1 to test if voice buffers are clean before any mixer processing.
var rawVoiceMode = os.Getenv("TEST_RAW_VOICE") == "1"

// singleVoiceMode limits mixer to ONE voice at a time, using normal mixer path.
// Use SINGLE_VOICE=1 to test if multi-voice accumulation is causing distortion.
var singleVoiceMode = os.Getenv("SINGLE_VOICE") == "1"

// rawVoiceFullAmp outputs first voice at full amplitude (no 0.2 scaling).
// Use TEST_RAW_VOICE_FULL=1 to test if full amplitude causes issues.
var rawVoiceFullAmp = os.Getenv("TEST_RAW_VOICE_FULL") == "1"

// rawVoiceNoScale outputs first voice with NO multiplication at all (direct val to int16).
// Use TEST_RAW_VOICE_NOSCALE=1 to test if multiplication is causing issues.
var rawVoiceNoScale = os.Getenv("TEST_RAW_VOICE_NOSCALE") == "1"

// testToneMode outputs a pure 440Hz sine wave instead of normal audio when set via TEST_TONE=1.
// Use this to test if Oto/driver is working correctly.
var testToneMode = os.Getenv("TEST_TONE") == "1"
var testTonePhase float64 = 0

// testVoiceMode replaces all synth voices with a simple sine wave via TEST_VOICE=1.
// Use this to test if the voice/mixer path is working correctly.
var testVoiceMode = os.Getenv("TEST_VOICE") == "1"

func init() {
	if testToneMode {
		log.Println("[AUDIO DEBUG] TEST_TONE=1 - Outputting 440Hz test tone (bypassing all synth)")
	}
	if testVoiceMode {
		log.Println("[AUDIO DEBUG] TEST_VOICE=1 - Replacing all synth with simple sine voices")
	}
	if bypassChannelProc {
		log.Println("[AUDIO DEBUG] BYPASS_CHANNEL_PROC=1 - Skipping all channel processing (volume, EQ)")
	}
	if audioClipDebug {
		log.Println("[AUDIO DEBUG] AUDIO_CLIP_DEBUG=1 - Logging clipping events")
	}
	if debugMixer {
		log.Println("[AUDIO DEBUG] DEBUG_MIXER=1 - Logging mixer workBuf values")
	}
	if bypassHeadroom {
		log.Println("[AUDIO DEBUG] BYPASS_HEADROOM=1 - Skipping 0.25 headroom attenuation")
	}
	if rawVoiceMode {
		log.Println("[AUDIO DEBUG] TEST_RAW_VOICE=1 - Outputting first voice raw samples (no processing)")
	}
	if rawVoiceFullAmp {
		log.Println("[AUDIO DEBUG] TEST_RAW_VOICE_FULL=1 - Outputting first voice at FULL amplitude")
	}
	if rawVoiceNoScale {
		log.Println("[AUDIO DEBUG] TEST_RAW_VOICE_NOSCALE=1 - Outputting first voice with NO scaling")
	}
	if singleVoiceMode {
		log.Println("[AUDIO DEBUG] SINGLE_VOICE=1 - Limiting mixer to ONE voice at a time")
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
	log.Println("[AUDIO CAPTURE] Started capturing output")
}

// StopOutputCapture stops capturing and returns the captured samples.
// Returns a copy of the captured float64 samples (post-headroom, pre-int16 conversion).
func StopOutputCapture() []float64 {
	outputCaptureMu.Lock()
	outputCaptureEnabled = false
	result := make([]float64, len(outputCaptureBuf))
	copy(result, outputCaptureBuf)
	outputCaptureMu.Unlock()
	log.Printf("[AUDIO CAPTURE] Stopped. Captured %d samples (%.3f seconds)",
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

func (m *mixer) Stop(id string) {
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
func (m *mixer) processBlock(offset, blockLen int, p []byte) {
	// Lazily initialize work buffers (needed for tests that create mixer{} directly)
	if m.workBuf == nil {
		m.workBuf = make([]float64, blockSize)
		m.voiceTemp = make([]float64, blockSize)
		m.masterBuf = make([]float64, blockSize)
		if m.instSlots == nil {
			m.instSlots = make(map[string]int)
		}
	}

	// TEST_TONE mode: output pure 440Hz sine wave, bypassing all synth
	if testToneMode {
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
		return
	}

	// TEST_RAW_VOICE mode: output raw voice samples with no processing
	// This tests if the voice buffer itself is causing distortion
	if rawVoiceMode && len(m.voices) > 0 {
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
				if rawVoiceNoScale {
					// Direct value, no multiplication at all
					sum = val
				} else {
					scale := 0.2
					if rawVoiceFullAmp {
						scale = 1.0
					}
					sum = val * scale
				}

				// Debug: log sample values periodically
				if debugMixer && i == 0 && debugMixerCount%100 == 0 {
					log.Printf("[RAW_VOICE] val=%.6f sum=%.6f noscale=%v", val, sum, rawVoiceNoScale)
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
		return
	}

	// Zero output buffers
	for i := 0; i < blockLen; i++ {
		m.workBuf[i] = 0
		m.masterBuf[i] = 0
	}

	// Reset active instrument tracking
	m.activeSlots = m.activeSlots[:0]

	// === PHASE 1: Render voices + headroom → per-instrument buffers (NO EQ) ===
	i := 0
	voicesProcessed := 0
	for i < len(m.voices) {
		// SINGLE_VOICE mode: only process one voice per block
		if singleVoiceMode && voicesProcessed >= 1 {
			break
		}

		vs := m.voices[i]
		startPos := m.pos + offset

		if startPos < vs.start {
			i++
			continue
		}

		done := m.renderVoiceIntoInstBuf(vs, blockLen)
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

	// === PHASE 2: Per-instrument channel processing → masterBuf ===
	if !bypassChannelProc {
		for _, slot := range m.activeSlots {
			id := m.instSlotIDs[slot]
			ch := channelForInstrument(id)
			ch.ProcessBlockLocal(m.instBufs[slot][:blockLen], m.masterBuf[:blockLen])
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

	// === PHASE 2.5: Send effects (delay + reverb) → masterBuf ===
	if sendFX != nil && sendFX.initialized && !bypassChannelProc {
		sendFX.processSlotSends(m.instBufs, m.instSlotIDs, m.activeSlots, blockLen, m.masterBuf[:blockLen])
	}

	// === PHASE 3: Master channel processing → workBuf ===
	mainCh := channelForInstrument("") // returns main channel
	if !bypassChannelProc {
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

	// Debug: log mixer workBuf stats
	if debugMixer && debugMixerCount%1000 == 0 {
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
			log.Printf("[MIXER] voices=%d insts=%d nonZero=%d min=%.6f max=%.6f",
				len(m.voices), len(m.activeSlots), nonZero, minVal, maxVal)
		}
	}
	debugMixerCount++

	// Capture output for debugging (after all processing, before int16 conversion)
	if outputCaptureEnabled {
		outputCaptureMu.Lock()
		outputCaptureBuf = append(outputCaptureBuf, m.workBuf[:blockLen]...)
		outputCaptureMu.Unlock()
	}

	// Multi-channel capture: tap per-instrument + master for recording feature.
	// The atomic pointer check is zero-cost when not recording.
	if mc := multiCapturePtr.Load(); mc != nil {
		mc.appendBlock(m.instBufs, m.instSlotIDs, m.activeSlots, m.workBuf[:], blockLen)
	}

	// Diagnostic: count clipping events before hard clamp
	var clippedInBlock int
	if audioClipDebug {
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
			log.Printf("[AUDIO] Clipping stats: %d samples clipped in %d blocks (total blocks: %d)",
				clipSampleCount.Load(), clipBlockCount.Load(), totalBlockCount.Load())
		}
	}

	// Convert to int16 output with simple hard clamp (safety net).
	// Use math.Round instead of truncation to reduce quantization noise.
	for i := 0; i < blockLen; i++ {
		sum := m.workBuf[i]
		// Hard clamp to [-1, 1] - rarely triggers after headroom attenuation.
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
}

// debugVoiceCount tracks voice sample logging frequency
var debugVoiceCount int64

// mixHeadroom is applied per-voice BEFORE accumulation to prevent clipping.
// -12dB headroom (0.25) allows ~4 simultaneous voices at full amplitude.
const mixHeadroom = 0.25

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

// renderVoiceIntoInstBuf renders up to blockLen samples from a voice, applies
// headroom, and accumulates into the per-instrument buffer. Does NOT apply any
// channel/EQ processing — that happens in phase 2/3.
// Uses BlockVoice bulk path when available for reduced function-call overhead.
// Returns true if the voice is done and should be removed.
// Caller must hold m.mu.
func (m *mixer) renderVoiceIntoInstBuf(vs *voiceState, blockLen int) bool {
	instBuf := m.ensureInstBuf(vs.slot, blockLen)

	var rendered int
	var done bool

	// Try BlockVoice bulk path first.
	if bv, ok := vs.v.(BlockVoice); ok {
		rendered, done = bv.SampleBlock(m.voiceTemp[:blockLen])
	} else {
		// Fallback: sample-by-sample rendering.
		for i := 0; i < blockLen; i++ {
			val, d := vs.v.Sample()
			if d {
				rendered = i
				done = true
				break
			}
			m.voiceTemp[i] = val
		}
		if !done {
			rendered = blockLen
		}
	}

	if rendered > 0 {
		// Debug: log voice sample stats
		if debugMixer && debugVoiceCount%500 == 0 {
			var minVal, maxVal float64
			for i := 0; i < rendered; i++ {
				if m.voiceTemp[i] < minVal {
					minVal = m.voiceTemp[i]
				}
				if m.voiceTemp[i] > maxVal {
					maxVal = m.voiceTemp[i]
				}
			}
			log.Printf("[VOICE %s] min=%.6f max=%.6f bypass=%v block=%v",
				vs.id, minVal, maxVal, bypassChannelProc,
				func() bool { _, ok := vs.v.(BlockVoice); return ok }())
		}
		debugVoiceCount++

		// Apply per-voice headroom BEFORE accumulation to prevent clipping.
		if !bypassHeadroom {
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
