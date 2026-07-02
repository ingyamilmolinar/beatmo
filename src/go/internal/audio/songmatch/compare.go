package songmatch

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

type AxisVerdict struct {
	Axis      string
	State     string // "OK" | "Drift" | "Mismatch"
	Magnitude float64
	Detail    string
}

type CorrectnessReport struct {
	Axes             []AxisVerdict
	Distance         Score
	ImpliedTranspose int
}

func stateFor(mag float64, cfg fingerprint.AnalysisConfig) string {
	switch {
	case mag < cfg.DriftTol:
		return "OK"
	case mag < cfg.MismatchTol:
		return "Drift"
	default:
		return "Mismatch"
	}
}

// Compare builds the per-axis correctness report (tempo, rhythm, key, mix) plus
// the weighted SongDistance and the implied key transposition.
func Compare(ref, cand fingerprint.SongFingerprint, cfg fingerprint.AnalysisConfig) CorrectnessReport {
	s := SongDistance(ref, cand, cfg)
	keyMag, shift := KeyMagnitude(ref, cand)
	modeStr := "same mode"
	if ref.Mode != cand.Mode {
		modeStr = "mode differs"
	}
	axes := []AxisVerdict{
		{Axis: "tempo", Magnitude: s.Tempo, State: stateFor(s.Tempo, cfg),
			Detail: fmt.Sprintf("ref %.1f BPM vs template %.1f BPM", ref.TempoBPM, cand.TempoBPM)},
		{Axis: "rhythm", Magnitude: s.Rhythm, State: stateFor(s.Rhythm, cfg),
			Detail: "onset bar-phase histogram L1"},
		{Axis: "key", Magnitude: keyMag, State: stateFor(keyMag, cfg),
			Detail: fmt.Sprintf("implied transpose %+d semitones, %s", shift, modeStr)},
		{Axis: "mix", Magnitude: s.Mix, State: stateFor(s.Mix, cfg),
			Detail: "per-band energy balance"},
	}
	return CorrectnessReport{Axes: axes, Distance: s, ImpliedTranspose: shift}
}
