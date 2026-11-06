//go:build !test

package ui

// END-TO-END source-of-truth capture: real audio -> real analysis -> real
// render inputs, digested byte-for-byte.
//
// WHY A REAL-BUILD FILE. Under `-tags test` the synth renders through a
// stub and NewEQProcessor is a pass-through gain (internal/audio/stub.go),
// so an audio digest there would be a digest of the stub. Same reasoning
// as export_import_master_audio_harness_test.go, whose harness this reuses
// rather than growing a second one. Runs under `make test-real`.
//
// WHAT IT PROVES, and what its fast-path sibling cannot:
//
//	audio_tab_tap_test.go        starts from a CANONICAL fixture. It pins
//	                             the derivations and the cross-platform
//	                             contract, but it never touches the DSP.
//	this file                    starts from the shipped synth, through the
//	                             real mixer, into the real FFT observer the
//	                             desktop analyzer service uses, and on into
//	                             the real render derivations. If a voice, an
//	                             insert effect, the mixer or the analysis
//	                             window changes what a tab displays, it
//	                             moves a digest here.
//
// The layering matters as much as the digest. A single end-to-end hash
// would say "the Spectrum tab changed" and leave you to find out whether
// the synth moved or the axis did. These say which.

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

// realAudioTapSeconds is the render window. Shorter than the identity
// tests' 4 s: this walks every channel separately, and the analysis window
// only ever reads the last SpectrumFFTSize samples anyway.
const realAudioTapSeconds = 2.0

// channelPCM renders ONE instrument's contribution through the real offline
// mixer — the per-channel source of truth an audio tab's channel selector
// points at. Passing only that instrument's notes is what isolates it;
// everything downstream (its insert chain, its EQ, its sends, the master
// bus) is the shipped signal path.
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

// analyzeRealPCM runs the DESKTOP analysis path over rendered audio: the
// same wave.FFTObserver at the same size the analyzer service configures
// (analyzer.SpectrumFFTSize), so the ChannelMetrics this produces is what
// the tab would actually be handed.
func analyzeRealPCM(id channel.ID, pcm []float64, sampleRate int) analyzer.ChannelMetrics {
	obs := wave.NewFFTObserver(analyzer.SpectrumFFTSize).Observe(
		wave.Wave{Samples: pcm, SampleRate: sampleRate})

	// The waveform the Wave tab draws is the tail of the buffer, which is
	// also the window the FFT was taken over — so the two layers describe
	// the same instant instead of drifting apart.
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
		// math.Sqrt is a single IEEE-754 operation, correctly rounded and
		// therefore identical on every platform — unlike Exp/Log, which is
		// why the cross-platform digest exists at all.
		rms = math.Sqrt(sum / float64(len(tail)))
	}
	return analyzer.ChannelMetrics{
		ID: id, Name: id.String(),
		PeakDB: LinearToDBSafe(peak), RMSDB: LinearToDBSafe(rms),
		Waveform: tail, FFTBins: obs.Bins, FreqBins: obs.FreqBins,
		Active: peak > 0,
	}
}

// captureRealChannel is the whole pipeline for one channel, as one
// layered snapshot.
func captureRealChannel(t *testing.T, g *Game, instID instrument.ID, label string) *audiotap.Snapshot {
	t.Helper()
	sr := audio.SampleRate()
	pcm := channelPCM(t, g, instID)
	ch := analyzeRealPCM(channel.Of(instID), pcm, sr)
	snap := CaptureAudioTabTap(label, &ch, nil, sr)
	// The audio layer itself, prepended conceptually (section order in the
	// report is alphabetical by layer, so "audio.*" sorts first and the
	// failure reads outward from the source).
	snap.Floats("audio.pcm", pcm...)
	snap.Ints("audio.len", len(pcm), sr)
	return snap
}

// TestRealAudioTabTapIsDeterministic — the same circuit, rendered and
// analysed twice, is the same bytes at every layer.
//
// This is the property the whole framework rests on: if it does not hold,
// no golden downstream means anything. It catches DSP state surviving
// across renders (the class of defect audio.ResetDSPState exists for —
// biquad history, delay lines, LFO phase, compressor envelope), and any
// analysis or derivation that quietly depends on call order.
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

// TestRealAudioTabTapCoversEveryDisplayBand — the shape invariant against
// REAL analysis output rather than a fixture, at the sample rate the engine
// is actually running.
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

// TestRealAudioTabTapDistinguishesInstruments — a sanity check on the
// capture's discriminating power. Four different instruments must produce
// four different digests at every layer; if they collide, the capture is
// reading something other than the audio and every other test here is
// vacuous.
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
