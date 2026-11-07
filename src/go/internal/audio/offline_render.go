//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Choppy playback is a REAL-TIME DELIVERY problem; an offline render computes the same samples cleanly. So this checks
// SIGNAL correctness deterministically through the real mixer, no device and no wall clock.

// A decaying sine, not a 1-sample impulse, which the 5 ms fade-in would multiply by ~0.
type burstVoice struct {
	pos   int
	n     int
	freq  float64
	decay float64
	amp   float64
	sr    float64
}

// 1 kHz decaying over ~8 ms.
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

// No device or real-time player.
func newTestMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
}

// Post headroom/EQ/soft clip, pre-int16.
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

// A burst dipping below floor only at its zero crossings counts once; a new hit needs a quiet gap first.
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

// float32 LE: the cross-platform transport width.
func hashFloat64PCM(buf []float64) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(float32(v)))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
