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
		// Bowed strings family — LFO→pitch vibrato showcase (Phase-1 seeds).
		// The no-edit Render uses renderModular (identity defaults); user edits
		// and the recipe path apply the seed via nativeModularRecipe.
		"violin": CVariantInstrument{
			Name:   "violin",
			Render: renderModular,
			Beats:  2.0,
		},
		"violin-ensemble": CVariantInstrument{
			Name:   "violin-ensemble",
			Render: renderModular,
			Beats:  2.0,
		},
		"cello": CVariantInstrument{
			Name:   "cello",
			Render: renderModular,
			Beats:  2.0,
		},
		"cello-warm": CVariantInstrument{
			Name:   "cello-warm",
			Render: renderModular,
			Beats:  2.0,
		},
		"organ-church": CVariantInstrument{
			Name:   "organ-church",
			Render: renderModular,
			Beats:  2.0,
		},
		"scifi-lead": CVariantInstrument{
			Name:   "scifi-lead",
			Render: renderModular,
			Beats:  2.0,
		},

		// Plucked strings — Karplus-Strong (Task 2, Beats: 1.5).
		"guitar-nylon": CVariantInstrument{
			Name:   "guitar-nylon",
			Render: renderModular,
			Beats:  1.5,
		},
		"guitar-nylon-bright": CVariantInstrument{
			Name:   "guitar-nylon-bright",
			Render: renderModular,
			Beats:  1.5,
		},
		"guitar-steel": CVariantInstrument{
			Name:   "guitar-steel",
			Render: renderModular,
			Beats:  1.5,
		},
		"guitar-steel-warm": CVariantInstrument{
			Name:   "guitar-steel-warm",
			Render: renderModular,
			Beats:  1.5,
		},
		"guitar-electric": CVariantInstrument{
			Name:   "guitar-electric",
			Render: renderModular,
			Beats:  1.5,
		},
		"harp": CVariantInstrument{
			Name:   "harp",
			Render: renderModular,
			Beats:  2.0,
		},
		"guitar-electric-neck": CVariantInstrument{
			Name:   "guitar-electric-neck",
			Render: renderModular,
			Beats:  1.5,
		},

		// Keys — additive piano (Task 3, Beats: 2.0).
		"piano-grand": CVariantInstrument{
			Name:   "piano-grand",
			Render: renderModular,
			Beats:  2.0,
		},
		"piano-felt": CVariantInstrument{
			Name:   "piano-felt",
			Render: renderModular,
			Beats:  2.0,
		},

		// Woodwind (Task 4, Beats: 2.0).
		"flute": CVariantInstrument{
			Name:   "flute",
			Render: renderModular,
			Beats:  2.0,
		},
		"flute-breathy": CVariantInstrument{
			Name:   "flute-breathy",
			Render: renderModular,
			Beats:  2.0,
		},
		"oboe": CVariantInstrument{
			Name:   "oboe",
			Render: renderModular,
			Beats:  2.0,
		},
		"oboe-full": CVariantInstrument{
			Name:   "oboe-full",
			Render: renderModular,
			Beats:  2.0,
		},

		// Brass (Task 5, Beats: 2.0).
		"trumpet": CVariantInstrument{
			Name:   "trumpet",
			Render: renderModular,
			Beats:  2.0,
		},
		"trumpet-mellow": CVariantInstrument{
			Name:   "trumpet-mellow",
			Render: renderModular,
			Beats:  2.0,
		},
		"french-horn": CVariantInstrument{
			Name:   "french-horn",
			Render: renderModular,
			Beats:  2.0,
		},
		"french-horn-loud": CVariantInstrument{
			Name:   "french-horn-loud",
			Render: renderModular,
			Beats:  2.0,
		},

		// Bass guitar — tuned plucked electric bass (modular, Beats: 1.5).
		"bass-guitar": CVariantInstrument{
			Name:   "bass-guitar",
			Render: renderModular,
			Beats:  1.5,
		},

		// Synth bass variants (modular, Beats: 1.5).
		"bass-acid": CVariantInstrument{
			Name:   "bass-acid",
			Render: renderModular,
			Beats:  1.5,
		},
		"bass-reese": CVariantInstrument{
			Name:   "bass-reese",
			Render: renderModular,
			Beats:  1.5,
		},
		"bass-fm": CVariantInstrument{
			Name:   "bass-fm",
			Render: renderModular,
			Beats:  1.5,
		},
		"bass-808": CVariantInstrument{
			Name:   "bass-808",
			Render: renderModular,
			Beats:  1.5,
		},

		// Modal conga (Task 7, Beats: 0.5 — short percussion).
		"conga": CVariantInstrument{
			Name:   "conga",
			Render: renderModular,
			Beats:  0.5,
		},
		"conga-open": CVariantInstrument{
			Name:   "conga-open",
			Render: renderModular,
			Beats:  0.5,
		},
		"conga-tumba": CVariantInstrument{
			Name:   "conga-tumba",
			Render: renderModular,
			Beats:  0.5,
		},

		// Masterpiece template set — three new instruments (all renderModular).
		// Organ — additive drawbars (Bach, house pads, techno chords, reggae bubble, salsa).
		"organ": CVariantInstrument{
			Name:   "organ",
			Render: renderModular,
			Beats:  2.0,
		},
		// Saxophone — subtractive reed (jazz: So What; bossa: Girl from Ipanema).
		"sax": CVariantInstrument{
			Name:   "sax",
			Render: renderModular,
			Beats:  2.0,
		},
		// ── Configurable KICK stage family (gen-bank kick voice, source==5). Each
		// is a SEEDED modular preset; bakedModularRender bakes the seed so the
		// no-edit playback dispatch plays the tuned kick, not the bare ~218 Hz
		// modular voice. Beats sets the buffer length (the kick's global fade is
		// buffer-normalized). ──
		// dnb-kick — deep, organic DnB sub-kick (pure sub + long pitch glide + bloom).
		"dnb-kick": CVariantInstrument{
			Name:   "dnb-kick",
			Render: bakedModularRender(dnbKickSeed),
			Beats:  1.0,
		},
		// kick-electro — hard electronic/EDM kick: clicky, punchy, harmonic, tight.
		"kick-electro": CVariantInstrument{
			Name:   "kick-electro",
			Render: bakedModularRender(electroKickSeed),
			Beats:  0.5,
		},
		// kick-808 — long boomy 808 sub: deep pitch-drop sub with a long ringing tail.
		"kick-808": CVariantInstrument{
			Name:   "kick-808",
			Render: bakedModularRender(kick808Seed),
			Beats:  1.5,
		},
		// kick-acoustic — natural tight acoustic kick: beater click + woody body.
		"kick-acoustic": CVariantInstrument{
			Name:   "kick-acoustic",
			Render: bakedModularRender(acousticKickSeed),
			Beats:  0.5,
		},
		// kick-punchy — punchy/raw/brutal kick on the LAYERED kick DSP (variant 5):
		// the grit + distortion + crest punch are all in the voice, no DefaultFX.
		"kick-punchy": CVariantInstrument{
			Name:   "kick-punchy",
			Render: bakedModularRender(punchyKickSeed),
			Beats:  0.5,
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
	clearInstrumentDisplayNames()
	bindBuiltinInstrumentRecipes()
}
