//go:build !test && !js

package audio

// ResetInstruments restores the built-in instrument set.
func ResetInstruments() {
	instMu.Lock()
	instruments = map[string]Instrument{
		// Core acoustic-leaning synth kit.
		"snare":   Snare{},
		"kick":    Kick{},
		"hihat":   HiHat{},
		"tom":     Tom{},
		"clap":    Clap{},
		"cowbell": Cowbell{},

		// New distinct instruments.
		"rimshot":   Rimshot{},
		"sidestick": Sidestick{},
		"kick-deep": KickDeep{},
		"shaker":    Shaker{},
		"ride":      Ride{},
		"crash":     Crash{},

		// Bass instruments.
		"bass-guitar": CVariantInstrument{
			Name:   "bass-guitar",
			Render: renderBassGuitarVoice, // modular fast path (Phase-2 cutover)
			Beats:  1.5,                   // Longer sustain for bass
		},
		"sub-bass": CVariantInstrument{
			Name:   "sub-bass",
			Render: renderSubBassVoice, // modular fast path (Phase-2 cutover)
			Beats:  2.0,                // Very long sustain for sub
		},

		// Variant set 1: Electronic/Tight — shorter, punchier, aggressive.
		"snare-1": CVariantInstrument{
			Name:   "snare-1",
			Render: renderSnareRimshotVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.5,
			DefaultFX: []EffectSlot{
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.5, "tone": 8000, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.5) },
		},
		"kick-1": CVariantInstrument{
			Name:   "kick-1",
			Render: renderKickPunchyVoice, // modular fast path (Phase-3 cutover)
			Beats:  0.3,
		},
		"hihat-1": CVariantInstrument{
			Name:   "hihat-1",
			Render: renderOpenHiHatVoice, // modular fast path (Phase-6 cutover)
			Beats:  0.7,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 3000, "q": 0.707, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 1.4, "tone": 8000, "mix": 1}},
			},
		},
		"tom-1": CVariantInstrument{
			Name:   "tom-1",
			Render: renderTomHighVoice, // modular fast path (Phase-4 cutover)
			Beats:  0.3,
			DefaultFX: []EffectSlot{
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.0, "tone": 8000, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.4) },
		},
		"clap-1": CVariantInstrument{
			Name:   "clap-1",
			Render: renderClapVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.25,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 4000, "q": 0.707, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.5) },
		},
		"cowbell-1": CVariantInstrument{
			Name:   "cowbell-1",
			Render: renderCowbellVoice, // modular fast path (Phase-6 cutover)
			Beats:  0.3,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 800, "q": 0.707, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.5, "tone": 8000, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.6) },
		},

		// Bass variants: different timbres.
		"bass-guitar-1": CVariantInstrument{
			Name:   "bass-guitar-1",
			Render: renderBassGuitarVoice, // modular fast path (Phase-2 cutover)
			Beats:  1.2,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 80, "q": 0.707, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 1.6, "tone": 8000, "mix": 1}},
			},
		},
		"sub-bass-1": CVariantInstrument{
			Name:   "sub-bass-1",
			Render: renderSubBassVoice, // modular fast path (Phase-2 cutover)
			Beats:  1.5,
			DefaultFX: []EffectSlot{
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.0, "tone": 8000, "mix": 1}},
			},
		},

		// Variant set 2: Lo-fi/Dark — heavy bitcrush, dark LP filters.
		"snare-2": CVariantInstrument{
			Name:   "snare-2",
			Render: renderSnareVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.6,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 4, "rate": 1, "mix": 1}},
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 3000, "q": 0.707, "mix": 1}},
			},
		},
		"kick-2": CVariantInstrument{
			Name:   "kick-2",
			Render: renderKickLofiVoice, // modular fast path (Phase-3 cutover)
			Beats:  0.5,
		},
		"hihat-2": CVariantInstrument{
			Name:   "hihat-2",
			Render: renderHiHatVoice, // modular fast path (Phase-6 cutover)
			Beats:  0.2,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 4, "rate": 1, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.35) },
		},
		"tom-2": CVariantInstrument{
			Name:   "tom-2",
			Render: renderTomLowVoice, // modular fast path (Phase-4 cutover)
			Beats:  0.7,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 5, "rate": 1, "mix": 1}},
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 2000, "q": 0.707, "mix": 1}},
			},
		},
		"clap-2": CVariantInstrument{
			Name:   "clap-2",
			Render: renderClapVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.3,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 4, "rate": 1, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 2.0, "tone": 8000, "mix": 1}},
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 500, "q": 0.707, "mix": 1}},
			},
		},
		"cowbell-2": CVariantInstrument{
			Name:   "cowbell-2",
			Render: renderCowbellVoice, // modular fast path (Phase-6 cutover)
			Beats:  0.4,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 4, "rate": 1, "mix": 1}},
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 2500, "q": 0.707, "mix": 1}},
			},
		},

		// Variant set 3: expressive dynamics.
		"snare-ghost": CVariantInstrument{
			Name:   "snare-ghost",
			Render: renderSnareVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.4,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 3000, "q": 0.707, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.6) },
		},
		"kick-tight": CVariantInstrument{
			Name:   "kick-tight",
			Render: renderKickTightVoice, // modular fast path (Phase-3 cutover)
			Beats:  0.3,
		},
		"hihat-pedal": CVariantInstrument{
			Name:   "hihat-pedal",
			Render: renderHiHatVoice, // modular fast path (Phase-6 cutover)
			Beats:  0.15,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 6000, "q": 0.707, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.3) },
		},
		"clap-tight": CVariantInstrument{
			Name:   "clap-tight",
			Render: renderClapVoice, // modular fast path (Phase-5 cutover)
			Beats:  0.25,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 300, "q": 0.707, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.4) },
		},

		// FM synthesis instruments.
		"fm-bass": CVariantInstrument{
			Name:   "fm-bass",
			Render: renderFMBassVoice, // modular fast path (Phase-7 cutover)
			Beats:  1.5,
		},
		"fm-bell": CVariantInstrument{
			Name:   "fm-bell",
			Render: renderFMBellVoice, // modular fast path (Phase-7 cutover)
			Beats:  2.0,
		},
		"fm-lead": CVariantInstrument{
			Name:   "fm-lead",
			Render: renderFMLeadVoice, // modular fast path (Phase-7 cutover)
			Beats:  1.0,
		},
		"fm-epiano": CVariantInstrument{
			Name:   "fm-epiano",
			Render: renderFMEPianoVoice, // modular fast path (Phase-7 cutover)
			Beats:  2.0,
		},
		"fm-pluck": CVariantInstrument{
			Name:   "fm-pluck",
			Render: renderFMPluckVoice, // modular fast path (Phase-7 cutover)
			Beats:  0.5,
		},

		// FM variants (post-processed via DefaultFX).
		"fm-bass-1": CVariantInstrument{
			Name:   "fm-bass-1",
			Render: renderFMBassVoice, // modular fast path (Phase-7 cutover)
			Beats:  1.0,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 60, "q": 0.707, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 1.8, "tone": 8000, "mix": 1}},
			},
		},
		"fm-bell-1": CVariantInstrument{
			Name:   "fm-bell-1",
			Render: renderFMBellVoice, // modular fast path (Phase-7 cutover)
			Beats:  1.5,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 4000, "q": 0.707, "mix": 1}},
			},
		},
		"fm-lead-1": CVariantInstrument{
			Name:   "fm-lead-1",
			Render: renderFMLeadVoice, // modular fast path (Phase-7 cutover)
			Beats:  0.7,
			DefaultFX: []EffectSlot{
				{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{"bits": 5, "rate": 1, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.6) },
		},
		"fm-epiano-1": CVariantInstrument{
			Name:   "fm-epiano-1",
			Render: renderFMEPianoVoice, // modular fast path (Phase-7 cutover)
			Beats:  1.5,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 0, "cutoff": 3000, "q": 0.707, "mix": 1}},
				{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 1.4, "tone": 8000, "mix": 1}},
			},
		},
		"fm-pluck-1": CVariantInstrument{
			Name:   "fm-pluck-1",
			Render: renderFMPluckVoice, // modular fast path (Phase-7 cutover)
			Beats:  0.3,
			DefaultFX: []EffectSlot{
				{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 150, "q": 0.707, "mix": 1}},
			},
			Post: func(buf []float32, _ int) { gateTail(buf, 0.5) },
		},

		// Unified modular synth voice. The no-edit Render uses the engine's
		// built-in defaults (== synth-modular recipe identity defaults); user
		// edits flow through the recipe path (nativeModularRecipe) which renders
		// the full param block via render_modular_p.
		"modular": CVariantInstrument{
			Name:   "modular",
			Render: renderModular,
			Beats:  1.0,
		},
		// Second shipped modular preset — soft dark pad. The no-edit Render
		// bakes the pad defaults (modularPadSeed) so the cheap dispatch path
		// matches the synth-modular-pad recipe; user edits flow through the
		// recipe path (nativeModularRecipe on synth-modular-pad).
		"modular-pad": CVariantInstrument{
			Name:   "modular-pad",
			Render: renderModularPad,
			Beats:  1.0,
		},
	}
	instOrder = append([]string(nil), BuiltinInstrumentIDs...)
	// Snapshot the as-shipped instrument set so a factory Reset can restore a
	// single instrument's built-in render after the Sampler's Save overwrote it
	// with a chopped Sample (UnregisterSamplePCM).
	factoryInstruments = make(map[string]Instrument, len(instruments))
	for id, inst := range instruments {
		factoryInstruments[id] = inst
	}
	instMu.Unlock()
	globalVoiceCache.Clear()
	bumpInstrumentsVersion()
	ClearAllInsertEffects()
	resetInstrumentChannels(instOrder)
	bindBuiltinInstrumentRecipes()
}
