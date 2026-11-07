//go:build !test

package ui

// Real build only: under -tags test the synth and EQ are stubs, so a digest there is a digest of the stub.
// From the shipped synth through the real mixer and FFT observer into the render derivations, layered so a
// failure says whether the voice or the axis moved.

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audiotap"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Every channel renders separately, and the analysis only reads the last SpectrumFFTSize samples.
const realAudioTapSeconds = 2.0

// Only this instrument's notes, through the shipped path after them: its inserts, EQ and the master.
func channelPCM(t *testing.T, g *Game, instID instrument.ID) []float64 {
	t.Helper()
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	notes, total := liveArrangement(t, g, realAudioTapSeconds)
	if instID != "" {
		filtered := notes[:0:0]
		for _, n := range notes {
			if n.InstID == instID {
				filtered = append(filtered, n)
			}
		}
		notes = filtered
	}
	return audio.RenderMasterMixOffline(notes, audio.SampleRate(), total)
}

// The same FFTObserver at the size the analyser service uses, so these are the metrics the tab is handed.
func analyzeRealPCM(id channel.ID, pcm []float64, sampleRate int) analyzer.ChannelMetrics {
	obs := wave.NewFFTObserver(analyzer.SpectrumFFTSize).Observe(
		wave.Wave{Samples: pcm, SampleRate: sampleRate})

	// The tail is also the FFT window, so both layers describe the same instant.
	tail := pcm
	if n := analyzer.SpectrumFFTSize; len(tail) > n {
		tail = tail[len(tail)-n:]
	}
	peak, sum := 0.0, 0.0
	for _, v := range tail {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
		sum += v * v
	}
	rms := 0.0
	if len(tail) > 0 {
		// Sqrt is correctly rounded and identical on every platform, unlike Exp/Log.
		rms = math.Sqrt(sum / float64(len(tail)))
	}
	return analyzer.ChannelMetrics{
		ID: id, Name: id.String(),
		PeakDB: LinearToDBSafe(peak), RMSDB: LinearToDBSafe(rms),
		Waveform: tail, FFTBins: obs.Bins, FreqBins: obs.FreqBins,
		Active: peak > 0,
	}
}

func captureRealChannel(t *testing.T, g *Game, instID instrument.ID, label string) *audiotap.Snapshot {
	t.Helper()
	sr := audio.SampleRate()
	pcm := channelPCM(t, g, instID)
	ch := analyzeRealPCM(channel.Of(instID), pcm, sr)
	snap := CaptureAudioTabTap(label, &ch, nil, sr)
	// "audio.*" sorts first, so a failure reads outward from the source.
	snap.Floats("audio.pcm", pcm...)
	snap.Ints("audio.len", len(pcm), sr)
	return snap
}

// Same bytes at every layer when rendered twice; without it no golden means anything. Catches DSP state
// surviving across renders and order-dependent analysis.
func TestRealAudioTabTapIsDeterministic(t *testing.T) {
	g, ids := editedProject(t)
	for _, id := range append([]instrument.ID{""}, ids...) {
		label := "master"
		if id != "" {
			label = string(id)
		}
		t.Run(label, func(t *testing.T) {
			a := captureRealChannel(t, g, id, label)
			b := captureRealChannel(t, g, id, label)
			if diffs := a.Diff(b); len(diffs) > 0 {
				t.Fatalf("channel %q is not deterministic — the same circuit rendered twice "+
					"produced different bytes:\n%s", label, audiotap.Report(diffs))
			}
		})
	}
}

// Against real analysis output, at the rate the engine runs.
func TestRealAudioTabTapCoversEveryDisplayBand(t *testing.T) {
	g, ids := editedProject(t)
	sr := audio.SampleRate()
	for _, id := range ids {
		ch := analyzeRealPCM(channel.Of(id), channelPCM(t, g, id), sr)
		_, measured := spectrumBandDB(ch.FFTBins, ch.FreqBins, isoBands)
		for b, ok := range measured {
			if !ok {
				t.Errorf("%s @ %d Hz: display band %d (%.0f-%.0f Hz) holds no FFT bin — "+
					"it reports the reference floor whatever this instrument does",
					id, sr, b, isoBands[b][0], isoBands[b][1])
			}
		}
	}
}

// Four instruments, four digests at every layer: a collision means the capture is not reading the audio.
func TestRealAudioTabTapDistinguishesInstruments(t *testing.T) {
	g, ids := editedProject(t)
	seen := map[string]string{}
	for _, id := range ids {
		snap := captureRealChannel(t, g, id, string(id))
		key := snap.Overall()
		if prev, dup := seen[key]; dup {
			t.Errorf("%s and %s produced an IDENTICAL capture — the tap is not reading "+
				"per-channel audio", prev, id)
		}
		seen[key] = string(id)
	}
	if len(seen) != len(ids) {
		t.Errorf("captured %d distinct digests for %d instruments", len(seen), len(ids))
	}
}
