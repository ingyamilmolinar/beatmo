package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// "" when unmapped (callers skip the subtitle). Case- and unit-suffix-insensitive so display labels pass
// verbatim; anything longer than ~24 chars belongs in the tab legend instead.
func PlainEnglish(label string) string {
	key := plainEnglishKey(label)
	if k, ok := plainEnglishMap[key]; ok {
		return i18n.T(k)
	}
	// Resolved here so the Chain codes answer to the same app-wide lookup as every tab.
	for code, k := range chainPillGlosses() {
		if plainEnglishKey(code) == key {
			return i18n.T(k)
		}
	}
	return ""
}

func plainEnglishKey(label string) string {
	s := strings.TrimSpace(strings.ToLower(label))
	for _, suf := range []string{" (db)", " (hz)", " (khz)", " (s)", " (ms)", " %", " hz", " db", " khz", " ms"} {
		if strings.HasSuffix(s, suf) {
			s = strings.TrimSpace(s[:len(s)-len(suf)])
		}
	}
	return s
}

// Keep entries ≤ 24 chars with lowercase keys. Section subtitles are duplicated here so a single knob label
// can take the gloss without the section-card path.
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
	"snare":              i18n.KeyGlossSnare,
	"variant":            i18n.KeyGlossVariant,
	"harmonic decay":     i18n.KeyGlossHarmonicDecay,
	"drop speed":         i18n.KeyGlossDropSpeed,
	"pitch drop":         i18n.KeyGlossPitchDrop,
	"mode split":         i18n.KeyGlossModeSplit,
	"mode body":          i18n.KeyGlossModeBody,
	"mode ring":          i18n.KeyGlossModeRing,
	"body hold":          i18n.KeyGlossBodyHold,
	"air":                i18n.KeyGlossAir,
	"harmonics":          i18n.KeyGlossHarmonics,
	"punch speed":        i18n.KeyGlossPunchSpeed,
	"level":              i18n.KeyGlossLevel,
	"tail":               i18n.KeyGlossTail,
	"fm":                 i18n.KeyGlossFm,
	"env":                i18n.KeyGlossEnv,
	// Phase-15 voice/choir knobs (Task 8): FORMANT + ENSEMBLE + RESONATOR.
	"formant":       i18n.KeyGlossFormant,
	"modes":         i18n.KeyGlossModal,
	"strike":        i18n.KeyGlossStrike,
	"noise":         i18n.KeyGlossNoiseband,
	"stretch":       i18n.KeyGlossStretch,
	"damping":       i18n.KeyGlossDamping,
	"brightness":    i18n.KeyGlossModalBright,
	"partials":      i18n.KeyGlossPartials,
	"decay rate":    i18n.KeyGlossDecayRate,
	"decay tilt":    i18n.KeyGlossDecayTilt,
	"top damping":   i18n.KeyGlossTopDamping,
	"contact":       i18n.KeyGlossContact,
	"hardness":      i18n.KeyGlossHardness,
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
	"voice":     i18n.KeyGlossVoice,
	"osc":       i18n.KeyGlossOsc,

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

	// FINISH stage (the post ops + output limit).
	"resample":   i18n.KeyGlossResample,
	"fade speed": i18n.KeyGlossFadeSpeed,
	"low boost":  i18n.KeyGlossLowBoost,
	"darken":     i18n.KeyGlossDarken,
	"crunch":     i18n.KeyGlossCrunch,
	"limit":      i18n.KeyGlossLimit,
	"crisp":      i18n.KeyGlossCrisp,

	// Op-numbered labels have no unit suffix, so the keys carry the full lowercase label.
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
	// Cymbal modal-plate knobs (Phase-18).
	"plate modes":   i18n.KeyGlossPlateModes,
	"lowest mode":   i18n.KeyGlossLowestMode,
	"highest mode":  i18n.KeyGlossHighestMode,
	"highs fade":    i18n.KeyGlossHighsFade,
	"tick corner":   i18n.KeyGlossTickCorner,
	"shimmer":       i18n.KeyGlossShimmer,
	"strike tone":   i18n.KeyGlossStrikeTone,
	"strike width":  i18n.KeyGlossStrikeWidth,
	"body tone":     i18n.KeyGlossBodyTone,
	"body width":    i18n.KeyGlossBodyWidth,
	"sparkle":       i18n.KeyGlossSparkle,
	"sparkle tone":  i18n.KeyGlossSparkleTone,
	"sparkle width": i18n.KeyGlossSparkleWidth,
	"wash":          i18n.KeyGlossWash,
	"softness":      i18n.KeyGlossSoftness,
	"rattle":        i18n.KeyGlossRattle,
	"rattle speed":  i18n.KeyGlossRattleSpeed,
	"rattle length": i18n.KeyGlossRattleLength,
	"wire delay":    i18n.KeyGlossWireDelay,
	"buzz hold":     i18n.KeyGlossBuzzHold,

	// Bass family knobs.
	"pluck":       i18n.KeyGlossPluck,
	"pick":        i18n.KeyGlossPick,
	"fade":        i18n.KeyGlossFade,
	"length":      i18n.KeyGlossLength,
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
