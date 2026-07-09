package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// PlainEnglish returns a kid-readable, one-line gloss for a UI label that
// otherwise carries an audio-engineering term (Attack, Cutoff, Q, etc.).
// Always-on; callers render the result as a subtitle below the jargon
// label at ~0.7× the body font size, the same way the Synth tab already
// renders section subtitles via sectionSubtitle().
//
// Returns "" when the label has no plain-English mapping — callers treat
// that as "no subtitle, skip the row". Lookup is case-insensitive and
// trims a trailing " (dB)" / " (Hz)" / " %" suffix before matching so
// renderers can pass display labels verbatim without stripping units.
//
// Mappings stay narrow and audio-pedagogy focused. Anything that turns
// into a sentence longer than ~24 characters belongs in the per-tab
// legend chip (see audio_panel_legend.go), not here.
func PlainEnglish(label string) string {
	key := plainEnglishKey(label)
	if k, ok := plainEnglishMap[key]; ok {
		return i18n.T(k)
	}
	return ""
}

// plainEnglishKey normalises a label for case- and units-insensitive
// lookup. Strips leading/trailing whitespace, lowercases, and removes a
// trailing " (dB)" / " (Hz)" / " %" suffix.
func plainEnglishKey(label string) string {
	s := strings.TrimSpace(strings.ToLower(label))
	for _, suf := range []string{" (db)", " (hz)", " (khz)", " (s)", " (ms)", " %", " hz", " db", " khz", " ms"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSpace(s[:len(s)-len(suf)])
		}
	}
	return s
}

// plainEnglishMap stores the canonical kid-friendly gloss for each
// supported jargon term. Keep entries short (≤ 24 chars), lowercase the
// keys; the lookup normalises input case + units suffix.
//
// Section-card subtitles already live in sectionSubtitle() (synth_panel
// _zone.go) — those are duplicated here so the same helper can serve
// any callsite that wants to apply the gloss to an individual knob
// label without going through the section-card path.
var plainEnglishMap = map[string]i18n.Key{
	"sax blow":           i18n.KeyGlossSaxBlow,
	"sax reed offset":    i18n.KeyGlossSaxReed,
	"sax reed stiffness": i18n.KeyGlossSaxReedStiff,
	"sax bell":           i18n.KeyGlossSaxBell,
	"sax breath":         i18n.KeyGlossSaxBreath,
	"sax brightness":     i18n.KeyGlossSaxBright,
	"bow position":       i18n.KeyGlossBowPos,
	"bow pressure":       i18n.KeyGlossBowPress,
	"bow speed":          i18n.KeyGlossBowSpeed,
	"bow brightness":     i18n.KeyGlossBowBright,
	"oscillator":         i18n.KeyGlossOscillator,
	"octave":             i18n.KeyGlossOctave,
	"algorithm":          i18n.KeyGlossAlgorithm,
	"op1 ratio":          i18n.KeyGlossOp1Ratio,
	"op2 ratio":          i18n.KeyGlossOp2Ratio,
	"op3 ratio":          i18n.KeyGlossOp3Ratio,
	"op4 ratio":          i18n.KeyGlossOp4Ratio,
	"op1 depth":          i18n.KeyGlossOp1Depth,
	"op2 depth":          i18n.KeyGlossOp2Depth,
	"op3 depth":          i18n.KeyGlossOp3Depth,
	"op4 depth":          i18n.KeyGlossOp4Depth,
	"op1 level":          i18n.KeyGlossOp1Level,
	"op2 level":          i18n.KeyGlossOp2Level,
	"op3 level":          i18n.KeyGlossOp3Level,
	"op4 level":          i18n.KeyGlossOp4Level,
	"curve":              i18n.KeyGlossCurve,
	"filter env":         i18n.KeyGlossFilterEnv,
	"env amount":         i18n.KeyGlossEnvAmount,
	"env rise":           i18n.KeyGlossEnvRise,
	"env decay":          i18n.KeyGlossEnvDecay,
	"lfo":                i18n.KeyGlossLfo,
	"rate":               i18n.KeyGlossRate,
	"depth":              i18n.KeyGlossDepth,
	"lfo target":         i18n.KeyGlossLfoTarget,
	"vibrato delay":      i18n.KeyGlossVibratoDelay,
	"pitch env":          i18n.KeyGlossPitchEnv,
	"sweep":              i18n.KeyGlossSweep,
	"burst":              i18n.KeyGlossBurst,
	"sharpness":          i18n.KeyGlossSharpness,
	"hit 1 time":         i18n.KeyGlossHit1Time,
	"hit 2 time":         i18n.KeyGlossHit2Time,
	"hit 3 time":         i18n.KeyGlossHit3Time,
	"hit 4 time":         i18n.KeyGlossHit4Time,
	"hit 1 level":        i18n.KeyGlossHit1Level,
	"hit 2 level":        i18n.KeyGlossHit2Level,
	"hit 3 level":        i18n.KeyGlossHit3Level,
	"hit 4 level":        i18n.KeyGlossHit4Level,
	"voices":             i18n.KeyGlossVoices,
	"mix":                i18n.KeyGlossMix,
	"voice drift":        i18n.KeyGlossVoiceDrift,
	"drift depth":        i18n.KeyGlossDriftDepth,
	"kick":               i18n.KeyGlossKick,
	"variant":            i18n.KeyGlossVariant,
	"harmonic decay":     i18n.KeyGlossHarmonicDecay,
	"drop speed":         i18n.KeyGlossDropSpeed,
	"pitch drop":         i18n.KeyGlossPitchDrop,
	"mode split":         i18n.KeyGlossModeSplit,
	"mode body":          i18n.KeyGlossModeBody,
	"mode ring":          i18n.KeyGlossModeRing,
	"tail":               i18n.KeyGlossTail,
	"fm":                 i18n.KeyGlossFm,
	"env":                i18n.KeyGlossEnv,
	// Phase-15 voice/choir knobs (Task 8): FORMANT + ENSEMBLE + RESONATOR.
	"formant":       i18n.KeyGlossFormant,
	"vowel":         i18n.KeyGlossVowel,
	"voice type":    i18n.KeyGlossVoiceType,
	"breath":        i18n.KeyGlossBreath,
	"shine":         i18n.KeyGlossShine,
	"head size":     i18n.KeyGlossHeadSize,
	"morph speed":   i18n.KeyGlossMorphSpeed,
	"morph to":      i18n.KeyGlossMorphTo,
	"scatter":       i18n.KeyGlossScatter,
	"vibrato rate":  i18n.KeyGlossVibratoRate,
	"vibrato depth": i18n.KeyGlossVibratoDepth,
	"humanize":      i18n.KeyGlossHumanize,
	"body model":    i18n.KeyGlossBodyModel,
	"body mix":      i18n.KeyGlossBodyMix,
	"bow dynamics":  i18n.KeyGlossBowDynamics,
	// Phase-16 voice-realism knobs: ens_jitter (Roughness) + formant_dry (Dry Blend).
	"roughness": i18n.KeyGlossRoughness,
	"dry blend": i18n.KeyGlossDryBlend,
	// Synth pipeline sections (Phase 8B unified Synth tab).
	"voice": i18n.KeyGlossVoice,
	"osc":   i18n.KeyGlossOsc,

	// Pitch
	"pitch":     i18n.KeyGlossPitch,
	"waveform":  i18n.KeyGlossWaveform,
	"generator": i18n.KeyGlossGenerator,
	"tune":      i18n.KeyGlossTune,
	"glide":     i18n.KeyGlossGlide,
	"bend":      i18n.KeyGlossBend,
	"detune":    i18n.KeyGlossDetune,

	// Envelope
	"envelope": i18n.KeyGlossEnvelope,
	"attack":   i18n.KeyGlossAttack,
	"decay":    i18n.KeyGlossDecay,
	"sustain":  i18n.KeyGlossSustain,
	"release":  i18n.KeyGlossRelease,

	// Tone / filter
	"tone":      i18n.KeyGlossTone,
	"cutoff":    i18n.KeyGlossCutoff,
	"filter":    i18n.KeyGlossFilter,
	"reso":      i18n.KeyGlossReso,
	"resonance": i18n.KeyGlossResonance,
	"body":      i18n.KeyGlossBody,

	// FM family knobs (native-deprecation migration). Op-numbered labels
	// normalise through plainEnglishKey verbatim (no unit suffix), so the
	// keys carry the full lowercase label.
	"base pitch":    i18n.KeyGlossBasePitch,
	"pitch sweep":   i18n.KeyGlossPitchSweep,
	"sweep time":    i18n.KeyGlossSweepTime,
	"op 1 ratio":    i18n.KeyGlossOpRatio,
	"op 2 ratio":    i18n.KeyGlossOpRatio,
	"op 3 ratio":    i18n.KeyGlossOpRatio,
	"op 4 ratio":    i18n.KeyGlossOpRatio,
	"op 1 fm depth": i18n.KeyGlossOpFMDepth,
	"op 2 fm depth": i18n.KeyGlossOpFMDepth,
	"op 3 fm depth": i18n.KeyGlossOpFMDepth,
	"op 4 fm depth": i18n.KeyGlossOpFMDepth,
	"op 1 decay":    i18n.KeyGlossOpDecay,
	"op 2 decay":    i18n.KeyGlossOpDecay,
	"op 3 decay":    i18n.KeyGlossOpDecay,
	"op 4 decay":    i18n.KeyGlossOpDecay,

	// Kick family knobs (native-deprecation migration).
	"fundamental":  i18n.KeyGlossFundamental,
	"sweep speed":  i18n.KeyGlossSweepSpeed,
	"boom decay":   i18n.KeyGlossBoomDecay,
	"body decay":   i18n.KeyGlossBodyDecay,
	"2nd harmonic": i18n.KeyGloss2ndHarmonic,
	"3rd harmonic": i18n.KeyGloss3rdHarmonic,
	"4th harmonic": i18n.KeyGloss4thHarmonic,
	"click":        i18n.KeyGlossClick,
	"thud":         i18n.KeyGlossThud,

	// Tom family knobs.
	"ring decay": i18n.KeyGlossRingDecay,
	"overtone 1": i18n.KeyGlossOvertone1,
	"overtone 2": i18n.KeyGlossOvertone2,
	"stick":      i18n.KeyGlossStick,
	"room":       i18n.KeyGlossRoom,

	// Snare family knobs.
	"tone 2":      i18n.KeyGlossTone2,
	"noise tune":  i18n.KeyGlossNoiseTune,
	"tone decay":  i18n.KeyGlossToneDecay,
	"noise decay": i18n.KeyGlossNoiseDecay,
	"tail decay":  i18n.KeyGlossTailDecay,
	"tone level":  i18n.KeyGlossToneLevel,
	"noise level": i18n.KeyGlossNoiseLevel,
	"wires":       i18n.KeyGlossWires,
	"snap":        i18n.KeyGlossSnap,

	// Cymbal family knobs.
	"metal tune":   i18n.KeyGlossMetalTune,
	"attack decay": i18n.KeyGlossAttackDecay,
	"metal level":  i18n.KeyGlossMetalLevel,
	"sizzle level": i18n.KeyGlossSizzleLevel,
	"sizzle decay": i18n.KeyGlossSizzleDecay,

	// Bass family knobs.
	"pluck":       i18n.KeyGlossPluck,
	"pick":        i18n.KeyGlossPick,
	"fade":        i18n.KeyGlossFade,
	"overtone":    i18n.KeyGlossOvertone,
	"pitch punch": i18n.KeyGlossPitchPunch,
	"punch":       i18n.KeyGlossPunch,

	// Drive / dynamics
	"drive":      i18n.KeyGlossDrive,
	"gain":       i18n.KeyGlossGain,
	"saturation": i18n.KeyGlossSaturation,
	"sat":        i18n.KeyGlossSat,
	"distortion": i18n.KeyGlossDistortion,

	// Spectrum / metering
	"slope":    i18n.KeyGlossSlope,
	"slopes":   i18n.KeyGlossSlopes,
	"pre":      i18n.KeyGlossPre,
	"post":     i18n.KeyGlossPost,
	"peak":     i18n.KeyGlossPeak,
	"rms":      i18n.KeyGlossRMS,
	"clip":     i18n.KeyGlossClip,
	"clips":    i18n.KeyGlossClips,
	"headroom": i18n.KeyGlossHeadroom,
	"lufs":     i18n.KeyGlossLUFS,
	"k-20":     i18n.KeyGlossK20,

	// Mode / chain selectors
	"overlay":   i18n.KeyGlossOverlay,
	"split":     i18n.KeyGlossSplit,
	"diff":      i18n.KeyGlossDiff,
	"ovr":       i18n.KeyGlossOvr,
	"spl":       i18n.KeyGlossSpl,
	"dif":       i18n.KeyGlossDif,
	"ag":        i18n.KeyGlossAG,
	"auto gain": i18n.KeyGlossAutoGain,

	// Sends / outs
	"out":    i18n.KeyGlossOut,
	"delay":  i18n.KeyGlossDelay,
	"reverb": i18n.KeyGlossReverb,
	"send":   i18n.KeyGlossSend,

	// Transport
	"play":  i18n.KeyGlossPlay,
	"stop":  i18n.KeyGlossStop,
	"pause": i18n.KeyGlossPause,
}
