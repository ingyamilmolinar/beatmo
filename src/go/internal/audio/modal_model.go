package audio

import (
	"math"
	"strconv"
)

// A line-for-line mirror of src/c/modal_bank.c, for the Synth tab's mode graph and the WAV-clone inversion: change one,
// change both.

// In HEARD time, after a non-melodic voice's playback resample.
type ModalMode struct {
	Row                     int
	Freq, Alpha, Amp, Phase float64
}

// 1/√(1 + (π (f·tc)³)²): the raised-cosine contact spectrum's smooth envelope.
func ModalStrikeSpectrum(f, tc float64) float64 {
	x := f * tc
	r := math.Pi * x * x * x
	return 1 / math.Sqrt(1+r*r)
}

// A melodic voice renders AT the pitch (k = 1); anything else at its own core pitch, resampled by 2^(pitch/12).
func ModalVoice(p RecipeParams, pitch float64, melodic bool) (voiceHz, k float64) {
	st := p["pitch"]
	k = 1
	if melodic {
		st = pitch
	} else {
		k = math.Pow(2, pitch/12)
	}
	voiceHz = modularBaseFreqHz * math.Pow(2, p["osc_octave"]) * math.Pow(2, st/12) * math.Pow(2, p["osc_detune"]/1200)
	if voiceHz < 1 {
		voiceHz = 1
	}
	if v := p["voice_freq_hz"]; v > 0 {
		voiceHz = v
	}
	return voiceHz, k
}

// modularBaseFreqHz is MODULAR_BASE_FREQ (src/c/modular.c).
const modularBaseFreqHz = 220.0

// Rows the C skips (non-positive, at or past the limit, past Keep) are absent; nil when the stage is off.
func ModalModes(p RecipeParams, pitch float64, melodic bool, rate int) []ModalMode {
	if math.Round(p["modal_enabled"]) == 0 {
		return nil
	}
	voiceHz, k := ModalVoice(p, pitch, melodic)
	ratio := math.Round(p["modal_freq_mode"]) == 0
	ref := 1000.0
	if ratio {
		ref = voiceHz
	}
	limit := math.Min(20000, 0.45*float64(rate))
	tune := math.Pow(2, p["modal_tune"]/1200)
	strike := math.Round(p["strike_enabled"]) != 0
	clampRows := func(v float64) int { return min(128, max(0, int(math.Round(v)))) }
	count := min(clampRows(p["modal_count"]), clampRows(p["modal_keep"]))
	var out []ModalMode
	for i := 0; i < count; i++ {
		cell := func(col string) float64 { return p[col+"["+strconv.Itoa(i)+"]"] }
		f := cell("modal_freq")
		if ratio {
			f *= voiceHz
		}
		if !(f > 0) {
			continue
		}
		if p["modal_stretch"] != 0 {
			f *= math.Pow(f/ref, p["modal_stretch"])
		}
		f *= tune
		if !(f < limit) {
			continue
		}
		law := p["modal_law_a"]*math.Pow(f/1000, p["modal_law_p"]) + p["modal_law_b"]*math.Pow(f/10000, 6)
		alpha := math.Max(0, cell("modal_decay")*p["modal_decay_x"]*law)
		oct := math.Log2(f / 1000)
		a := cell("modal_amp")
		if p["modal_bright"] != 0 {
			a *= math.Pow(10, p["modal_bright"]*oct/20)
		}
		if strike {
			a *= ModalStrikeSpectrum(f, p["strike_tc"]) * math.Pow(10, p["strike_hardness"]*oct/20)
		}
		out = append(out, ModalMode{Row: i, Freq: f * k, Alpha: alpha * k, Amp: a, Phase: cell("modal_phase")})
	}
	return out
}
