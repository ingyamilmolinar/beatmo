package clone

import (
	"fmt"
	"math"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Markdown renders the human report (spec §6a).
func (a Analysis) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Clone analysis — %s\n\n", a.Label)
	fmt.Fprintf(&b, "- rate %d Hz · onset %.3f s · floor %.1f dB · peak %.1f dB · RMS(400 ms) %.1f dB\n",
		a.Rate, a.OnsetSec, a.FloorDB, a.PeakDB, a.RMS400DB)
	fmt.Fprintf(&b, "- decay window %.3f → %.3f s after the onset\n", a.WindowStartSec, a.WindowEndSec)
	if a.Pitch.Pitched {
		fmt.Fprintf(&b, "- pitch %.2f Hz (%s, confidence %.2f)", a.Pitch.F0Hz, a.Pitch.Source, a.Pitch.Confidence)
	} else {
		fmt.Fprintf(&b, "- unpitched (modes in absolute Hz; confidence %.2f)", a.Pitch.Confidence)
	}
	if a.Pitch.HasSemis {
		fmt.Fprintf(&b, " · catalogue note %+d st from A3", a.Pitch.SemisFromA3)
	}
	b.WriteString("\n")
	for _, n := range a.Notes {
		fmt.Fprintf(&b, "- **note:** %s\n", n)
	}

	fmt.Fprintf(&b, "\n## Modes (%d, by energy)\n\n| # | Hz | ratio | cents from harmonic | T60 s | dB re strongest | SNR dB |\n|---|---|---|---|---|---|---|\n", len(a.Modes))
	var top float64
	for _, m := range a.Modes {
		top = math.Max(top, m.Amp)
	}
	for i, m := range a.Modes {
		ratio, cents := "—", "—"
		if a.Pitch.Pitched && a.Pitch.F0Hz > 0 {
			r := m.Freq / a.Pitch.F0Hz
			h := math.Max(1, math.Round(r))
			ratio = fmt.Sprintf("%.4f", r)
			cents = fmt.Sprintf("%+.1f", 1200*math.Log2(r/h))
		}
		fmt.Fprintf(&b, "| %d | %.1f | %s | %s | %.2f | %.1f | %.0f |\n",
			i+1, m.Freq, ratio, cents, m.T60(), dB(m.Amp)-dB(top), m.SNRdB)
	}
	if len(a.Missed) > 0 {
		b.WriteString("\n**Missed** (STFT partials with no ESPRIT mode nearby):")
		for _, m := range a.Missed {
			fmt.Fprintf(&b, " %.1f Hz (%.1f dB);", m.Hz, m.PeakDB)
		}
		b.WriteString("\n")
	}
	if a.TransientModes > 0 {
		fmt.Fprintf(&b, "\n%d fast / over-peak poles were dropped as onset transient (left to the residual).\n", a.TransientModes)
	}
	fmt.Fprintf(&b, "\n## Decay law\n\nα(f) = %.4g·(f/1 kHz)^%.2f + %.4g·(f/10 kHz)^6 · relative RMS error %.1f%%\n",
		a.DecayLaw.A, a.DecayLaw.P, a.DecayLaw.B, 100*a.DecayLaw.RelRMS)
	fmt.Fprintf(&b, "\n## Strike\n\nTc %.3f ms · hardness %+.2f dB/oct · click %.1f dB re reference · fit RMS %.2f dB\n",
		1000*a.Strike.TcSec, a.Strike.HardnessDBPerOct, a.Strike.ClickLevelDB, a.Strike.FitRMSdB)
	fmt.Fprintf(&b, "\n## Residual\n\n%.1f%% of the energy is not modal.\n\n| Hz | level dB | attack ms | fast /s | fast mix | tail /s |\n|---|---|---|---|---|---|\n",
		100*a.Residual.EnergyFraction)
	for _, nb := range a.Residual.Bands {
		if !nb.Active {
			continue
		}
		fmt.Fprintf(&b, "| %.0f | %.1f | %.1f | %.1f | %.2f | %.1f |\n",
			nb.CenterHz, nb.LevelDB, 1000*nb.AttackSec, nb.FastRate, nb.FastMix, nb.TailRate)
	}
	t := a.Timings
	fmt.Fprintf(&b, "\n## Timings (ms)\n\nmodes %.1f · strike %.1f · decay law %.1f · residual %.1f · cross-check %.1f · total %.1f\n",
		t.ModesMs, t.StrikeMs, t.DecayLawMs, t.ResidualMs, t.CrossCheckMs, t.TotalMs)
	return b.String()
}

type CandidateReport struct {
	DocID     instrument.ID `json:"doc_id"`
	DocSHA    string        `json:"doc_sha"`
	Amplitude float64       `json:"amplitude"`
	Invert    InvertReport  `json:"invert"`
	Refine    *RefineResult `json:"refine,omitempty"`
	Notes     []string      `json:"notes"`
	PrepareMs float64       `json:"prepare_ms"`
	InvertMs  float64       `json:"invert_ms"`
	TotalMs   float64       `json:"total_ms"`
}

// Markdown renders the candidate section appended to the analysis report.
func (c CandidateReport) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n## Candidate `%s`\n\n- document hash `%s` (render.amplitude excluded) · amplitude %.3f\n", c.DocID, c.DocSHA, c.Amplitude)
	iv := c.Invert
	mode := "unpitched (Hz rows, resampled like every drum)"
	if iv.Melodic {
		mode = "melodic (ratio rows)"
	}
	fmt.Fprintf(&b, "- %s · fit pitch %+.2f st · %d modes kept, %d dropped, %d clamped · %d noise rows\n",
		mode, iv.FitPitch, iv.Kept, iv.Dropped, iv.Clamped, iv.NoiseRows)
	for _, n := range append(append([]string(nil), iv.Notes...), c.Notes...) {
		fmt.Fprintf(&b, "- **note:** %s\n", n)
	}
	if r := c.Refine; r != nil {
		row := func(name string, l LossReport) {
			fmt.Fprintf(&b, "| %s | %.3f | %.2f | %.4f | %.2f | %.3f | %.2f | %.3f |\n",
				name, l.MRSTFT, l.LogMagDB, l.SC, l.Envelope, l.Descriptors, l.Prior, l.Total)
		}
		b.WriteString("\n## Loss\n\n| | MR-STFT | log-mag dB | SC | envelope dB | descriptors | prior dB | total |\n|---|---|---|---|---|---|---|---|\n")
		row("analysis only", r.Before)
		row("refined", r.After)
		verdict := "refined document kept"
		if r.Kept {
			verdict = "refine did not improve the loss — the analysis-only document is kept"
		}
		fmt.Fprintf(&b, "\n%s · %d dimensions · %d generations · %d renders · warm-up %.0f ms · refine %.0f ms\n",
			verdict, len(r.Dims), len(r.Gens), r.Evals, r.WarmupMs, r.TotalMs)
		if len(r.Gens) > 0 {
			b.WriteString("\n| gen | best | mean | ms |\n|---|---|---|---|\n")
			for i, g := range r.Gens {
				if i%10 == 0 || i == len(r.Gens)-1 {
					fmt.Fprintf(&b, "| %d | %.4f | %.4f | %.0f |\n", g.Gen, g.Best, g.Mean, g.Ms)
				}
			}
		}
	}
	fmt.Fprintf(&b, "\n## Timings (ms)\n\nprepare %.1f · invert %.1f · total %.1f\n", c.PrepareMs, c.InvertMs, c.TotalMs)
	return b.String()
}
