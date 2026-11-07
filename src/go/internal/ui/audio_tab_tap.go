package ui

// Calls the SAME functions the renderers call: a parallel reimplementation would agree with itself
// while the renderer drifted, so a mismatch is a bug here. No build tags: the browser must reproduce
// the native golden byte for byte.

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audiotap"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A live analyser differs by platform (Hann + Go FFT vs Blackman + the browser's), so the shared
// input is synthesized from integer math and Exp/Sin. Deliberately BASS-HEAVY: top-heavy fixtures hid
// empty bass bands.
func CanonicalChannelMetrics(id channel.ID, bins, sampleRate int) analyzer.ChannelMetrics {
	spec := make([]float64, bins)
	nyquist := float64(sampleRate) / 2
	for i := range spec {
		hz := float64(i) * nyquist / float64(bins)
		// A small broadband floor so no band is zero for the wrong reason.
		v := 0.9*math.Exp(-hz/90.0) + 0.05*math.Exp(-hz/2500.0) + 0.0008
		spec[i] = v
	}
	fft, freq := SpectrumLinearToDBBins(spec, sampleRate)

	wave := make([]float64, 512)
	for i := range wave {
		t := float64(i) / float64(sampleRate)
		wave[i] = 0.7 * math.Exp(-t*40) * math.Sin(2*math.Pi*60*t)
	}
	peak, sum := 0.0, 0.0
	for _, v := range wave {
		if a := math.Abs(v); a > peak {
			peak = a
		}
		sum += v * v
	}
	return analyzer.ChannelMetrics{
		ID:       id,
		Name:     id.String(),
		PeakDB:   LinearToDBSafe(peak),
		RMSDB:    LinearToDBSafe(math.Sqrt(sum / float64(len(wave)))),
		Waveform: wave,
		FFTBins:  fft,
		FreqBins: freq,
		Active:   true,
	}
}

// Pass audio.BridgeSpectrumBins for the browser's shape.
func CanonicalAnalyzerState(masterBins, detailBins, sampleRate int) *analyzer.State {
	master := CanonicalChannelMetrics(channel.Master, masterBins, sampleRate)
	detail := CanonicalChannelMetrics(channel.Of(roster.DnbKick), detailBins, sampleRate)
	return &analyzer.State{
		Master: master,
		Detail: &detail,
		Instruments: []analyzer.InstrumentMetrics{
			{ID: channel.Of(roster.DnbKick), Name: "Dnb Kick", PeakDB: detail.PeakDB, RMSDB: detail.RMSDB, Active: true},
			{ID: channel.Of(roster.Snare), Name: "Snare", PeakDB: -18.5, RMSDB: -24.25, Active: true},
			{ID: channel.Of(roster.Shaker), Name: "Shaker", PeakDB: analyzerDBFloor, RMSDB: analyzerDBFloor, Active: false},
		},
	}
}

// label is part of the digest, so captures of different things never match.
func CaptureAudioTabTap(label string, ch *analyzer.ChannelMetrics, st *analyzer.State, sampleRate int) *audiotap.Snapshot {
	snap := audiotap.New(label, sampleRate)
	if ch == nil {
		snap.Ints("shape.absent", 1)
		return snap
	}

	// The only layer comparable against a live analyser on the other platform; binHz in integer
	// millihertz so it is exact.
	binHzMilli := 0
	if n := len(ch.FFTBins); n > 0 {
		binHzMilli = int(math.Round(float64(sampleRate) / 2 / float64(n) * 1000))
	}
	snap.Ints("shape.spectrum", len(ch.FFTBins), len(ch.FreqBins), binHzMilli, sampleRate)
	snap.Ints("shape.waveform", len(ch.Waveform))

	// Bins per display band is a property of the data's resolution, so it belongs to shape.
	counts := make([]int, len(isoBands))
	for b, band := range isoBands {
		for _, hz := range ch.FreqBins {
			if hz >= band[0] && hz < band[1] {
				counts[b]++
			}
		}
	}
	snap.Ints("shape.band_bin_counts", counts...)

	// ---- analysis ------------------------------------------------------
	snap.Floats("analysis.fft_bins", ch.FFTBins...)
	snap.Floats("analysis.freq_bins", ch.FreqBins...)
	snap.Floats("analysis.waveform", ch.Waveform...)
	snap.Floats("analysis.levels", ch.PeakDB, ch.RMSDB)
	snap.Bools("analysis.active", []bool{ch.Active})

	// The renderer's own functions, in its own order.
	bandDB, measured := spectrumBandDB(ch.FFTBins, ch.FreqBins, isoBands)
	floor, ceil := audioDBWindow(spectrumPeakDB(ch))
	bars := make([]float64, len(bandDB))
	for i, db := range bandDB {
		bars[i] = audioDBNorm(db, floor, ceil)
	}
	snap.Floats("render.spectrum.band_db", bandDB[:]...)
	snap.Bools("render.spectrum.band_measured", measured[:])
	snap.Floats("render.spectrum.window", floor, ceil)
	snap.Floats("render.spectrum.bar_norms", bars...)

	// Run eqBandLevels to a fixed point, or the digest encodes how many frames elapsed.
	lin := make([]float64, len(ch.FFTBins))
	for i, db := range ch.FFTBins {
		lin[i] = math.Pow(10, db/20)
	}
	var eqLevels []float64
	win := spectrumDBWindow{Floor: floor, Ceil: ceil}
	for i := 0; i < 64; i++ {
		eqLevels = eqBandLevels(lin, eqLevels, win)
	}
	snap.Floats("render.eq.band_levels", eqLevels...)

	// ---- render: Wave ---------------------------------------------------
	wavePeak := 0.0
	for _, v := range ch.Waveform {
		if a := math.Abs(v); a > wavePeak {
			wavePeak = a
		}
	}
	gain := autoGainForPeak(wavePeak)
	scaled := make([]float64, len(ch.Waveform))
	for i, v := range ch.Waveform {
		scaled[i] = v * gain
	}
	snap.Floats("render.wave.gain", gain)
	snap.Floats("render.wave.scaled", scaled...)

	// ---- render: Levels -------------------------------------------------
	if st != nil {
		ids := make([]string, 0, len(st.Instruments))
		levels := make([]float64, 0, len(st.Instruments)*2)
		active := make([]bool, 0, len(st.Instruments))
		for _, im := range st.Instruments {
			ids = append(ids, im.ID.String())
			levels = append(levels, im.PeakDB, im.RMSDB)
			active = append(active, im.Active)
		}
		snap.Strings("render.levels.ids", ids...)
		snap.Floats("render.levels.db", levels...)
		snap.Bools("render.levels.active", active)
	}
	return snap
}
