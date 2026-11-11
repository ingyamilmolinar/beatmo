package ui

// Source-of-truth capture for the audio panel's data tabs.
//
// CaptureAudioTabTap walks a channel from the shape of the analyser data,
// through the analysis values, to the exact numbers each tab will draw —
// and digests each layer separately (see internal/audiotap for why layered).
//
// The render layer calls the SAME functions the renderers call
// (spectrumBandDB, audioDBWindow, audioDBNorm, eqBandLevels,
// autoGainForPeak). That is the whole point: a digest computed by a
// parallel reimplementation would agree with itself forever while the real
// renderer drifted. Anything here that stops matching a renderer is a bug
// in this file, not a reason to relax the test.
//
// No build tags. It has to compile and produce identical bytes both
// natively and under GOOS=js, because the cross-platform assertion is
// "the browser, running this code, reproduces the native golden".

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audiotap"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// CanonicalChannelMetrics builds a deterministic, platform-independent
// stand-in for one channel's analyser reading.
//
// Cross-platform value comparison needs an input both sides can generate
// bit-identically, which a live analyser is not: Go's observer windows with
// Hann and runs its own FFT, a WebAudio AnalyserNode windows with Blackman
// and runs the browser's. Comparing those values would be comparing two
// correct answers to slightly different questions.
//
// So the shared input is synthesized here, from integer arithmetic and
// math.Exp/Sin only, and both platforms run the real derivations over it.
// The spectrum is deliberately BASS-HEAVY — a kick — because a top-heavy
// fixture is exactly what hid the original defect: every existing fixture
// decayed from bin 0, so the mid and high bands always had data and nobody
// noticed the bottom four bands were empty.
func CanonicalChannelMetrics(id channel.ID, bins, sampleRate int) analyzer.ChannelMetrics {
	spec := make([]float64, bins)
	nyquist := float64(sampleRate) / 2
	for i := range spec {
		hz := float64(i) * nyquist / float64(bins)
		// A 60 Hz fundamental with a decaying harmonic series, plus a small
		// broadband floor so no band is ever exactly zero for the wrong
		// reason.
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

// CanonicalAnalyzerState is a whole-panel fixture: a master bus plus one
// selected instrument channel, at the resolution the platform actually
// ships (pass audio.BridgeSpectrumBins for the browser's shape).
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

// CaptureAudioTabTap digests one channel across all three layers.
//
// `label` names what was captured (circuit + channel) and is part of the
// overall digest, so two captures of different things can never be
// mistaken for a match.
func CaptureAudioTabTap(label string, ch *analyzer.ChannelMetrics, st *analyzer.State, sampleRate int) *audiotap.Snapshot {
	snap := audiotap.New(label, sampleRate)
	if ch == nil {
		snap.Ints("shape.absent", 1)
		return snap
	}

	// ---- shape ---------------------------------------------------------
	// The layer the 2026-09-19 defect lived in, and the only one that is
	// meaningfully comparable against a LIVE analyser on the other
	// platform. binHz is recorded in millihertz as an integer so it is
	// exact rather than a float that "nearly" matches.
	binHzMilli := 0
	if n := len(ch.FFTBins); n > 0 {
		binHzMilli = int(math.Round(float64(sampleRate) / 2 / float64(n) * 1000))
	}
	snap.Ints("shape.spectrum", len(ch.FFTBins), len(ch.FreqBins), binHzMilli, sampleRate)
	snap.Ints("shape.waveform", len(ch.Waveform))

	// Band coverage belongs to shape, not render: "how many FFT bins land
	// in each display band" is a property of the data's resolution, and it
	// is the single number that went to zero for the bottom four bands.
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

	// ---- render: Spectrum ---------------------------------------------
	// Through the renderer's own functions, in the renderer's own order.
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

	// ---- render: EQ backdrop -------------------------------------------
	// eqBandLevels smooths toward its target, so one pass is a transient.
	// Run it to a fixed point (the blend halves the error each step) and
	// digest the settled heights — otherwise the digest would encode "how
	// many frames had elapsed", which is not a property of the circuit.
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
