package soundview

import "math"

// Envelope is loudness over time: per 5 ms hop, the RMS over a 20 ms window
// centred on the hop (RMSdB) and the sample peak inside the hop (PeakdB),
// both in dBFS, clamped to FloorDB. Hop i is centred at (i+0.5)*HopSec.
type Envelope struct {
	HopSec float64
	RMSdB  []float64
	PeakdB []float64
}

const (
	envHopSec = 0.005
	envWinSec = 0.02
)

// Envelope returns loudness over time.
func (a *Analysis) Envelope() Envelope {
	a.envOnce.Do(a.computeEnvelope)
	return a.env
}

// PeakDB is the note's sample peak in dBFS.
func (a *Analysis) PeakDB() float64 {
	a.envOnce.Do(a.computeEnvelope)
	return a.peakDB
}

// AudibleSec is how long the note sounds: the centre of the last hop whose
// RMS is within AudibleDropDB of the loudest hop. 0 for silence.
func (a *Analysis) AudibleSec() float64 {
	a.envOnce.Do(a.computeEnvelope)
	return a.audible
}

func (a *Analysis) computeEnvelope() {
	defer a.markDone(NeedEnvelope)
	hopN := max(1, int(math.Round(float64(a.sr)*envHopSec)))
	winN := max(1, int(math.Round(float64(a.sr)*envWinSec)))
	n := len(a.pcm)
	frames := (n + hopN - 1) / hopN
	a.env = Envelope{HopSec: float64(hopN) / float64(a.sr), RMSdB: make([]float64, frames), PeakdB: make([]float64, frames)}
	a.peakDB = FloorDB
	// Prefix sums of x² make every window O(1).
	sq := make([]float64, n+1)
	for i, s := range a.pcm {
		v := float64(s)
		sq[i+1] = sq[i] + v*v
	}
	maxRMS := FloorDB
	for f := 0; f < frames; f++ {
		a.yieldEvery(f, 64)
		lo, hi := f*hopN, min(n, (f+1)*hopN)
		pk := 0.0
		for _, s := range a.pcm[lo:hi] {
			pk = math.Max(pk, math.Abs(float64(s)))
		}
		c := lo + hopN/2
		wlo, whi := max(0, c-winN/2), min(n, c+winN/2)
		rms := 0.0
		if whi > wlo {
			rms = math.Sqrt((sq[whi] - sq[wlo]) / float64(whi-wlo))
		}
		a.env.PeakdB[f] = toDB(pk)
		a.env.RMSdB[f] = toDB(rms)
		a.peakDB = math.Max(a.peakDB, a.env.PeakdB[f])
		maxRMS = math.Max(maxRMS, a.env.RMSdB[f])
	}
	a.audible = 0
	if maxRMS <= FloorDB+1 {
		return
	}
	for f := frames - 1; f >= 0; f-- {
		if a.env.RMSdB[f] >= maxRMS-AudibleDropDB {
			a.audible = (float64(f) + 0.5) * a.env.HopSec
			return
		}
	}
}

// RMSAt returns the envelope's RMS dB at time t (seconds; clamped).
func (e Envelope) RMSAt(t float64) float64 {
	if len(e.RMSdB) == 0 || e.HopSec <= 0 {
		return FloorDB
	}
	i := int(math.Round(t/e.HopSec - 0.5))
	i = max(0, min(len(e.RMSdB)-1, i))
	return e.RMSdB[i]
}

// MaxRMS returns the loudest hop's RMS dB.
func (e Envelope) MaxRMS() float64 {
	m := FloorDB
	for _, v := range e.RMSdB {
		m = math.Max(m, v)
	}
	return m
}

// --- onsets + attack (1 ms resolution) ---

const (
	blockSec       = 0.001
	holdBlocks     = 10    // 10 ms peak-hold: catches every cycle above 100 Hz
	onsetFloorDB   = -30.0 // a hit quieter than peak-30 dB is not an onset (float: -30/20 would be integer division)
	onsetRise      = 4.0   // x4 (12 dB) above the previous 3..30 ms
	onsetSpacing   = 0.03  // two onsets are at least 30 ms apart
	attackWithinDB = 1.0   // attack ends within 1 dB of the loudest moment
	attackRMSSec   = 0.02  // the loudness the attack is judged by: 20 ms RMS (the envelope's window)
)

// Peak level (largest |sample| within ±7 ms) every 1 ms from the first onset: unlike RMS it does not ripple with a low
// note's cycle, and a click shows as the jump it is.
type OnsetCurve struct {
	StepSec float64
	DB      []float64
}

const (
	onsetCurveSec  = 0.3
	onsetStepSec   = 0.001
	onsetHoldSec   = 0.007
	clickHeadSec   = 0.010 // the click: the loudest moment of the first 10 ms …
	clickBodyFrom  = 0.030 // … over the body's level 30 …
	clickBodyUntil = 0.080 // … to 80 ms after the onset
)

// OnsetCurve returns the first moment at 1 ms resolution.
func (a *Analysis) OnsetCurve() OnsetCurve {
	a.atkOnce.Do(a.computeAttack)
	return a.onset
}

// The loudest of the first 10 ms over the body 30–80 ms later: a click, a stick, a pluck's transient.
func (a *Analysis) ClickDB() float64 {
	a.atkOnce.Do(a.computeAttack)
	return a.clickDB
}

// computeOnsetCurve fills the onset curve and the click from the first onset.
func (a *Analysis) computeOnsetCurve() {
	a.onset = OnsetCurve{StepSec: onsetStepSec}
	if len(a.onsets) == 0 {
		return
	}
	n := len(a.pcm)
	st := int(a.onsets[0] * float64(a.sr))
	step := max(1, int(math.Round(onsetStepSec*float64(a.sr))))
	hold := max(1, int(math.Round(onsetHoldSec*float64(a.sr))))
	steps := int(onsetCurveSec / onsetStepSec)
	for k := 0; k < steps; k++ {
		c := st + k*step
		if c >= n {
			break
		}
		pk := 0.0
		for i := max(st, c-hold); i < min(n, c+hold); i++ {
			pk = math.Max(pk, math.Abs(float64(a.pcm[i])))
		}
		a.onset.DB = append(a.onset.DB, toDB(pk))
	}
	// Loudest against loudest: both stretches are judged by their peaks.
	head, body, seen := FloorDB, FloorDB, false
	for k, db := range a.onset.DB {
		t := float64(k) * onsetStepSec
		switch {
		case t < clickHeadSec:
			head = math.Max(head, db)
		case t >= clickBodyFrom && t < clickBodyUntil:
			body, seen = math.Max(body, db), true
		}
	}
	if !seen {
		return
	}
	a.clickDB = math.Max(0, head-body)
}

// Onsets returns the start times (seconds) of each hit: the first where the
// note becomes audible (30 dB under its peak), later ones where the level
// jumps 12 dB over the preceding 3..30 ms (a flam, a burst hit).
func (a *Analysis) Onsets() []float64 {
	a.atkOnce.Do(a.computeAttack)
	return a.onsets
}

// AttackSec is the time from the first onset until the note's 20 ms RMS is
// within 1 dB of its loudest. 0 for silence.
func (a *Analysis) AttackSec() float64 {
	a.atkOnce.Do(a.computeAttack)
	return a.attack
}

func (a *Analysis) computeAttack() {
	defer a.markDone(NeedOnsets)
	bn := max(1, int(math.Round(float64(a.sr)*blockSec)))
	nb := (len(a.pcm) + bn - 1) / bn
	blk := make([]float64, nb)
	peak := 0.0
	for b := range blk {
		lo, hi := b*bn, min(len(a.pcm), (b+1)*bn)
		for _, s := range a.pcm[lo:hi] {
			blk[b] = math.Max(blk[b], math.Abs(float64(s)))
		}
		peak = math.Max(peak, blk[b])
	}
	if peak < 1e-6 {
		return
	}
	th := peak * math.Pow(10, onsetFloorDB/20)
	last := math.Inf(-1)
	for b := range blk {
		a.yieldEvery(b, 400)
		if blk[b] < th {
			continue
		}
		hist := 0.0
		for k := max(0, b-30); k <= b-3; k++ {
			hist = math.Max(hist, blk[k])
		}
		t := float64(b*bn) / float64(a.sr)
		// The FIRST onset is where the note becomes audible, however slowly it
		// rises (a swell never jumps 12 dB); later ones must jump.
		first := len(a.onsets) == 0
		if (first || blk[b] >= onsetRise*hist) && t-last >= onsetSpacing {
			a.onsets = append(a.onsets, t)
			last = t
		}
	}
	// Ends when the note is LOUD (within 1 dB of its loudest 10 ms RMS), not at its peak SAMPLE: a peak-normalised bow or
	// reed note can open with its loudest transient and keep swelling.
	hold := make([]float64, nb)
	for b := range blk {
		a.yieldEvery(b, 400)
		for k := max(0, b-holdBlocks+1); k <= b; k++ {
			hold[b] = math.Max(hold[b], blk[k])
		}
	}
	a.hold = hold
	if len(a.onsets) == 0 {
		return
	}
	n := len(a.pcm)
	sq := make([]float64, n+1)
	for i, s := range a.pcm {
		v := float64(s)
		sq[i+1] = sq[i] + v*v
	}
	a.computeOnsetCurve()
	win := max(1, int(math.Round(float64(a.sr)*attackRMSSec)))
	rms := make([]float64, nb)
	maxRMS := 0.0
	for b := range rms {
		c := b*bn + bn/2
		lo, hi := max(0, c-win/2), min(n, c+win/2)
		if hi > lo {
			rms[b] = math.Sqrt((sq[hi] - sq[lo]) / float64(hi-lo))
		}
		maxRMS = math.Max(maxRMS, rms[b])
	}
	near := maxRMS * math.Pow(10, -attackWithinDB/20)
	start := int(math.Round(a.onsets[0] * float64(a.sr) / float64(bn)))
	for b := start; b < nb; b++ {
		if rms[b] >= near {
			frac := 1.0
			if b > 0 && rms[b] > rms[b-1] {
				frac = (near - rms[b-1]) / (rms[b] - rms[b-1])
			}
			// Block b's RMS is centred at its middle.
			at := (float64(b) - 0.5 + frac) * float64(bn) / float64(a.sr)
			a.attack = math.Max(0, at-a.onsets[0])
			return
		}
	}
}
