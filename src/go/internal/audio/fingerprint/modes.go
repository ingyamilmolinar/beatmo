package fingerprint

import (
	"math"
	"sort"
)

// An f0-free partial tracker: bars, bells, drums and cymbals often have no reliable fundamental, and anchoring on
// DetectF0 put the two sides of a comparison on different pitch grids. A mode is a peak that persists across frames;
// ratios are taken against the LOWEST prominent mode, the same rule on both sides.

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

	// A beating pair's composite peak drops under the gate for a frame at each null while the partial still sounds; with
	// no tolerance the track fragments into pieces too short to be Modes. Only an established track (2+ frames) gets
	// it. 1 frame bridges most drops while keeping white noise's spurious mode count within budget; 2 did not.
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

// A peak surviving modeMinFrames consecutive frames, linked within modeLinkTolFrac, becomes a Mode.
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
			// Proportional to frequency but capped in BINS: at 8192/44.1 kHz an uncapped tolerance is 40+ bins wide near Nyquist
			// and noise links into spurious modes. 4 bins still follows a high partial's slow drift with margin, below where
			// noise breaks through (6).
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
				// A single-hit track is exactly an isolated noise spike; letting it bridge a gap links unrelated noise into false
				// modes.
				if tr.miss > modeMaxMissFrames || len(tr.frames) < 2 {
					done = append(done, tr)
					continue
				}
				// Kept alive against its last frequency with no frame recorded (buildMode leaves it NaN).
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

	// The fit SKIPS gap frames: stopping at the first NaN threw away exactly the frames the tracker kept, and on a beating
	// mode measured the beat's dip instead of the decay. Nothing is interpolated. It still stops at the real endings:
	// modeDecayFloorDB below the peak (noise floor) and the last frame.
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

// A track that split across a bin boundary must not count twice.
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

// Ascending Hz.
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

// The anchor itself is 1.0 when it qualifies.
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

func RequiresModes(b *Bundle) bool {
	return RequiresAudible(b) && len(b.Modes().Strong(modeStrongWithin)) >= 2
}
