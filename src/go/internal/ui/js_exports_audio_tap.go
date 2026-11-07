//go:build js && !test

package ui

// audioTabTapDigest runs the same derivations over the same fixture as the native golden, so any difference is
// a Go-under-wasm divergence; audioTabTapLiveShape measures the live bridge, where only the SHAPE compares.

import (
	"encoding/json"
	"math"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

func (g *Game) initJSAudioTap() {
	// No live state on purpose: it compares CODE across platforms.
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

	// How many bins arrived, how wide, and how many land in each band.
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
