package audio

import "math"

// preview_render.go — the "Your sound" fast-compute preview.
//
// RenderInstrumentPreview renders a lightweight, pure-Go APPROXIMATION of one
// note of an instrument's synth voice for the UI right-pane mirror. It is
// deliberately NOT the exact engine output (the bit-exact note needs the
// platform DSP — C on desktop, the emscripten/WebAudio path in the browser,
// neither reachable from the cgo-free Go-WASM build). Instead it is a fast
// model that reads EVERY major stage param, so turning ANY knob visibly changes
// the wave, and it renders identically on desktop, wasm, and under test (no
// cgo, no mixer, no build tags). Accuracy is traded for responsiveness +
// cross-platform consistency — the right call for a glanceable preview.

const previewSampleRate = 48000.0

// previewGeneratorType resolves the oscillator/generator WAVE SHAPE index for a
// recipe so the preview reflects the wave-shape knob on EVERY recipe — modular's
// osc_type, OR a bespoke "Generator" enum (custom name like kick_wave /
// snare_wave / fm_wave, group core, Sine/Saw/Square/Triangle). 0 (sine) when the
// recipe has no shape selector. Index convention matches previewOscShape.
func previewGeneratorType(recipeID string, p RecipeParams) int {
	// Scan VISIBLE params for the wave-shape selector. Bespoke recipes carry a
	// HIDDEN osc_type (default sine) from the Phase-8A migration that must be
	// skipped, or it would mask the real visible generator (kick_wave, fm_wave…).
	if reg, ok := RecipeRegistrations()[recipeID]; ok && reg != nil {
		for _, d := range reg.Params {
			if d.Group == SynthHiddenGroup {
				continue
			}
			if d.Name == "osc_type" || d.Label == "Generator" ||
				(len(d.Enum) >= 2 && d.Enum[0] == "Sine" && d.Enum[1] == "Saw") {
				if v, ok := p[d.Name]; ok {
					return int(v + 0.5)
				}
			}
		}
	}
	if v, ok := p["osc_type"]; ok {
		return int(v + 0.5)
	}
	return 0
}

// RenderInstrumentPreview returns float64 mono samples for one note of the
// instrument's resolved recipe params. durationMs is clamped so the call stays
// cheap. Safe to call from a worker goroutine (reads param maps only).
func RenderInstrumentPreview(instrumentID string, durationMs int) []float64 {
	if durationMs <= 0 {
		durationMs = 200
	}
	if durationMs > 1000 {
		durationMs = 1000
	}
	recipeID := RecipeForInstrument(instrumentID)
	if recipeID == "" {
		return nil
	}
	p := MergeRecipeDefaults(recipeID, GetInstrumentParams(instrumentID))

	get := func(name string, def float64) float64 {
		if v, ok := p[name]; ok {
			return v
		}
		return def
	}
	has := func(name string) bool { _, ok := p[name]; return ok }
	// A stage gated by a *_enabled param is active when that param is >= 0.5.
	// When the param is absent (bespoke recipes), the stage is treated as on.
	stageOn := func(name string) bool {
		v, ok := p[name]
		if !ok {
			return true
		}
		return v >= 0.5
	}

	sr := previewSampleRate
	n := int(sr * float64(durationMs) / 1000.0)
	if n < 2 {
		n = 2
	}
	total := float64(n) / sr

	// Pitch: fundamental (Hz) shifted by octave + detune (cents) + pitch (semis).
	baseHz := get("fundamental", 0)
	if baseHz <= 0 {
		baseHz = 220
	}
	// FM voices carry their fundamental as fm_base_freq ("Base Pitch", 20–2000 Hz,
	// the legacy fm_base alias on older recipes), not `fundamental`. Honor it so
	// the FM carrier period tracks the knob (conceptPitchWave routes it). Reading
	// it for non-FM recipes is harmless: they lack the param, so get returns 0 and
	// baseHz is unchanged. Applied before octave/detune/pitch so those compose.
	if fb := get("fm_base_freq", 0); fb > 0 {
		baseHz = fb
	} else if fb := get("fm_base", 0); fb > 0 {
		baseHz = fb
	}
	baseHz *= math.Pow(2, get("osc_octave", 0))
	baseHz *= math.Pow(2, get("osc_detune", 0)/1200.0)
	baseHz *= math.Pow(2, get("pitch", 0)/12.0)

	oscType := previewGeneratorType(recipeID, p)

	// Multi-operator FM: active when fm is enabled (or, for bespoke recipes
	// without the toggle, when any operator has depth / the algorithm is additive).
	fmState := previewFMStateFrom(get)
	fmActive := (has("fm_enabled") && stageOn("fm_enabled")) || (!has("fm_enabled") && fmState.active())

	// Amp ADSR (on by default). Falls back to a generic exp decay.
	envActive := stageOn("env_enabled")
	atk := get("amp_attack", 0.005)
	dec := get("amp_decay", get("decay", 0.3))
	sus := get("amp_sustain", 0)
	rel := get("amp_release", 0.08)

	// Pitch envelope (gated): exponential semitone sweep toward 0.
	peActive := has("pitchenv_enabled") && stageOn("pitchenv_enabled")
	peAmt := get("pitchenv_amt", 0)
	peDecay := get("pitchenv_decay", 0.08)
	if peDecay <= 0 {
		peDecay = 0.08
	}

	// State-variable filter (gated): cutoff + resonance + type (0=LP 1=HP 2=BP).
	filtActive := has("filter_enabled") && stageOn("filter_enabled")
	cutoff := get("filter_cutoff", 20000)
	if cutoff < 20 {
		cutoff = 20
	}
	if cutoff > sr/2.2 {
		cutoff = sr / 2.2
	}
	reso := get("filter_resonance", 0.707)
	if reso < 0.5 {
		reso = 0.5
	}
	filtType := int(get("filter_type", 0) + 0.5)
	svfF := 2 * math.Sin(math.Pi*cutoff/sr)
	svfQ := 1.0 / reso
	var svfLow, svfBand float64

	// LFO (gated): amplitude tremolo.
	lfoActive := has("lfo_enabled") && stageOn("lfo_enabled")
	lfoRate := get("lfo_rate", 5)
	lfoDepth := get("lfo_depth", 0)

	// Drive (gated) + output gain.
	driveActive := (has("drive_enabled") && stageOn("drive_enabled")) || (!has("drive_enabled") && get("drive", 0) > 0)
	drive := get("drive", 0)
	gain := get("gain", 1)

	out := make([]float64, n)
	for i := 0; i < n; i++ {
		t := float64(i) / sr

		hz := baseHz
		if peActive && peAmt != 0 {
			hz *= math.Pow(2, (peAmt*math.Exp(-t/peDecay))/12.0)
		}
		ph := hz * t

		var s float64
		if fmActive && fmState.active() {
			s = previewFMSample(fmState, oscType, ph)
		} else {
			s = previewOscShape(oscType, ph)
		}

		if envActive {
			s *= previewADSR(t, total, atk, dec, sus, rel)
		} else {
			d := dec
			if d < 0.01 {
				d = 0.01
			}
			s *= math.Exp(-t / d)
		}

		if lfoActive && lfoDepth > 0 {
			s *= 1 - lfoDepth*0.5*(1-math.Cos(2*math.Pi*lfoRate*t))
		}

		if filtActive {
			svfLow += svfF * svfBand
			high := s - svfLow - svfQ*svfBand
			svfBand += svfF * high
			switch filtType {
			case 1:
				s = high
			case 2:
				s = svfBand
			default:
				s = svfLow
			}
		}

		if driveActive && drive > 0 {
			k := 1 + drive*9
			s = math.Tanh(k*s) / math.Tanh(k)
		}

		out[i] = s * gain
	}
	return out
}

// RenderInstrumentPreviewWave renders `cycles` cycles of the instrument's STEADY
// timbre — oscillator (+FM) → filter → drive — at `pointsPerCycle` resolution,
// with NO amp envelope and NO LFO. Because it renders a FIXED number of cycles,
// the wave SHAPE is always legible regardless of pitch (a 57 Hz kick and a
// 440 Hz lead both show the same count of clean cycles), and the output is
// soft-clamped into [-1,1] then normalized so it can never block out the trace.
// This is the clean "wave" used by the right-pane mirror and the per-knob effect
// pictures. `override` replaces specific params (e.g. to draw the wave at a
// contrasting value of one knob so its effect is visible side by side).
func RenderInstrumentPreviewWave(instrumentID string, override map[string]float64, cycles, pointsPerCycle int) []float64 {
	if cycles < 1 {
		cycles = 1
	}
	if pointsPerCycle < 8 {
		pointsPerCycle = 8
	}
	recipeID := RecipeForInstrument(instrumentID)
	if recipeID == "" {
		return nil
	}
	p := MergeRecipeDefaults(recipeID, GetInstrumentParams(instrumentID))
	if len(override) > 0 {
		merged := make(RecipeParams, len(p)+len(override))
		for k, v := range p {
			merged[k] = v
		}
		for k, v := range override {
			merged[k] = v
		}
		p = merged
	}

	get := func(name string, def float64) float64 {
		if v, ok := p[name]; ok {
			return v
		}
		return def
	}
	has := func(name string) bool { _, ok := p[name]; return ok }
	stageOn := func(name string) bool {
		v, ok := p[name]
		if !ok {
			return true
		}
		return v >= 0.5
	}

	baseHz := get("fundamental", 0)
	if baseHz <= 0 {
		baseHz = 220
	}
	oscType := previewGeneratorType(recipeID, p)

	fmState := previewFMStateFrom(get)
	fmActive := (has("fm_enabled") && stageOn("fm_enabled")) || (!has("fm_enabled") && fmState.active())

	// Filter coefficient is computed against an effective sample rate of
	// pointsPerCycle samples per fundamental cycle, so the cutoff acts RELATIVE
	// to the note (cutoff near the fundamental rounds the wave; far above it
	// barely touches it) — which is what makes the filter's effect visible.
	filtActive := has("filter_enabled") && stageOn("filter_enabled")
	cutoff := get("filter_cutoff", 20000)
	reso := get("filter_resonance", 0.707)
	if reso < 0.5 {
		reso = 0.5
	}
	filtType := int(get("filter_type", 0) + 0.5)
	srEff := float64(pointsPerCycle) * baseHz
	fc := cutoff
	if fc < 20 {
		fc = 20
	}
	svfF := 2 * math.Sin(math.Pi*fc/srEff)
	// Clamp to the Chamberlin SVF's stable range. Above this the filter
	// oscillates at Nyquist (a buzzy ±peak square). The clamp also means a high
	// cutoff lands as "filter wide open" (no effect) — exactly right.
	if svfF > 0.9 {
		svfF = 0.9
	}
	svfQ := 1.0 / reso
	var svfLow, svfBand float64

	driveActive := (has("drive_enabled") && stageOn("drive_enabled")) || (!has("drive_enabled") && get("drive", 0) > 0)
	drive := get("drive", 0)
	gain := get("gain", 1)

	// Render a few extra cycles up front so the filter settles, then keep the
	// last `cycles` cycles (steady state).
	const settle = 4
	totalCycles := cycles + settle
	total := totalCycles * pointsPerCycle
	keep := cycles * pointsPerCycle
	out := make([]float64, keep)
	peak := 0.0
	for i := 0; i < total; i++ {
		ph := float64(i) / float64(pointsPerCycle)
		var s float64
		if fmActive && fmState.active() {
			s = previewFMSample(fmState, oscType, ph)
		} else {
			s = previewOscShape(oscType, ph)
		}
		if filtActive {
			svfLow += svfF * svfBand
			high := s - svfLow - svfQ*svfBand
			svfBand += svfF * high
			switch filtType {
			case 1:
				s = high
			case 2:
				s = svfBand
			default:
				s = svfLow
			}
		}
		if driveActive && drive > 0 {
			k := 1 + drive*9
			s = math.Tanh(k*s) / math.Tanh(k)
		}
		s *= gain
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		if i >= settle*pointsPerCycle {
			j := i - settle*pointsPerCycle
			out[j] = s
			if a := math.Abs(s); a > peak {
				peak = a
			}
		}
	}
	if peak > 1e-3 {
		g := 0.9 / peak
		for i := range out {
			out[i] *= g
		}
	}
	return out
}

// previewOscShape returns one sample of oscillator type at phase ph (cycles).
// 0=sine 1=saw 2=square 3=triangle 5/6=noise (4/FM handled by the caller).
func previewOscShape(oscType int, ph float64) float64 {
	frac := ph - math.Floor(ph) // [0,1)
	switch oscType {
	case 1:
		return 2*frac - 1
	case 2:
		if frac < 0.5 {
			return 1
		}
		return -1
	case 3:
		return 1 - 4*math.Abs(frac-0.5)
	case 5, 6:
		// Deterministic per-phase pseudo-noise so the trace is reproducible.
		u := uint32(ph*4096.0)*2654435761 + 0x9E3779B9
		u ^= u >> 15
		v := float64(int32(u)) / float64(int32(1)<<30)
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		if oscType == 6 {
			v *= 0.6
		}
		return v
	default:
		return math.Sin(2 * math.Pi * frac)
	}
}

// previewFMState holds the per-operator FM parameters and the routing topology
// for the multi-operator preview FM model.
type previewFMState struct {
	ratios [4]float64 // frequency × fundamental per operator
	depths [4]float64 // modulation index per operator
	levels [4]float64 // carrier output mix per operator (Parallel algorithm)
	algo   int        // 0 2-op · 1 Parallel · 2 3-op chain · 3 4-op stack
}

// active reports whether any operator contributes modulation.
func (fm previewFMState) active() bool {
	return fm.depths[0] > 0 || fm.depths[1] > 0 || fm.depths[2] > 0 || fm.depths[3] > 0 ||
		fm.algo == 1 // Parallel sums carriers even at zero depth
}

// fmLevelScale maps an operator's output LEVEL (0..1) to a modulation-index
// multiplier in [0.5, 1]. A modulator at level 0 still contributes (so DEPTH
// alone reads on recipes without a level knob — bespoke FM), but raising level
// scales its contribution up to 1× — so the level knob visibly changes the wave.
func fmLevelScale(level float64) float64 {
	if level < 0 {
		level = 0
	} else if level > 1 {
		level = 1
	}
	return 0.5 + 0.5*level
}

// previewFMSample returns one carrier sample at fundamental phase ph (cycles)
// for the given oscillator wave shape. It models the four operators routed by
// algo so that EVERY operator's ratio/depth/level and the algorithm itself
// change the output — a faithful-enough approximation of the C operator matrix
// for the glanceable preview. Operator phase modulation is the classic
// phase-summation form: a modulator at ratio r and index d adds
// d·sin(2π·r·ph)/(2π) to the modulated operator's phase. Op1 (index 0) is the
// carrier; ops 2..4 (indices 1..3) are the modulators.
func previewFMSample(fm previewFMState, oscType int, ph float64) float64 {
	op := func(i int, phaseMod float64) float64 {
		return previewOscShape(oscType, fm.ratios[i]*ph+phaseMod)
	}
	// pm returns the phase contribution of operator i oscillating at its own
	// ratio, scaled by depth × its level multiplier.
	pm := func(i int) float64 {
		return fmLevelScale(fm.levels[i]) * fm.depths[i] *
			math.Sin(2*math.Pi*fm.ratios[i]*ph) / (2 * math.Pi)
	}
	switch fm.algo {
	case 1: // Parallel: all four are independent carriers summed by level.
		var s, wsum float64
		for i := 0; i < 4; i++ {
			lvl := fm.levels[i]
			if lvl <= 0 {
				continue
			}
			// Each carrier is lightly self-bent by its own depth so depth still
			// reads even in the additive topology.
			s += lvl * op(i, pm(i))
			wsum += lvl
		}
		if wsum <= 0 {
			return op(0, 0)
		}
		return s / wsum
	case 2: // 3-op chain: op3 → op2 → op1 (carrier).
		m3 := pm(2)
		m2 := fmLevelScale(fm.levels[1]) * fm.depths[1] *
			math.Sin(2*math.Pi*(fm.ratios[1]*ph+m3)) / (2 * math.Pi)
		return op(0, m2)
	case 3: // 4-op stack: op4 → op3 → op2 → op1 (carrier).
		m4 := pm(3)
		m3 := fmLevelScale(fm.levels[2]) * fm.depths[2] *
			math.Sin(2*math.Pi*(fm.ratios[2]*ph+m4)) / (2 * math.Pi)
		m2 := fmLevelScale(fm.levels[1]) * fm.depths[1] *
			math.Sin(2*math.Pi*(fm.ratios[1]*ph+m3)) / (2 * math.Pi)
		return op(0, m2)
	default: // 2-op (default): the carrier is phase-modulated by the SUM of every
		// modulator operator (2..4) that has depth, so each modulator's ratio,
		// depth and level all bend the wave even with no explicit algorithm knob.
		phaseMod := pm(1) + pm(2) + pm(3)
		return op(0, phaseMod)
	}
}

// previewFMStateFrom extracts the multi-operator FM state from a resolved param
// map (get supplies defaults for absent keys).
func previewFMStateFrom(get func(string, float64) float64) previewFMState {
	fm := previewFMState{
		ratios: [4]float64{get("fm_op1_ratio", 1), get("fm_op2_ratio", 1), get("fm_op3_ratio", 1), get("fm_op4_ratio", 1)},
		depths: [4]float64{get("fm_op1_depth", 0), get("fm_op2_depth", 0), get("fm_op3_depth", 0), get("fm_op4_depth", 0)},
		levels: [4]float64{get("fm_op1_level", 1), get("fm_op2_level", 0), get("fm_op3_level", 0), get("fm_op4_level", 0)},
		algo:   int(get("fm_algorithm", 0) + 0.5),
	}
	for i := range fm.ratios {
		if fm.ratios[i] <= 0 {
			fm.ratios[i] = 1
		}
	}
	return fm
}

// previewADSR returns the amplitude envelope at time t over a note of length
// total seconds (attack/decay/release in seconds, sustain in [0,1]). Release
// starts at total-rel.
func previewADSR(t, total, atk, dec, sus, rel float64) float64 {
	if atk < 0 {
		atk = 0
	}
	if dec < 0 {
		dec = 0
	}
	if rel < 0 {
		rel = 0
	}
	relStart := total - rel
	if relStart < 0 {
		relStart = 0
	}
	switch {
	case t < atk && atk > 0:
		return t / atk
	case t < atk+dec && dec > 0:
		return 1 - (1-sus)*(t-atk)/dec
	case t >= relStart && rel > 0:
		return sus * (1 - (t-relStart)/rel)
	default:
		return sus
	}
}
