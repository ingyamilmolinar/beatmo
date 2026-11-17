package ui

import "math"

// animation_helpers.go — Phase 3 PR3 runtime consumers for the
// generated animation primitives in design_tokens.gen.go.
//
// Every animation magic number that previously lived inline in a draw
// call site (game_highlight_state.go's 0.8/0.02 decay, drumview_draw.go's
// 0.4 + 0.6·|sin(0.05·t)| pulse, components.go's fadeColor 0.3, etc.)
// is now sourced from a generated genAnim* constant via these helpers.
// The functions are intentionally tiny — escape analysis flattens them
// to the same machine code as the inline math, so per-frame call sites
// pay no cost.
//
// To retune cadence: edit DESIGN.md `animations:` and run
// `make gen-design-tokens` — never edit the genAnim* symbols directly.

// DecayStep applies one tick of an exponential-decay animation.
// Returns the new value and false when the value drops below the
// configured threshold (caller should drop the entry).
func DecayStep(v float64, anim ExpDecayAnim) (float64, bool) {
	v *= anim.Rate
	if v < anim.Threshold {
		return 0, false
	}
	return v, true
}

// SinPulse evaluates one frame of a sin-driven oscillator.
// Returns base + amplitude * |sin(frame * frame-step)|, clamped to
// [base, base+amplitude]. Caller multiplies by anim.AlphaScale (or
// any other ceiling) when driving an opacity channel. `frame` is
// int64 so call sites with monotonic int64 counters (Game.frame,
// DrumView.frame) don't have to widen at every call.
func SinPulse(frame int64, anim SinPulseAnim) float64 {
	return anim.Base + anim.Amplitude*math.Abs(math.Sin(float64(frame)*anim.FrameStep))
}

// SinPulseAlpha is SinPulse · AlphaScale rounded to uint8 — the most
// common pattern at call sites that drive WithAlpha(...) compositions.
func SinPulseAlpha(frame int64, anim SinPulseAnim) uint8 {
	v := SinPulse(frame, anim) * float64(anim.AlphaScale)
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v)
}
