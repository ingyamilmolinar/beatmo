//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

// Deterministic offline audio-quality harness.
//
// The choppiness/lag users hear during playback is a REALTIME DELIVERY problem
// (the sequencer goroutine being starved on a saturated thread), not a signal
// COMPUTATION problem — an offline render of the same circuit is always clean
// because the samples are computed correctly; only their real-time delivery
// glitches. So this harness validates SIGNAL CORRECTNESS deterministically:
// it drives the real desktop mixer (no Oto, no wall clock) block-by-block,
// captures the post-master-chain master PCM, and lets a caller assert the
// output matches an analytically-known signal (one transient per scheduled
// trigger, silence between, no NaN/clip/noise) plus a byte-exact golden hash.
//
// A separate measurement (node_logic_cost_test.go) deterministically times the
// predictor/scheduling CPU cost that DOES cause the realtime starvation.

// burstVoice renders a short decaying sine — a deterministic, perfectly
// reproducible "hit" that survives the mixer's 5ms anti-pop fade-in (a literal
// 1-sample impulse would be multiplied by ~0 at voice start and be inaudible).
// Same params → byte-identical samples every run, so it pins a golden hash.
type burstVoice struct {
	pos      int
	n        int
	freq     float64
	decay    float64
	amp      float64
	sr       float64
}

// newBurstVoice returns the canonical harness instrument: a 1 kHz sine that
// decays over ~8ms. Deterministic and allocation-light.
func newBurstVoice() Voice {
	sr := float64(sampleRate)
	return &burstVoice{
		n:     int(0.008 * sr), // ~8ms
		freq:  1000,
		decay: 420, // e^(-420*0.008) ≈ 0.035 — fully decayed by end
		amp:   0.6,
		sr:    sr,
	}
}

func (v *burstVoice) Sample() (float64, bool) {
	if v.pos >= v.n {
		return 0, true
	}
	t := float64(v.pos) / v.sr
	s := v.amp * math.Sin(2*math.Pi*v.freq*t) * math.Exp(-v.decay*t)
	v.pos++
	return s, false
}

// newTestMixer builds a mixer driven directly (no Oto context / no realtime
// player). Field set mirrors newMixer/newTemplateMixer.
func newTestMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[string]int),
	}
}

// renderMixer drives the mixer block-by-block and returns the captured master
// PCM (float64, post headroom/EQ/compressor/limiter, pre-int16).
func renderMixer(m *mixer, nSamples int) []float64 {
	ClearOutputCapture()
	StartOutputCapture()
	buf := make([]byte, blockSize*2)
	for done := 0; done < nSamples; {
		n := blockSize
		if done+n > nSamples {
			n = nSamples - done
		}
		m.Read(buf[:n*2])
		done += n
	}
	return StopOutputCapture()
}

// --- analysis helpers (deterministic signal validation) ---------------------

func argmaxAbs(x []float64) (int, float64) {
	bi, bv := 0, 0.0
	for i, v := range x {
		if a := math.Abs(v); a > bv {
			bv, bi = a, i
		}
	}
	return bi, bv
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// detectOnsets returns the start index of each transient: a rising edge above
// `floor` that follows at least `refractory` samples of quiet. A single
// sustained burst (which dips below floor only at its sine zero-crossings)
// counts exactly once; a genuinely new hit requires a quiet gap first.
func detectOnsets(x []float64, floor float64) []int {
	var onsets []int
	const refractory = 256 // samples of quiet required to re-arm
	belowRun := refractory // start armed so the first transient registers
	for i, v := range x {
		if math.Abs(v) >= floor {
			if belowRun >= refractory {
				onsets = append(onsets, i)
			}
			belowRun = 0
		} else {
			belowRun++
		}
	}
	return onsets
}

// hashFloat64PCM is a byte-exact digest of the float64 master converted to
// float32 LE (matches the cross-platform float32 transport width).
func hashFloat64PCM(buf []float64) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(float32(v)))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
