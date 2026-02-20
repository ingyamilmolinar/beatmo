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
			Render: renderBassGuitar,
			Beats:  1.5, // Longer sustain for bass
		},
		"sub-bass": CVariantInstrument{
			Name:   "sub-bass",
			Render: renderSubBass,
			Beats:  2.0, // Very long sustain for sub
		},

		// Variant set 1: Electronic/Tight — shorter, punchier, aggressive.
		"snare-1": CVariantInstrument{
			Name:   "snare-1",
			Render: renderSnareRimshot,
			Beats:  0.5,
			Post: func(buf []float32, sr int) {
				gateTail(buf, 0.5)
				softClip(buf, 2.5, 0.85)
			},
		},
		"kick-1": CVariantInstrument{
			Name:   "kick-1",
			Render: renderKickPunchy,
			Beats:  0.3,
		},
		"hihat-1": CVariantInstrument{
			Name:   "hihat-1",
			Render: renderOpenHiHat,
			Beats:  0.7,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 3000.0)
				softClip(buf, 1.4, 0.95)
			},
		},
		"tom-1": CVariantInstrument{
			Name:   "tom-1",
			Render: renderTomHigh,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				gateTail(buf, 0.4)
				softClip(buf, 2.0, 0.85)
			},
		},
		"clap-1": CVariantInstrument{
			Name:   "clap-1",
			Render: renderClap,
			Beats:  0.25,
			Post: func(buf []float32, sr int) {
				gateTail(buf, 0.5)
				lpFilter(buf, sr, 4000.0)
			},
		},
		"cowbell-1": CVariantInstrument{
			Name:   "cowbell-1",
			Render: renderCowbell,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 800.0)
				gateTail(buf, 0.6)
				softClip(buf, 2.5, 0.85)
			},
		},

		// Bass variants: different timbres.
		"bass-guitar-1": CVariantInstrument{
			Name:   "bass-guitar-1",
			Render: renderBassGuitar,
			Beats:  1.2, // Tighter, more percussive
			Post: func(buf []float32, sr int) {
				// Brighter, more aggressive pluck
				hpFilter(buf, sr, 80.0)
				softClip(buf, 1.6, 0.9)
			},
		},
		"sub-bass-1": CVariantInstrument{
			Name:   "sub-bass-1",
			Render: renderSubBass,
			Beats:  1.5, // Shorter, punchier
			Post: func(buf []float32, sr int) {
				// Add harmonics for presence on small speakers
				softClip(buf, 2.0, 0.85)
			},
		},

		// Variant set 2: Lo-fi/Dark — heavy bitcrush, dark LP filters.
		"snare-2": CVariantInstrument{
			Name:   "snare-2",
			Render: renderSnare,
			Beats:  0.6,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 4)
				lpFilter(buf, sr, 3000.0)
			},
		},
		"kick-2": CVariantInstrument{
			Name:   "kick-2",
			Render: renderKickLofi,
			Beats:  0.5,
		},
		"hihat-2": CVariantInstrument{
			Name:   "hihat-2",
			Render: renderHiHat,
			Beats:  0.2,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 4)
				gateTail(buf, 0.35)
			},
		},
		"tom-2": CVariantInstrument{
			Name:   "tom-2",
			Render: renderTomLow,
			Beats:  0.7,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 5)
				lpFilter(buf, sr, 2000.0)
			},
		},
		"clap-2": CVariantInstrument{
			Name:   "clap-2",
			Render: renderClap,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 4)
				softClip(buf, 2.0, 0.8)
				hpFilter(buf, sr, 500.0)
			},
		},
		"cowbell-2": CVariantInstrument{
			Name:   "cowbell-2",
			Render: renderCowbell,
			Beats:  0.4,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 4)
				lpFilter(buf, sr, 2500.0)
			},
		},

		// Variant set 3: expressive dynamics.
		"snare-ghost": CVariantInstrument{
			Name:   "snare-ghost",
			Render: renderSnare,
			Beats:  0.4,
			Post: func(buf []float32, sr int) {
				lpFilter(buf, sr, 3000.0)
				gateTail(buf, 0.6)
			},
		},
		"kick-tight": CVariantInstrument{
			Name:   "kick-tight",
			Render: renderKickTight,
			Beats:  0.3,
		},
		"hihat-pedal": CVariantInstrument{
			Name:   "hihat-pedal",
			Render: renderHiHat,
			Beats:  0.15,
			Post: func(buf []float32, sr int) {
				gateTail(buf, 0.3)
				hpFilter(buf, sr, 6000.0)
			},
		},
		"clap-tight": CVariantInstrument{
			Name:   "clap-tight",
			Render: renderClap,
			Beats:  0.25,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 300.0)
				gateTail(buf, 0.4)
			},
		},

		// FM synthesis instruments.
		"fm-bass": CVariantInstrument{
			Name:   "fm-bass",
			Render: renderFMBass,
			Beats:  1.5,
		},
		"fm-bell": CVariantInstrument{
			Name:   "fm-bell",
			Render: renderFMBell,
			Beats:  2.0,
		},
		"fm-lead": CVariantInstrument{
			Name:   "fm-lead",
			Render: renderFMLead,
			Beats:  1.0,
		},
		"fm-epiano": CVariantInstrument{
			Name:   "fm-epiano",
			Render: renderFMEPiano,
			Beats:  2.0,
		},
		"fm-pluck": CVariantInstrument{
			Name:   "fm-pluck",
			Render: renderFMPluck,
			Beats:  0.5,
		},

		// FM variants (post-processed).
		"fm-bass-1": CVariantInstrument{
			Name:   "fm-bass-1",
			Render: renderFMBass,
			Beats:  1.0,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 60.0)
				softClip(buf, 1.8, 0.9)
			},
		},
		"fm-bell-1": CVariantInstrument{
			Name:   "fm-bell-1",
			Render: renderFMBell,
			Beats:  1.5,
			Post: func(buf []float32, sr int) {
				lpFilter(buf, sr, 4000.0)
			},
		},
		"fm-lead-1": CVariantInstrument{
			Name:   "fm-lead-1",
			Render: renderFMLead,
			Beats:  0.7,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 5)
				gateTail(buf, 0.6)
			},
		},
		"fm-epiano-1": CVariantInstrument{
			Name:   "fm-epiano-1",
			Render: renderFMEPiano,
			Beats:  1.5,
			Post: func(buf []float32, sr int) {
				lpFilter(buf, sr, 3000.0)
				softClip(buf, 1.4, 0.95)
			},
		},
		"fm-pluck-1": CVariantInstrument{
			Name:   "fm-pluck-1",
			Render: renderFMPluck,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 150.0)
				gateTail(buf, 0.5)
			},
		},
	}
	instOrder = append([]string(nil), BuiltinInstrumentIDs...)
	instMu.Unlock()
	globalVoiceCache.Clear()
	bumpInstrumentsVersion()
	ClearAllInsertEffects()
	resetInstrumentChannels(instOrder)
}
