package ui

import (
	"image"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

const (
	eqCurveMinDB   = -12.0
	eqCurveMaxDB   = 12.0
	eqCurvePoints  = 200
	eqHandleRadius = 10
	// eqGuideDB is the ± dB value at which the EQ plot draws faint guide
	// rulers with end labels, so the otherwise-unlabeled handle field
	// carries a visible dB scale.
	eqGuideDB = 6.0
)

// eqGuideLabelScale sizes the ±6 dB end labels (caption-sized, derived from
// the font tokens so it tracks any future font change).
var eqGuideLabelScale = FontSizeCaption / FontSizeBody

// freqToX maps a frequency (Hz) to an X pixel within eqRect using log scale.
func freqToX(hz float64, r image.Rectangle) int {
	if hz <= 0 {
		return r.Min.X
	}
	logMin := math.Log10(20)
	logMax := math.Log10(20000)
	t := (math.Log10(hz) - logMin) / (logMax - logMin)
	return r.Min.X + int(t*float64(r.Dx()))
}

// xToFreq maps an X pixel within eqRect to a frequency (Hz) using inverse log scale.
func xToFreq(x int, r image.Rectangle) float64 {
	if r.Dx() <= 0 {
		return 20
	}
	t := float64(x-r.Min.X) / float64(r.Dx())
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	logMin := math.Log10(20)
	logMax := math.Log10(20000)
	return math.Pow(10, logMin+t*(logMax-logMin))
}

// gainDBToY maps a dB value to a Y pixel within eqRect.
// +12 dB at top, -12 dB at bottom, 0 dB at center.
func gainDBToY(db float64, r image.Rectangle) int {
	t := (eqCurveMaxDB - db) / (eqCurveMaxDB - eqCurveMinDB)
	return r.Min.Y + int(t*float64(r.Dy()))
}

// yToGainDB maps a Y pixel to dB, inverse of gainDBToY.
func yToGainDB(y int, r image.Rectangle) float64 {
	t := float64(y-r.Min.Y) / float64(r.Dy())
	db := eqCurveMaxDB - t*(eqCurveMaxDB-eqCurveMinDB)
	if db < eqCurveMinDB {
		db = eqCurveMinDB
	}
	if db > eqCurveMaxDB {
		db = eqCurveMaxDB
	}
	return math.Round(db*10) / 10
}

// eqCurveEnsure recomputes the cached frequency response if dirty.
func (dv *DrumView) eqCurveEnsure() {
	if !dv.eqCurveDirty && len(dv.eqCurveCache) > 0 {
		return
	}
	ch := dv.activeEQChannel()
	var gains []float64
	var muted []bool
	if ch == "main" {
		gains = dv.eqBandGainsDB()
		muted = dv.eqBandMuted()
	} else {
		for _, r := range dv.Rows {
			if r.Instrument == ch {
				gains = r.EQGainsDB
				muted = r.EQBandMuted
				break
			}
		}
	}
	bands := dv.buildFullEQBands(gains, muted,
		dv.activeHPFEnabled(), dv.activeHPFCutoffHz(),
		dv.activeLPFEnabled(), dv.activeLPFCutoffHz())
	dv.eqCurveCache = audio.ComputeFreqResponse(audio.SampleRate(), bands, eqCurvePoints, 20, 20000)
	dv.eqCurveDirty = false
}

// curveYAtX returns the Y pixel on the cached frequency response curve closest
// to the given X pixel. Falls back to the 0 dB line.
func (dv *DrumView) curveYAtX(px int, r image.Rectangle) int {
	dv.eqCurveEnsure()
	best := gainDBToY(0, r)
	bestDist := r.Dx() + 1
	for _, p := range dv.eqCurveCache {
		cx := freqToX(p.FreqHz, r)
		d := cx - px
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = gainDBToY(p.GainDB, r)
		}
	}
	if best < r.Min.Y+eqHandleRadius {
		best = r.Min.Y + eqHandleRadius
	}
	if best > r.Max.Y-eqHandleRadius {
		best = r.Max.Y - eqHandleRadius
	}
	return best
}

// handleEQCurveDrag processes mouse input for EQ curve handle dragging.
// Returns true if the input was consumed.
func (dv *DrumView) handleEQCurveDrag(mx, my int, pressed bool) bool {
	r := dv.eqRect
	if r.Empty() || dv.eqWaveformMode {
		return false
	}

	// Handle active filter (HPF/LPF) drag.
	if dv.eqCurveDragFilter != "" {
		if !pressed {
			dv.applyEQ()
			dv.eqCurveDragFilter = ""
			return true
		}
		freq := xToFreq(mx, r)
		if dv.eqCurveDragFilter == "hpf" {
			if freq < 20 {
				freq = 20
			}
			if freq > 2000 {
				freq = 2000
			}
			dv.setActiveHPF(true, freq)
		} else {
			if freq < 1000 {
				freq = 1000
			}
			if freq > 20000 {
				freq = 20000
			}
			dv.setActiveLPF(true, freq)
		}
		dv.eqCurveDirty = true
		return true
	}

	if dv.eqCurveDragBand >= 0 {
		// Currently dragging a peaking band handle.
		if !pressed {
			// Released — commit the change.
			dv.applyEQ()
			dv.eqCurveDragBand = -1
			return true
		}
		// Update the gain for the dragged band.
		db := yToGainDB(my, r)
		band := dv.eqCurveDragBand
		ch := dv.activeEQChannel()
		if ch == "main" {
			if band < len(dv.eqBandGainsDB()) {
				dv.eqBandGainsDB()[band] = db
			}
		} else {
			for _, row := range dv.Rows {
				if row.Instrument == ch {
					dv.ensureRowEQ(dv.rowIndex(row))
					if band < len(row.EQGainsDB) {
						row.EQGainsDB[band] = db
					}
					break
				}
			}
		}
		dv.eqCurveDirty = true
		return true
	}

	if !pressed {
		return false
	}

	// Hit-test handles.
	pt := image.Pt(mx, my)
	if !pt.In(r) {
		return false
	}

	hitRadius := Profile().EQHandleRadius

	// Hit-test HPF handle first.
	if dv.activeHPFEnabled() {
		hpfHz := dv.activeHPFCutoffHz()
		hx := freqToX(hpfHz, r)
		hy := dv.curveYAtX(hx, r)
		dx := mx - hx
		dy := my - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			dv.eqCurveDragFilter = "hpf"
			return true
		}
	}

	// Hit-test LPF handle.
	if dv.activeLPFEnabled() {
		lpfHz := dv.activeLPFCutoffHz()
		lx := freqToX(lpfHz, r)
		ly := dv.curveYAtX(lx, r)
		dx := mx - lx
		dy := my - ly
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			dv.eqCurveDragFilter = "lpf"
			return true
		}
	}

	// Hit-test peaking band handles.
	ch := dv.activeEQChannel()
	var gains []float64
	if ch == "main" {
		gains = dv.eqBandGainsDB()
	} else {
		for _, row := range dv.Rows {
			if row.Instrument == ch {
				gains = row.EQGainsDB
				break
			}
		}
	}

	for i, def := range eqBandDefs {
		center := math.Sqrt(def.loHz * def.hiHz)
		hx := freqToX(center, r)
		gain := 0.0
		if i < len(gains) {
			gain = gains[i]
		}
		hy := gainDBToY(gain, r)
		dx := mx - hx
		dy := my - hy
		if dx*dx+dy*dy <= hitRadius*hitRadius {
			dv.eqCurveDragBand = i
			return true
		}
	}
	return false
}

// rowIndex returns the index of the given row, or -1 if not found.
func (dv *DrumView) rowIndex(row *DrumRow) int {
	for i, r := range dv.Rows {
		if r == row {
			return i
		}
	}
	return -1
}
