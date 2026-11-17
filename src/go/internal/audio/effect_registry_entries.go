package audio

func init() {
	// ── Existing effects ────────────────────────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectDistortion, DisplayName: "Distortion", Category: "creative",
		Params: []EffectParamDef{
			{Name: "drive", Min: 1, Max: 20, Default: 2},
			{Name: "tone", Min: 200, Max: 8000, Default: 4000, Unit: "Hz"},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newDistortion(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectDelay, DisplayName: "Delay", Category: "creative",
		Params: []EffectParamDef{
			{Name: "time", Min: 10, Max: 1000, Default: 250, Unit: "ms"},
			{Name: "feedback", Min: 0, Max: 0.95, Default: 0.4},
			{Name: "mix", Min: 0, Max: 1, Default: 0.3},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newDelay(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectReverb, DisplayName: "Reverb", Category: "creative",
		Params: []EffectParamDef{
			{Name: "room", Min: 0, Max: 1, Default: 0.5},
			{Name: "damping", Min: 0, Max: 1, Default: 0.5},
			{Name: "mix", Min: 0, Max: 1, Default: 0.3},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newReverb(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectChorus, DisplayName: "Chorus", Category: "modulation",
		Params: []EffectParamDef{
			{Name: "rate", Min: 0.1, Max: 10, Default: 1.5, Unit: "Hz"},
			{Name: "depth", Min: 0, Max: 20, Default: 5, Unit: "ms"},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newChorus(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectBitcrusher, DisplayName: "Bitcrusher", Category: "creative",
		Params: []EffectParamDef{
			{Name: "bits", Min: 2, Max: 16, Default: 8},
			{Name: "rate", Min: 0.01, Max: 1, Default: 0.5},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newBitcrusher(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectFilter, DisplayName: "Filter", Category: "utility",
		Params: []EffectParamDef{
			{Name: "mode", Min: 0, Max: 2, Default: 0},
			{Name: "cutoff", Min: 20, Max: 20000, Default: 1000, Unit: "Hz"},
			{Name: "q", Min: 0.1, Max: 10, Default: 0.707},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newFilter(sr, p) },
	})

	// ── New modulation effects ──────────────────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectPhaser, DisplayName: "Phaser", Category: "modulation",
		Params: []EffectParamDef{
			{Name: "stages", Min: 2, Max: 12, Default: 4},
			{Name: "rate", Min: 0.1, Max: 10, Default: 0.5, Unit: "Hz"},
			{Name: "depth", Min: 0, Max: 1, Default: 0.7},
			{Name: "feedback", Min: 0, Max: 0.95, Default: 0.5},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newPhaser(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectFlanger, DisplayName: "Flanger", Category: "modulation",
		Params: []EffectParamDef{
			{Name: "rate", Min: 0.1, Max: 10, Default: 0.5, Unit: "Hz"},
			{Name: "depth", Min: 0.5, Max: 10, Default: 3, Unit: "ms"},
			{Name: "feedback", Min: -0.95, Max: 0.95, Default: 0.5},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newFlanger(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectTremolo, DisplayName: "Tremolo", Category: "modulation",
		Params: []EffectParamDef{
			{Name: "rate", Min: 0.5, Max: 20, Default: 4, Unit: "Hz"},
			{Name: "depth", Min: 0, Max: 1, Default: 0.5},
			{Name: "shape", Min: 0, Max: 2, Default: 0},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newTremolo(sr, p) },
	})

	// ── New dynamics effects ────────────────────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectGate, DisplayName: "Noise Gate", Category: "dynamics",
		Params: []EffectParamDef{
			{Name: "threshold", Min: -60, Max: 0, Default: -30, Unit: "dB"},
			{Name: "attack", Min: 0.1, Max: 50, Default: 1, Unit: "ms"},
			{Name: "release", Min: 1, Max: 500, Default: 50, Unit: "ms"},
			{Name: "range", Min: -90, Max: 0, Default: -90, Unit: "dB"},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newGate(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectLimiter, DisplayName: "Limiter", Category: "dynamics",
		Params: []EffectParamDef{
			{Name: "threshold", Min: -20, Max: 0, Default: -1, Unit: "dB"},
			{Name: "release", Min: 1, Max: 500, Default: 50, Unit: "ms"},
			{Name: "ceiling", Min: -6, Max: 0, Default: -0.3, Unit: "dB"},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newLimiter(sr, p) },
	})

	// ── New creative effects ────────────────────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectRingMod, DisplayName: "Ring Mod", Category: "creative",
		Params: []EffectParamDef{
			{Name: "frequency", Min: 20, Max: 5000, Default: 440, Unit: "Hz"},
			{Name: "shape", Min: 0, Max: 1, Default: 0},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newRingMod(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectWaveshaper, DisplayName: "Waveshaper", Category: "creative",
		Params: []EffectParamDef{
			{Name: "curve", Min: 0, Max: 3, Default: 0},
			{Name: "drive", Min: 1, Max: 20, Default: 2},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newWaveshaper(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectAutoWah, DisplayName: "Auto-Wah", Category: "modulation",
		Params: []EffectParamDef{
			{Name: "sensitivity", Min: 0, Max: 1, Default: 0.5},
			{Name: "rate", Min: 0.5, Max: 20, Default: 2, Unit: "Hz"},
			{Name: "depth", Min: 0, Max: 1, Default: 0.7},
			{Name: "mix", Min: 0, Max: 1, Default: 0.5},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newAutoWah(sr, p) },
	})

	// ── Dynamics: per-instrument compressor ─────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectCompressor, DisplayName: "Compressor", Category: "dynamics",
		Params: []EffectParamDef{
			{Name: "threshold", Min: -60, Max: 0, Default: -20, Unit: "dB"},
			{Name: "ratio", Min: 1, Max: 20, Default: 4},
			{Name: "attack", Min: 0.1, Max: 100, Default: 10, Unit: "ms"},
			{Name: "release", Min: 10, Max: 1000, Default: 100, Unit: "ms"},
			{Name: "makeup", Min: 0, Max: 24, Default: 0, Unit: "dB"},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newCompressorFX(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectTransient, DisplayName: "Transient Shaper", Category: "dynamics",
		Params: []EffectParamDef{
			{Name: "attack", Min: 0, Max: 200, Default: 100, Unit: "%"},
			{Name: "sustain", Min: 0, Max: 200, Default: 100, Unit: "%"},
			{Name: "speed", Min: 1, Max: 50, Default: 10, Unit: "ms"},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newTransient(sr, p) },
	})

	// ── Creative: tape saturation ─────────────────────────────────────

	RegisterEffect(EffectRegistration{
		Type: EffectTape, DisplayName: "Tape Saturation", Category: "creative",
		Params: []EffectParamDef{
			{Name: "drive", Min: 1, Max: 10, Default: 2},
			{Name: "warmth", Min: 0, Max: 1, Default: 0.5},
			{Name: "wow", Min: 0, Max: 1, Default: 0},
			{Name: "flutter", Min: 0, Max: 1, Default: 0},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newTape(sr, p) },
	})

	RegisterEffect(EffectRegistration{
		Type: EffectPitchShift, DisplayName: "Pitch Shifter", Category: "creative",
		Params: []EffectParamDef{
			{Name: "pitch", Min: -24, Max: 24, Default: 0, Unit: "st"},
			{Name: "mix", Min: 0, Max: 1, Default: 1},
			{Name: "window", Min: 20, Max: 100, Default: 50, Unit: "ms"},
		},
		New: func(sr int, p map[string]float64) InsertEffect { return newPitchShift(sr, p) },
	})
}
