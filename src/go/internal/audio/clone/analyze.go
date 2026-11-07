package clone

import (
	"fmt"
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// AnalysisFormat identifies analysis.json (a local dev artifact, never shipped).
const AnalysisFormat = "beatmo.clone.analysis/1"

// MissedMode is a strong STFT-tracked partial with no ESPRIT mode nearby.
type MissedMode struct {
	Hz     float64 `json:"hz"`
	PeakDB float64 `json:"peak_db"`
}

// Timings are wall-clock milliseconds per stage (measured, never assumed).
type Timings struct {
	ModesMs      float64 `json:"modes_ms"`
	StrikeMs     float64 `json:"strike_ms"`
	DecayLawMs   float64 `json:"decay_law_ms"`
	ResidualMs   float64 `json:"residual_ms"`
	CrossCheckMs float64 `json:"cross_check_ms"`
	TotalMs      float64 `json:"total_ms"`
}

// Analysis is everything measured about one reference (spec §2f).
type Analysis struct {
	Format         string       `json:"format"`
	Label          string       `json:"label"`
	Rate           int          `json:"rate"`
	OnsetSec       float64      `json:"onset_s"`
	FloorDB        float64      `json:"floor_db"`
	PeakDB         float64      `json:"peak_db"`
	RMS400DB       float64      `json:"rms400_db"`
	WindowStartSec float64      `json:"window_start_s"` // from the onset
	WindowEndSec   float64      `json:"window_end_s"`
	Pitch          PitchInfo    `json:"pitch"`
	Modes          []Mode       `json:"modes"`
	Strike         Strike       `json:"strike"`
	DecayLaw       DecayLaw     `json:"decay_law"`
	Residual       Residual     `json:"residual"`
	Bands          []BandDiag   `json:"bands"`
	Missed         []MissedMode `json:"missed"`
	GlideCents     float64      `json:"glide_cents"`
	TransientModes int          `json:"transient_modes"` // poles dropped as onset transient (left to the residual)
	Notes          []string     `json:"notes"`
	Timings        Timings      `json:"timings"`
}

const (
	maxResidualFraction = 0.5
	glideWarnCents      = 10
	missedRangeDB       = 40
)

// A not_modal refusal still returns the filled Analysis: partial results are always written.
func Analyze(p Prepared) (Analysis, error) {
	t0 := time.Now()
	rate := float64(p.Rate)
	a := Analysis{
		Format: AnalysisFormat, Label: p.Label, Rate: p.Rate,
		OnsetSec: float64(p.Onset) / rate, FloorDB: dB(p.FloorRMS), PeakDB: dB(p.Peak), RMS400DB: p.RMS400DB,
		WindowStartSec: float64(p.WindowStart-p.Onset) / rate, WindowEndSec: float64(p.WindowEnd-p.Onset) / rate,
		Pitch: p.Pitch, Notes: []string{}, Missed: []MissedMode{},
	}
	ms := func(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

	t := time.Now()
	mr, err := EstimateModes(p)
	if err != nil {
		return a, err
	}
	a.Modes, a.Bands, a.TransientModes = mr.Modes, mr.Bands, mr.Transient
	a.Timings.ModesMs = ms(t)

	t = time.Now()
	a.Strike = EstimateStrike(a.Modes)
	a.Timings.StrikeMs = ms(t)

	t = time.Now()
	a.DecayLaw = FitDecayLaw(a.Modes)
	a.Timings.DecayLawMs = ms(t)

	t = time.Now()
	a.Residual = MeasureResidual(p, a.Modes, math.Max(a.Strike.TcSec, 0.002))
	a.Strike.ClickLevelDB = a.Residual.ClickLevelDB
	a.Timings.ResidualMs = ms(t)

	t = time.Now()
	a.Missed = missedModes(p, a.Modes)
	if len(a.Modes) > 0 {
		a.GlideCents = glideCents(p, a.Modes[0])
		if math.Abs(a.GlideCents) > glideWarnCents {
			a.Notes = append(a.Notes, fmt.Sprintf(
				"pitch glide: the strongest mode (%.1f Hz) drifts %+.1f cents over the decay — a membrane? Fixed-frequency modes cannot express it (out of slice-1 scope)",
				a.Modes[0].Freq, a.GlideCents))
		}
	}
	a.Timings.CrossCheckMs = ms(t)
	a.Timings.TotalMs = ms(t0)

	if a.Residual.EnergyFraction > maxResidualFraction {
		return a, refuse(RefuseNotModal, "residual is %.0f%% of the energy after the modal fit — not a struck/modal sound",
			100*a.Residual.EnergyFraction)
	}
	return a, nil
}

// A tracked partial within 40 dB of the loudest with no ESPRIT mode within max(3 cents, one STFT bin): a 4096-point
// tracker places a partial only to about a bin.
func missedModes(p Prepared, modes []Mode) []MissedMode {
	out := []MissedMode{}
	w := wave.Wave{Samples: p.Samples[p.Onset:p.WindowEnd], SampleRate: p.Rate}
	st := fingerprint.NewBundle(w, p.Label).STFT(4096, 1024)
	set := fingerprint.AnalyzeModes(st)
	if len(set.Modes) == 0 {
		return out
	}
	loud := math.Inf(-1)
	for _, m := range set.Modes {
		loud = math.Max(loud, m.PeakDB)
	}
	bin := float64(p.Rate) / 4096
	for _, m := range set.Modes {
		if m.PeakDB < loud-missedRangeDB || m.Hz < bandLoHz {
			continue
		}
		tol := math.Max(3, 1200*math.Log2(1+bin/m.Hz))
		found := false
		for _, e := range modes {
			if math.Abs(1200*math.Log2(e.Freq/m.Hz)) <= tol {
				found = true
				break
			}
		}
		if !found {
			out = append(out, MissedMode{Hz: m.Hz, PeakDB: m.PeakDB})
		}
	}
	return out
}

// The strongest mode's drift in cents between the first and last third of the decay window.
func glideCents(p Prepared, m Mode) float64 {
	n := p.WindowEnd - p.WindowStart
	third := n / 3
	if third < 64 {
		return 0
	}
	f1, ok1 := peakNear(p.Samples[p.WindowStart:p.WindowStart+third], p.Rate, m.Freq)
	f2, ok2 := peakNear(p.Samples[p.WindowEnd-third:p.WindowEnd], p.Rate, m.Freq)
	if !ok1 || !ok2 {
		return 0
	}
	return 1200 * math.Log2(f2/f1)
}

// The largest power-of-two prefix: MagnitudeSpectrum windows the WHOLE padded buffer, which would leave the data
// under only the rising edge of the Hann window.
func peakNear(seg []float64, rate int, f float64) (float64, bool) {
	size := 1
	for size*2 <= len(seg) && size < 1<<18 {
		size <<= 1
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: seg[:size], SampleRate: rate}, size, wave.WindowHann)
	lo := max(1, int(f*0.97/binHz))
	hi := min(len(mag)-2, int(f*1.03/binHz)+1)
	if lo >= hi {
		return 0, false
	}
	k := lo
	for i := lo; i <= hi; i++ {
		if mag[i] > mag[k] {
			k = i
		}
	}
	a, b, c := dB(mag[k-1]), dB(mag[k]), dB(mag[k+1])
	off := 0.0
	if d := a - 2*b + c; d != 0 {
		off = 0.5 * (a - c) / d
	}
	return (float64(k) + off) * binHz, true
}
