//go:build js && !test

package ui

// Browser half of the audio-tab source-of-truth framework.
//
// Two exports, answering two different questions:
//
//	audioTabTapDigest()    — run the SAME Go derivations over the SAME
//	                         canonical fixture the native golden was built
//	                         from. Any difference is a genuine
//	                         Go-under-wasm divergence, so the comparison is
//	                         byte for byte.
//	audioTabTapLiveShape() — measure what the LIVE bridge actually delivers
//	                         for a real channel. Values here are NOT
//	                         comparable to native (Hann vs Blackman
//	                         windows, live timing); the SHAPE is, and shape
//	                         is where the 2026-09-19 blank-Spectrum defect
//	                         lived.

import (
	"encoding/json"
	"math"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

func (g *Game) initJSAudioTap() {
	// audioTabTapDigest() -> the layered capture as JSON, over the
	// canonical fixture. Takes no live state on purpose: this export exists
	// to compare CODE across platforms, and a live circuit would make the
	// input differ before the code ever ran.
	js.Global().Set("audioTabTapDigest", jsFn(func(args jsArgs) any {
		const sr = 48000
		st := CanonicalAnalyzerState(audio.BridgeSpectrumBins, audio.BridgeSpectrumBins, sr)
		snap := CaptureAudioTabTap("canonical/dnb-kick", st.Detail, st, sr)
		b, err := json.Marshal(snap)
		if err != nil {
			return js.Null()
		}
		return js.Global().Get("JSON").Call("parse", string(b))
	}))

	// audioTabTapLiveShape(channelID) -> the shape layer measured from the
	// real analyser bridge: how many bins arrived, how wide they are, and
	// how many land in each display band.
	//
	// Under the defect, bands 0-3 read 0 here while every audio assertion
	// in the repo stayed green.
	js.Global().Set("audioTabTapLiveShape", jsFn(func(args jsArgs) any {
		id := args.ChannelOr(0, channel.Master)
		if id.IsZero() {
			return nil
		}
		sr := audio.SampleRate()
		raw := audio.ChannelAnalyzerSnapshot(id)
		fft, freq := SpectrumLinearToDBBins(raw.Spectrum, sr)
		ch := analyzer.ChannelMetrics{
			ID: id, Name: id.String(),
			PeakDB: LinearToDBSafe(raw.Peak), RMSDB: LinearToDBSafe(raw.RMS),
			Waveform: raw.Waveform, FFTBins: fft, FreqBins: freq,
			Active: raw.Peak > 0 || raw.RMS > 0,
		}
		counts := make([]any, len(isoBands))
		for b, band := range isoBands {
			n := 0
			for _, hz := range ch.FreqBins {
				if hz >= band[0] && hz < band[1] {
					n++
				}
			}
			counts[b] = n
		}
		binHzMilli := 0
		if n := len(ch.FFTBins); n > 0 {
			binHzMilli = int(math.Round(float64(sr) / 2 / float64(n) * 1000))
		}
		_, measured := spectrumBandDB(ch.FFTBins, ch.FreqBins, isoBands)
		meas := make([]any, len(measured))
		for i, m := range measured {
			meas[i] = m
		}
		return map[string]any{
			"channel":        id.String(),
			"sampleRate":     sr,
			"spectrumBins":   len(ch.FFTBins),
			"waveformLen":    len(ch.Waveform),
			"binHzMilli":     binHzMilli,
			"bandBinCounts":  counts,
			"bandMeasured":   meas,
			"declaredBins":   audio.BridgeSpectrumBins,
			"declaredWave":   audio.BridgeWaveformSamples,
			"declaredFFTLen": audio.BridgeAnalyserFFTSize,
		}
	}))
}
