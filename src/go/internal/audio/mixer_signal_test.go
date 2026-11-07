//go:build !test

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// testVoice is a pre-rendered voice that returns samples from a buffer.
type testVoice struct {
	buf []float64
	pos int
}

func (v *testVoice) Sample() (float64, bool) {
	if v.pos >= len(v.buf) {
		return 0, true
	}
	s := v.buf[v.pos]
	v.pos++
	return s, false
}

// newSineVoice creates a testVoice with n samples of a sine wave.
func newSineVoice(freq float64, sr, n int) *testVoice {
	buf := make([]float64, n)
	for i := range buf {
		buf[i] = 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
	}
	return &testVoice{buf: buf}
}

// readMixerSamples reads n samples from the mixer into a float64 slice.
func readMixerSamples(m *mixer, n int) []float64 {
	buf := make([]byte, n*2) // 16-bit mono
	m.Read(buf)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		v := int16(buf[2*i]) | int16(buf[2*i+1])<<8
		out[i] = float64(v) / 32767.0
	}
	return out
}

// Overlapping voices of one instrument summed through ONE shared EQ: a biquad must see one continuous stream.
func TestMixerMultiVoiceSameInstrumentWithEQ(t *testing.T) {
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	// Set kick channel with lowpass EQ.
	SetChannelEQ(channel.Of(roster.Kick), sampleRate, EQBand{Kind: EQLowpass, Freq: 1000, Q: 0.707})

	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
	const voiceSamples = 4096

	// Schedule 3 overlapping kick voices all starting at sample 0.
	m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)
	m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)
	m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)

	out := readMixerSamples(m, voiceSamples*2)

	// Assert no clipping.
	for i, v := range out {
		if v > 1.0 || v < -1.0 {
			t.Fatalf("sample %d clipped: %.6f", i, v)
		}
	}

	// Assert non-zero audio was produced.
	outRMS := rms(out[:voiceSamples])
	if outRMS < 0.001 {
		t.Fatalf("expected non-zero audio output, got RMS=%.6f", outRMS)
	}

	// About 3 × 0.5 × 0.25 = 0.375 peak before EQ; 440 Hz passes the lowpass with some attenuation.
	t.Logf("multi-voice kick RMS: %.6f", outRMS)
	if outRMS > 0.5 {
		t.Fatalf("RMS unexpectedly high (possible EQ bypass): %.6f", outRMS)
	}
}

func TestMixerMultiInstrumentWithEQ(t *testing.T) {
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	// Different EQ per instrument.
	SetChannelEQ(channel.Of(roster.Kick), sampleRate, EQBand{Kind: EQLowpass, Freq: 1000, Q: 0.707})
	SetChannelEQ(channel.Of(roster.Snare), sampleRate, EQBand{Kind: EQLowpass, Freq: 5000, Q: 0.707})
	// Master: no EQ (pass-through).

	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
	const voiceSamples = 4096

	m.Schedule(channel.Of(roster.Kick), newSineVoice(200, sampleRate, voiceSamples), 0, -1, -1)
	m.Schedule(channel.Of(roster.Snare), newSineVoice(2000, sampleRate, voiceSamples), 0, -1, -1)
	m.Schedule(channel.Of(roster.Hihat), newSineVoice(8000, sampleRate, voiceSamples), 0, -1, -1)

	out := readMixerSamples(m, voiceSamples*2)

	// No clipping.
	for i, v := range out {
		if v > 1.0 || v < -1.0 {
			t.Fatalf("sample %d clipped: %.6f", i, v)
		}
	}

	// Non-zero energy.
	outRMS := rms(out[:voiceSamples])
	if outRMS < 0.001 {
		t.Fatalf("expected non-zero audio, got RMS=%.6f", outRMS)
	}

	t.Logf("multi-instrument RMS: %.6f", outRMS)
}

func TestMixerOutputCaptureMatchesWorkBuf(t *testing.T) {
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	// Set a lowpass EQ so we exercise the full pipeline.
	SetChannelEQ(channel.Of(roster.Kick), sampleRate, EQBand{Kind: EQLowpass, Freq: 2000, Q: 0.707})

	m := &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[channel.ID]int),
	}
	const voiceSamples = 4096

	StartOutputCapture()
	m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, voiceSamples), 0, -1, -1)

	// Read enough to consume the voice.
	readMixerSamples(m, voiceSamples*2)

	captured := StopOutputCapture()

	if len(captured) == 0 {
		t.Fatal("output capture returned empty buffer")
	}

	// Find peak amplitude in captured samples.
	var peak float64
	for _, v := range captured {
		if math.Abs(v) > peak {
			peak = math.Abs(v)
		}
	}

	if peak < 0.001 {
		t.Fatalf("captured audio has no energy: peak=%.6f", peak)
	}

	t.Logf("output capture: %d samples, peak=%.6f, RMS=%.6f",
		len(captured), peak, rms(captured))
}
