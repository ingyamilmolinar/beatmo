package clone

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// invert.go — Analysis → instrument document (spec §5a). The document holds
// MEASURED MODEL NUMBERS on the plan-2 stages and nothing else: mode rows
// stored relative to the models the C evaluates live (amplitudes ÷ the fitted
// strike, decays ÷ the fitted law), the fitted strike and law as the stages'
// absolute values, residual noise-band rows. At the fit pitch it reproduces
// the measurement exactly (TestInvertReproducesTheMeasuredModes).

// InvertOptions shapes the candidate document.
type InvertOptions struct {
	ID             instrument.ID
	Name, Category string
	// Into keeps an existing instrument's identity (id, name, category,
	// notes, render length) and replaces its sound.
	Into     *instrument.Doc
	MaxModes int     // default 128 (the table capacity)
	FloorDB  float64 // drop modes this far below the strongest's energy; default −60
}

// InvertReport says what Invert kept, dropped and clamped.
type InvertReport struct {
	Kept      int      `json:"kept"`
	Dropped   int      `json:"dropped"`
	Clamped   int      `json:"clamped"`
	NoiseRows int      `json:"noise_rows"`
	FitPitch  float64  `json:"fit_pitch"`
	Melodic   bool     `json:"melodic"`
	Notes     []string `json:"notes"`
}

const (
	modalCapacity     = 128
	noiseCapacity     = 32
	doubletRel        = 0.005
	minRenderSec      = 0.25
	maxRenderSec      = 10.0
	rowAmpBound       = 1000.0
	rowDecayBound     = 10000.0
	noiseRateBound    = 5000.0
	noiseLevelBoundHi = 24.0
)

// Knob bounds of the fitted values (modular_recipe.go cloneBounds).
var (
	lawABounds  = [2]float64{0, 1000}
	lawPBounds  = [2]float64{-2, 4}
	lawBBounds  = [2]float64{0, 100000}
	tcBounds    = [2]float64{0, 0.05}
	hardBounds  = [2]float64{-24, 24}
	noiseQBound = [2]float64{0.1, 100}
	// noiseCenterBounds / modeFreqHzMax are the table columns' bounds
	// (module_decls.go).
	noiseCenterBounds = [2]float64{10, 24000}
	modeFreqHzMax     = 24000.0
)

// RenderPitch is the node pitch a reference is fitted and auditioned at
// (spec §5b): a pitched reference sounds exactly its nominal f0
// (220·2^(p/12) = f0); an unpitched one its catalogue note when known.
func (p PitchInfo) RenderPitch() float64 {
	if p.Pitched && p.F0Hz > 0 {
		return 12 * math.Log2(p.F0Hz/modularBaseFreq)
	}
	if p.HasSemis {
		return float64(p.SemisFromA3)
	}
	return 0
}

// Invert builds the candidate document.
func Invert(a Analysis, opt InvertOptions) (instrument.Doc, InvertReport, error) {
	if opt.MaxModes <= 0 || opt.MaxModes > modalCapacity {
		opt.MaxModes = modalCapacity
	}
	if opt.FloorDB == 0 {
		opt.FloorDB = -60
	}
	rep := InvertReport{FitPitch: a.Pitch.RenderPitch(), Melodic: a.Pitch.Pitched && a.Pitch.F0Hz > 0, Notes: []string{}}
	if len(a.Modes) == 0 {
		return instrument.Doc{}, rep, fmt.Errorf("clone: no modes to invert")
	}

	// A percussion voice renders at pitch 0 and is resampled by k at the
	// fit pitch, so the model is stored in the pitch-0 domain.
	k := 1.0
	if !rep.Melodic {
		k = math.Pow(2, rep.FitPitch/12)
	}
	modes, dropped := selectModes(a.Modes, opt.MaxModes, opt.FloorDB)
	inRange := modes[:0]
	for _, m := range modes {
		m.Freq /= k
		m.Alpha /= k
		if m.Freq < modeFreqHzMax {
			inRange = append(inRange, m)
		}
	}
	dropped += len(modes) - len(inRange)
	modes = inRange
	rep.Kept, rep.Dropped = len(modes), dropped

	law := FitDecayLaw(modes)
	law.A = clamp(law.A, lawABounds)
	law.P = clamp(law.P, lawPBounds)
	law.B = clamp(law.B, lawBBounds)
	for _, m := range modes {
		if !(law.Eval(m.Freq) > 1e-12) {
			law = flatLaw(modes)
			rep.Notes = append(rep.Notes, "decay law vanished at a kept mode; stored a flat law")
			break
		}
	}
	tc := clamp(a.Strike.TcSec*k, tcBounds)
	hard := clamp(a.Strike.HardnessDBPerOct, hardBounds)

	reg := audio.BuiltinModuleRegistry()
	d := instrument.Doc{ID: opt.ID, Name: opt.Name, Category: opt.Category, Stages: map[string]instrument.Stage{}}
	if d.Name == "" {
		d.Name = string(opt.ID)
	}
	if d.Category == "" {
		d.Category = "percussion"
	}
	var err error
	for _, typ := range []string{"core", "finish", "modal", "strike", "noiseband"} {
		if d, err = instrument.AddStage(d, reg, typ, true); err != nil {
			return instrument.Doc{}, rep, err
		}
	}
	mv := d.Stages["modal"].Values
	mv["modal_freq_mode"] = 1
	if rep.Melodic {
		mv["modal_freq_mode"] = 0
	}
	mv["modal_law_a"], mv["modal_law_p"], mv["modal_law_b"] = law.A, law.P, law.B
	mv["modal_keep"] = float64(len(modes))
	sv := d.Stages["strike"].Values
	sv["strike_tc"], sv["strike_hardness"], sv["strike_click"] = tc, hard, 0

	mt := instrument.Table{Cols: []string{"freq", "decay", "amp", "phase", "attack"}}
	for _, m := range modes {
		f := m.Freq
		if rep.Melodic {
			f = m.Freq / a.Pitch.F0Hz
		}
		r := m.Alpha / law.Eval(m.Freq)
		amp := m.Amp / (StrikeSpectrum(m.Freq, tc) * HardnessGain(m.Freq, hard))
		if math.IsNaN(amp) || math.Abs(amp) > rowAmpBound || r > rowDecayBound {
			rep.Clamped++
			amp = math.Copysign(math.Min(math.Abs(amp), rowAmpBound), m.Amp)
			if math.IsNaN(amp) {
				amp = 0
			}
			r = math.Min(r, rowDecayBound)
		}
		mt.Rows = append(mt.Rows, []float64{f, r, amp, wrapPhase(m.Phase), 0})
	}
	if rep.Clamped > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf("%d mode rows clamped (amplitude far down the strike roll-off, or decay off the law)", rep.Clamped))
	}
	if d, err = instrument.SetTable(d, reg, "modal", mt); err != nil {
		return instrument.Doc{}, rep, err
	}

	nt := instrument.Table{Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"}}
	var bands []NoiseBand
	for _, b := range activeBands(a.Residual.Bands) {
		// Pre-scaled for a resample, a band can leave the column's range (a
		// conga at +22 st put the 31.5 Hz band at 8.8 Hz): it cannot be stored.
		if c := b.CenterHz / k; c >= noiseCenterBounds[0] && c <= noiseCenterBounds[1] {
			bands = append(bands, b)
		}
	}
	for _, b := range bands {
		nt.Rows = append(nt.Rows, []float64{
			b.CenterHz / k, clamp(b.Q, noiseQBound), math.Min(b.LevelDB, noiseLevelBoundHi),
			math.Min(b.AttackSec*k, 1),
			math.Min(b.FastRate/k, noiseRateBound), clamp(b.FastMix, [2]float64{0, 1}), math.Min(b.TailRate/k, noiseRateBound),
		})
	}
	rep.NoiseRows = len(nt.Rows)
	if len(nt.Rows) == 0 {
		st := d.Stages["noiseband"]
		st.On = false
		d.Stages["noiseband"] = st
	} else if d, err = instrument.SetTable(d, reg, "noiseband", nt); err != nil {
		return instrument.Doc{}, rep, err
	}

	sec := renderSeconds(modes, bands, k, math.Pow(10, a.FloorDB/20))
	d.Render = instrument.Render{Seconds: sec, Amplitude: 1, Melodic: rep.Melodic}
	if opt.Into != nil {
		d.ID, d.Name, d.Category, d.Notes = opt.Into.ID, opt.Into.Name, opt.Into.Category, opt.Into.Notes
		d.Render.Seconds = opt.Into.Render.Seconds
	}
	if err := instrument.Validate(d, reg); err != nil {
		return instrument.Doc{}, rep, err
	}
	return d, rep, nil
}

// selectModes ranks by energy, drops modes floorDB under the strongest, and
// keeps at most maxModes without splitting a doublet at the cut.
func selectModes(in []Mode, maxModes int, floorDB float64) ([]Mode, int) {
	ms := append([]Mode(nil), in...)
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Energy() > ms[j].Energy() })
	top := ms[0].Energy()
	n := 0
	for n < len(ms) && 10*math.Log10(ms[n].Energy()/top) >= floorDB {
		n++
	}
	if n > maxModes {
		n = maxModes
		for n > 0 && hasPartnerAfter(ms, n-1, n) {
			n--
		}
	}
	return ms[:n], len(in) - n
}

// hasPartnerAfter reports whether ms[i] has a doublet partner at index ≥ cut.
func hasPartnerAfter(ms []Mode, i, cut int) bool {
	for j := cut; j < len(ms); j++ {
		if math.Abs(ms[j].Freq/ms[i].Freq-1) <= doubletRel {
			return true
		}
	}
	return false
}

func flatLaw(modes []Mode) DecayLaw {
	var a, w float64
	for _, m := range modes {
		a += m.Energy() * m.Alpha
		w += m.Energy()
	}
	return DecayLaw{A: clamp(a/w, lawABounds)}
}

func activeBands(in []NoiseBand) []NoiseBand {
	var out []NoiseBand
	for _, b := range in {
		if b.Active && b.LevelDB > -200 && b.CenterHz > 10 {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LevelDB > out[j].LevelDB })
	if len(out) > noiseCapacity {
		out = out[:noiseCapacity]
	}
	return out
}

// renderSeconds is the time the slowest audible part of the model (pitch-0
// domain) takes to reach the reference's floor.
func renderSeconds(modes []Mode, bands []NoiseBand, k, floor float64) float64 {
	floor = math.Max(floor, 1e-6)
	t := 0.0
	for _, m := range modes {
		if a := math.Abs(m.Amp); a > floor && m.Alpha > 0 {
			t = math.Max(t, math.Log(a/floor)/m.Alpha)
		}
	}
	for _, b := range bands {
		if a := math.Pow(10, b.LevelDB/20); a > floor && b.TailRate > 0 {
			t = math.Max(t, b.AttackSec*k+math.Log(a/floor)/(b.TailRate/k))
		}
	}
	return clamp(t, [2]float64{minRenderSec, maxRenderSec})
}

func wrapPhase(p float64) float64 { return math.Remainder(p, 2*math.Pi) }

func clamp(v float64, b [2]float64) float64 { return math.Min(math.Max(v, b[0]), b[1]) }

// DocSHA identifies a candidate for the listening gate: the content hash with
// render.amplitude zeroed — `make measure-loudness` rewrites that field after
// adopt and must not invalidate the verdict (spec §6a).
func DocSHA(d instrument.Doc) string {
	d.Render.Amplitude = 0
	return instrument.ContentHash(d)
}
