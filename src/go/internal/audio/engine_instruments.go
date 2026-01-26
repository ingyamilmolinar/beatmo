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

		// Variant set 1: slightly brighter/tighter versions.
		"snare-1": CVariantInstrument{
			Name:   "snare-1",
			Render: renderSnare,
			Beats:  0.8,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 180.0)
				softClip(buf, 1.4, 0.9)
			},
		},
		"kick-1": CVariantInstrument{
			Name:   "kick-1",
			Render: renderKick,
			Beats:  0.5,
			Post: func(buf []float32, sr int) {
				// Brighter, more electronic punch: trim extreme sub and add
				// stronger saturation instead of low-pass muffling.
				hpFilter(buf, sr, 70.0)
				softClip(buf, 1.9, 0.9)
			},
		},
		"hihat-1": CVariantInstrument{
			Name:   "hihat-1",
			Render: renderOpenHiHat,
			Beats:  0.7, // longer tail for open/house-style hats
			Post: func(buf []float32, sr int) {
				// Emphasize sizzly top and keep lows out of the way.
				hpFilter(buf, sr, 3000.0)
				softClip(buf, 1.4, 0.95)
			},
		},
		"tom-1": CVariantInstrument{
			Name:   "tom-1",
			Render: renderTomHigh,
			Beats:  0.5, // Shorter for punchy high tom.
			Post: func(buf []float32, sr int) {
				// Gentle saturation only - let the natural sound through.
				softClip(buf, 1.2, 0.92)
			},
		},
		"clap-1": CVariantInstrument{
			Name:   "clap-1",
			Render: renderClap,
			Beats:  0.4,
			Post: func(buf []float32, sr int) {
				softClip(buf, 1.5, 0.9)
			},
		},
		"cowbell-1": CVariantInstrument{
			Name:   "cowbell-1",
			Render: renderCowbell,
			Beats:  0.6,
			Post: func(buf []float32, sr int) {
				hpFilter(buf, sr, 600.0)
				softClip(buf, 1.4, 0.9)
			},
		},

		// Variant set 2: more obviously digital/lofi flavours.
		"snare-2": CVariantInstrument{
			Name:   "snare-2",
			Render: renderSnare,
			Beats:  0.6,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 6)
				hpFilter(buf, sr, 400.0)
			},
		},
		"kick-2": CVariantInstrument{
			Name:   "kick-2",
			Render: renderKick,
			Beats:  0.5,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 7)
				softClip(buf, 1.8, 0.8)
			},
		},
		"hihat-2": CVariantInstrument{
			Name:   "hihat-2",
			Render: renderHiHat,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 5)
				hpFilter(buf, sr, 5000.0)
			},
		},
		"tom-2": CVariantInstrument{
			Name:   "tom-2",
			Render: renderTomLow,
			Beats:  0.7, // Longer for low tom ring.
			Post: func(buf []float32, sr int) {
				// Very gentle saturation - preserve natural warmth.
				softClip(buf, 1.15, 0.95)
			},
		},
		"clap-2": CVariantInstrument{
			Name:   "clap-2",
			Render: renderClap,
			Beats:  0.3,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 6)
				softClip(buf, 1.7, 0.85)
			},
		},
		"cowbell-2": CVariantInstrument{
			Name:   "cowbell-2",
			Render: renderCowbell,
			Beats:  0.5,
			Post: func(buf []float32, sr int) {
				crushBits(buf, 6)
				hpFilter(buf, sr, 800.0)
			},
		},

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
	}
	instOrder = []string{
		"snare", "kick", "hihat", "tom", "clap", "cowbell",
		"bass-guitar", "sub-bass",
		"snare-1", "kick-1", "hihat-1", "tom-1", "clap-1", "cowbell-1",
		"bass-guitar-1", "sub-bass-1",
		"snare-2", "kick-2", "hihat-2", "tom-2", "clap-2", "cowbell-2",
	}
	instMu.Unlock()
	bumpInstrumentsVersion()
	resetInstrumentChannels(instOrder)
}
