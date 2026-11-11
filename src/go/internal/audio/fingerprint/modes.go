package fingerprint

import (
	"math"
	"sort"
)

// modes.go — the f0-free partial tracker.
//
// Every partial-structure descriptor before this file anchored on DetectF0.
// That is the wrong model for bars, bells, drums and cymbals: 19 of 59
// corpus references have no reliable fundamental, and on the ones that do
// (kalimba, tubular-bells) the two sides of a comparison landed on different
// pitch grids. A mode is simply a spectral peak that persists across frames;
// ratios are taken against the LOWEST prominent mode. Both sides of any
// comparison use the same rule, so a bell's hum partial anchors both.

const (
	modeFFTSize      = 8192
	modeHop          = 2048
	modeMinFrames    = 4     // a peak must persist this many frames to be a mode
	modeMinRelDB     = -50.0 // per-frame floor relative to the frame's loudest bin
	modeProminenceDB = 6.0   // over the neighbouring +/-3 bins
	modeLinkTolFrac  = 0.005 // +/-0.5% frequency continuity between frames
	modeStrongWithin = 20.0  // Strong()'s default window, in dB below the loudest mode
	modeDecayFloorDB = 40.0  // fit the decay from the peak down to this far below it
	modeMaxPerFrame  = 40
	modeMinHz        = 30.0

	// modeMaxMissFrames tolerates this many CONSECUTIVE frames with no
	// matching peak before a track is declared done, but only once the track
	// already has 2+ matched frames of its own (see the length check where
	// it is used) — a brand-new single-hit track gets no tolerance at all.
	// Two real partials closer together than modeLinkTolFrac's own bin cap
	// beat against each other; at the beat's amplitude null the composite
	// peak drops below framePeaks' prominence/floor gate for a frame or two
	// even though the partial itself never stopped sounding. Without
	// tolerance, a killed-and-restarted track needs modeMinFrames
	// consecutive hits to count again, so a beating partial fragments into
	// pieces mostly too short to ever become a Mode — verified against a
	// 880/884 Hz pair (4 Hz beat, ~5.4 frames per cycle at modeHop): frames
	// drop out roughly once per cycle (occasionally two in a row), and
	// without gap tolerance only 1 of ~7 cycles happened to land 4 frames
	// long by chance, discarding the rest of the track's history.
	//
	// 1 frame (~46ms at modeHop/44.1kHz) was chosen over 2: with the
	// established-track guard alone, 2 pushed white noise's stable mode
	// count from 6 to 7 against TestAnalyzeModes_NoiseHasNoStableModes's
	// budget, while 1 keeps that test's margin AND is enough to bridge the
	// 880/884 Hz beat test (most drop-outs there are a single frame; the
	// rare two-in-a-row gaps just end that particular cycle's track a beat
	// early, which the caller's beat-rate estimate already tolerates).
	modeMaxMissFrames = 1
)

type Mode struct {
	Hz            float64 // amplitude-weighted mean frequency over the track
	PeakDB        float64 // loudest frame level, dBFS
	PeakFrame     int
	DecayDBPerSec float64   // OLS slope of dB vs seconds from PeakFrame to the last tracked frame (or first frame <= PeakDB-40); bridged gap frames are skipped, never interpolated
	DecayOK       bool      // false when fewer than 3 frames were available for the fit
	Track         []float64 // dB per frame; math.NaN() where the mode was not present
}

type ModeSet struct {
	Modes     []Mode // ascending Hz
	Anchor    int    // index into Modes; -1 when none
	HopSec    float64
	NumFrames int
}

const naNoModes = "fewer than two stable spectral modes were tracked"

type modeTrack struct {
	freqs, amps []float64 // per hit
	frames      []int
	lastHz      float64
	miss        int // consecutive frames since the last matched peak
}

// framePeaks returns the prominent local maxima of one magnitude frame.
func framePeaks(mag []float64, binHz float64) []spectralPartial {
	peaks := peakPickPartials(mag, binHz, modeMinHz, modeMaxPerFrame*4)
	if len(peaks) == 0 {
		return nil
	}
	maxAmp := 0.0
	for _, p := range peaks {
		if p.Amp > maxAmp {
			maxAmp = p.Amp
		}
	}
	floor := maxAmp * math.Pow(10, modeMinRelDB/20)
	var out []spectralPartial
	for _, p := range peaks {
		if p.Amp < floor {
			continue
		}
		bin := int(p.Freq/binHz + 0.5)
		// prominence over the neighbourhood, excluding the peak's own +/-1 bins
		nb := 0.0
		for d := 2; d <= 3; d++ {
			for _, j := range []int{bin - d, bin + d} {
				if j >= 0 && j < len(mag) && mag[j] > nb {
					nb = mag[j]
				}
			}
		}
		if nb > 0 && 20*math.Log10(p.Amp/nb) < modeProminenceDB {
			continue
		}
		out = append(out, p)
		if len(out) >= modeMaxPerFrame {
			break
		}
	}
	return out
}

// AnalyzeModes tracks spectral peaks across frames of st into persistent
// modes: a peak that survives modeMinFrames consecutive frames (linked by
// frequency continuity within modeLinkTolFrac) becomes a Mode.
func AnalyzeModes(st *STFTFrames) ModeSet {
	ms := ModeSet{Anchor: -1}
	if st == nil || len(st.Frames) == 0 {
		return ms
	}
	ms.HopSec, ms.NumFrames = st.HopSec, len(st.Frames)

	var active []*modeTrack
	var done []*modeTrack
	for fi, mag := range st.Frames {
		peaks := framePeaks(mag, st.BinHz)
		used := make([]bool, len(peaks))
		var next []*modeTrack
		for _, tr := range active {
			best, bestGap := -1, math.Inf(1)
			// tol is proportional to frequency (so a stable tone's small
			// relative jitter is tolerated) but capped in BIN terms — an
			// uncapped frequency-proportional tolerance grows to many bins
			// wide at high frequencies (8192-pt FFT / 44.1kHz means every
			// frequency shares one fixed ~5.4Hz bin width, so scaling the
			// tolerance by lastHz alone makes it 40+ bins wide near Nyquist),
			// which let unrelated noise peaks link across frames purely by
			// chance in the high band (measured: 11 spurious "modes" above
			// 9kHz with no cap).
			//
			// The cap must still admit real pitch drift/vibrato on a high
			// partial: a slowly detuning bell/bar mode swinging +/-0.3% at a
			// 1Hz rate needs at most ~df/dt = 2*pi*(0.003*4000Hz)*1Hz =~
			// 75Hz/s, i.e. ~3.5Hz of step per modeHop (2048/44100s); a 4-bin
			// cap (~21.5Hz) leaves that a >6x margin. Empirically, 2-5 bins
			// all track that drifting partial as one unbroken mode and keep
			// noise's spurious count low; 6 bins is where noise breaks
			// through (7 spurious modes). 4 bins is chosen as the midpoint —
			// comfortable margin below the 6-bin noise-failure point while
			// giving more headroom than the bare 3.5Hz requirement for
			// parabolic-interpolation jitter on real (non-idealized) audio.
			tol := math.Min(math.Max(tr.lastHz*modeLinkTolFrac, st.BinHz), 4*st.BinHz)
			for i, p := range peaks {
				if used[i] {
					continue
				}
				if gap := math.Abs(p.Freq - tr.lastHz); gap <= tol && gap < bestGap {
					best, bestGap = i, gap
				}
			}
			if best < 0 {
				tr.miss++
				// A brand-new, single-hit track gets no gap tolerance: that is
				// exactly the shape of an isolated noise spike, and letting it
				// bridge a gap to whatever shows up next would let unrelated
				// noise peaks link into a false "mode" purely by chance
				// (measured: bridging every track pushed white noise's stable
				// mode count from 6 to 7, over TestAnalyzeModes_NoiseHasNoStableModes's
				// budget). Requiring at least 2 already-matched frames means
				// only a track with some established continuity can survive
				// the gap.
				if tr.miss > modeMaxMissFrames || len(tr.frames) < 2 {
					done = append(done, tr)
					continue
				}
				// Still within tolerance: keep the track alive against its
				// last known frequency without recording a frame for this
				// gap (buildMode leaves ungapped indices as NaN already).
				next = append(next, tr)
				continue
			}
			used[best] = true
			tr.freqs = append(tr.freqs, peaks[best].Freq)
			tr.amps = append(tr.amps, peaks[best].Amp)
			tr.frames = append(tr.frames, fi)
			tr.lastHz = peaks[best].Freq
			tr.miss = 0
			next = append(next, tr)
		}
		for i, p := range peaks {
			if !used[i] {
				next = append(next, &modeTrack{freqs: []float64{p.Freq}, amps: []float64{p.Amp}, frames: []int{fi}, lastHz: p.Freq})
			}
		}
		active = next
	}
	done = append(done, active...)

	for _, tr := range done {
		if len(tr.frames) < modeMinFrames {
			continue
		}
		ms.Modes = append(ms.Modes, buildMode(tr, ms.NumFrames, st.HopSec))
	}
	sort.Slice(ms.Modes, func(i, j int) bool { return ms.Modes[i].Hz < ms.Modes[j].Hz })
	ms.Modes = mergeCloseModes(ms.Modes, st.BinHz)

	// Anchor: the lowest mode within modeStrongWithin dB of the loudest.
	maxDB := math.Inf(-1)
	for _, m := range ms.Modes {
		maxDB = math.Max(maxDB, m.PeakDB)
	}
	for i, m := range ms.Modes {
		if m.PeakDB >= maxDB-modeStrongWithin {
			ms.Anchor = i
			break
		}
	}
	return ms
}

func buildMode(tr *modeTrack, numFrames int, hopSec float64) Mode {
	m := Mode{Track: make([]float64, numFrames)}
	for i := range m.Track {
		m.Track[i] = math.NaN()
	}
	var wsum, fsum float64
	peakDB, peakFrame := math.Inf(-1), 0
	for i, f := range tr.frames {
		db := ampToDBFloor(tr.amps[i])
		m.Track[f] = db
		wsum += tr.amps[i]
		fsum += tr.amps[i] * tr.freqs[i]
		if db > peakDB {
			peakDB, peakFrame = db, f
		}
	}
	m.Hz = fsum / wsum
	m.PeakDB, m.PeakFrame = peakDB, peakFrame

	// The decay fit SKIPS gap frames rather than stopping at them.
	//
	// modeMaxMissFrames exists because a beating partial's composite peak
	// dips below the peak-picker's gate for a frame or two at each amplitude
	// null even though the partial never stopped sounding — that is why the
	// TRACKER bridges the gap and keeps the track alive. The gap frame is
	// left as NaN in Track (nothing is interpolated: the tracker records no
	// amplitude for a frame it never measured, and inventing one would put a
	// fabricated sample into an OLS fit). Breaking the fit at the first NaN
	// threw away every frame AFTER the gap, which is precisely the tolerance
	// the tracker had just bought — on a beating mode the fit collapsed to
	// the ~5 frames before the first null and measured the beat's own dip
	// instead of the decay envelope.
	//
	// So a NaN is skipped and the fit continues. It still stops for the two
	// reasons that are real endings: the level falling modeDecayFloorDB
	// below the peak (below that the reading is noise floor, not decay), and
	// running out of frames.
	var xs, ys []float64
	for f := peakFrame; f < numFrames; f++ {
		db := m.Track[f]
		if math.IsNaN(db) {
			continue
		}
		if db < peakDB-modeDecayFloorDB {
			break
		}
		xs = append(xs, float64(f-peakFrame)*hopSec)
		ys = append(ys, db)
	}
	if len(xs) >= 3 {
		m.DecayDBPerSec = olsSlope(xs, ys)
		m.DecayOK = true
	}
	return m
}

// mergeCloseModes folds tracks closer than one bin into the louder one — a
// track that briefly split across a bin boundary must not count twice.
func mergeCloseModes(modes []Mode, binHz float64) []Mode {
	var out []Mode
	for _, m := range modes {
		if n := len(out); n > 0 && m.Hz-out[n-1].Hz < binHz {
			if m.PeakDB > out[n-1].PeakDB {
				out[n-1] = m
			}
			continue
		}
		out = append(out, m)
	}
	return out
}

// Strong returns the modes whose PeakDB is within withinDB of the loudest
// mode's PeakDB, ascending Hz.
func (m ModeSet) Strong(withinDB float64) []Mode {
	maxDB := math.Inf(-1)
	for _, x := range m.Modes {
		maxDB = math.Max(maxDB, x.PeakDB)
	}
	var out []Mode
	for _, x := range m.Modes {
		if x.PeakDB >= maxDB-withinDB {
			out = append(out, x)
		}
	}
	return out
}

// AnchorHz returns the anchor mode's frequency, or 0 when there is none.
func (m ModeSet) AnchorHz() float64 {
	if m.Anchor < 0 || m.Anchor >= len(m.Modes) {
		return 0
	}
	return m.Modes[m.Anchor].Hz
}

// Ratios returns the first n Strong(20) mode frequencies divided by the
// anchor frequency (anchor itself included as 1.0 when it qualifies).
func (m ModeSet) Ratios(n int) []float64 {
	a := m.AnchorHz()
	if a <= 0 {
		return nil
	}
	var out []float64
	for _, x := range m.Strong(modeStrongWithin) {
		if x.Hz < a*0.999 {
			continue
		}
		out = append(out, x.Hz/a)
		if len(out) >= n {
			break
		}
	}
	return out
}

// Modes returns the bundle's tracked spectral modes, computed once.
func (b *Bundle) Modes() *ModeSet {
	b.modesOnce.Do(func() {
		ms := AnalyzeModes(b.STFT(modeFFTSize, modeHop))
		b.modes = &ms
	})
	return b.modes
}

// RequiresModes gates on the signal being audible and having at least two
// stable spectral modes worth comparing.
func RequiresModes(b *Bundle) bool {
	return RequiresAudible(b) && len(b.Modes().Strong(modeStrongWithin)) >= 2
}
