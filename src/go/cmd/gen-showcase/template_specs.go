package main

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/smf"
)

// This table is the whole of the template definition. Every built-in circuit is
// one row here: a committed public-domain score, a measure range, and a mapping
// from score parts to Beatmo instruments. There is no per-piece imperative code
// and no hand-entered note anywhere in this package — which is precisely what
// lets TestTemplateIsDerivedFromScore treat the score as an oracle instead of
// as documentation.
//
// Provenance for every ScoreID lives in internal/scoreref/catalogue.json, and
// the survey behind the admission rule is docs/score-sources.md. Adding a
// template is: add a catalogue entry + committed .mxl, add a row here, add the
// stem to internal/assets/templates.go's templateGroups, regenerate.
//
// Octave policy: NONE needed. An instrument that renders away from node pitch 0
// (cello, cello-warm and the basses an octave down; voice-soprano an octave up)
// is cancelled automatically by instrumentPitchShift in score_import.go, which
// asks audio.InstrumentPitchInfo rather than trusting a comment here. The rows
// below used to carry Octave: 1 for that, and the list drifted from the engine:
// double-bass was corrected for an offset it does not have (its contrabass line
// sounded in unison with the cello instead of an octave below it) while
// voice-soprano's real +1 octave was corrected nowhere. Set Octave only to
// re-voice a part deliberately. Transposing instruments (Bb clarinet, F horn)
// need no correction either — internal/musicxml applies <transpose> and reports
// SOUNDING pitch.

// PartMap binds one part of a score to one Beatmo instrument.
//
// Index is authoritative and Name is a checked assertion: several scores carry
// duplicate part names (Boulanger's orchestration has three parts literally
// named "Violoncello"), so selection must be by index — but an index alone
// would silently point at a different instrument if the upstream score were
// ever re-ordered. TestPartMapNamesMatchScore compares the two.
type PartMap struct {
	Index   int           // part index within the score
	Name    string        // expected part name, asserted against the score
	Inst    instrument.ID // Beatmo instrument id
	Display string        // row label

	// Octave shifts the row by whole octaves AFTER sounding pitch is resolved.
	Octave int

	// MaxVoices caps chord splitting: a part is spread across at most this
	// many rows, top note first. 1 keeps only the melody line.
	MaxVoices int

	Volume float64

	// Lowest lanes a part bottom-up: lane 0 takes the LOWEST note at each
	// onset — a bass line — instead of the highest.
	Lowest bool
	// BelowMIDI is a keyboard split: only notes below this MIDI key are kept
	// (0 keeps everything). A ragtime left hand alternates a low bass note with
	// a mid-register chord; splitting below the chords gives a bass row the
	// "oom" without the "pah". Notes above the split are counted, not lost.
	BelowMIDI int
	// SoundingPitch cancels a NON-melodic instrument's whole measured pitch
	// offset so its nodes sound the written pitch — how a timpani part plays
	// on a drum voice. Melodic instruments are always corrected
	// (instrumentPitchShift); this is only for drums asked to play notes.
	SoundingPitch bool
	// Kit routes a percussion part's GM keys to Beatmo instruments, overriding
	// gmKit key by key. Ignored for pitched parts.
	Kit map[int]KitPiece
}

// KitPiece is the Beatmo voice a GM percussion key plays on.
type KitPiece struct {
	Inst    instrument.ID // Beatmo instrument id
	Display string        // row label
	Pitch   float64       // node pitch: a tuning offset on the drum (tom heights)
	// Volume overrides the PartMap's for this piece's row (0 = inherit).
	Volume float64
}

// gmKit is the default General MIDI percussion map. It covers every key a
// score in the corpus plays; a key missing here and from the PartMap's Kit
// fails generation loudly rather than dropping the drum.
var gmKit = map[int]KitPiece{
	35: {Inst: roster.KickAcoustic, Display: "Kick"},
	36: {Inst: roster.Kick, Display: "Kick"},
	37: {Inst: roster.Sidestick, Display: "Side stick"},
	38: {Inst: roster.Snare, Display: "Snare"},
	39: {Inst: roster.Clap, Display: "Clap"},
	40: {Inst: roster.Snare, Display: "Snare"},
	41: {Inst: roster.Tom, Display: "Toms", Pitch: -7},
	42: {Inst: roster.Hihat, Display: "Hi-hat"},
	43: {Inst: roster.Tom, Display: "Toms", Pitch: -5},
	44: {Inst: roster.HihatPedal, Display: "Pedal hat"},
	45: {Inst: roster.Tom, Display: "Toms", Pitch: -3},
	46: {Inst: roster.HihatOpenBright, Display: "Open hat"},
	47: {Inst: roster.Tom, Display: "Toms", Pitch: -1},
	48: {Inst: roster.Tom, Display: "Toms", Pitch: 1},
	49: {Inst: roster.Crash, Display: "Cymbal"},
	50: {Inst: roster.Tom, Display: "Toms", Pitch: 3},
	51: {Inst: roster.Ride, Display: "Ride"},
	52: {Inst: roster.Crash, Display: "Cymbal"},
	53: {Inst: roster.Ride, Display: "Ride"},
	54: {Inst: roster.Shaker, Display: "Tambourine"},
	55: {Inst: roster.Crash, Display: "Cymbal"},
	56: {Inst: roster.Cowbell, Display: "Cowbell"},
	57: {Inst: roster.Crash, Display: "Cymbal"},
	59: {Inst: roster.Ride, Display: "Ride"},
	60: {Inst: roster.Conga, Display: "Bongo", Pitch: 7},
	61: {Inst: roster.Conga, Display: "Bongo", Pitch: 4},
	62: {Inst: roster.Conga, Display: "Conga"},
	63: {Inst: roster.CongaOpen, Display: "Conga open"},
	64: {Inst: roster.CongaTumba, Display: "Tumba"},
	65: {Inst: roster.TomHighAcoustic, Display: "Timbales", Pitch: 3},
	66: {Inst: roster.TomHighAcoustic, Display: "Timbales"},
	67: {Inst: roster.Cowbell, Display: "Agogô", Pitch: 3},
	68: {Inst: roster.Cowbell, Display: "Agogô"},
	69: {Inst: roster.Shaker, Display: "Shaker"},
	70: {Inst: roster.Shaker, Display: "Maracas"},
	73: {Inst: roster.Shaker, Display: "Güiro"},
	74: {Inst: roster.Shaker, Display: "Güiro"},
	75: {Inst: roster.Sidestick, Display: "Clave"},
	76: {Inst: roster.Rimshot, Display: "Wood block", Pitch: 2},
	77: {Inst: roster.Rimshot, Display: "Wood block"},
	80: {Inst: roster.FmBellSoft, Display: "Triangle", Pitch: 15},
	81: {Inst: roster.FmBellSoft, Display: "Triangle", Pitch: 15},
}

// kitPieceFor resolves a GM key through the part's Kit, then gmKit.
func kitPieceFor(pm PartMap, key int) (KitPiece, bool) {
	if kp, ok := pm.Kit[key]; ok {
		return kp, true
	}
	kp, ok := gmKit[key]
	return kp, ok
}

func gmName(key int) string { return smf.GMPercussionName(key) }

// GrooveLayer is a percussion score looped under a template's melody. Its
// length must divide the melody excerpt's, and a swing it marks is the whole
// band's feel: the melody is swung with it.
type GrooveLayer struct {
	ScoreID  string
	From, To int
	Parts    []PartMap
}

// TemplateSpec is one generated template.
type TemplateSpec struct {
	Stem    string
	ScoreID string

	// From/To are printed measure numbers, 1-based inclusive, in the score's
	// own numbering and BEFORE any repeat expansion (repeats are not expanded —
	// see internal/musicxml).
	From, To int

	Subdiv int
	// Movement selects which movement of a multi-movement score From/To count
	// within; 0 means the first. Measure numbering restarts per movement, so
	// From/To alone is ambiguous in any published quartet or symphony.
	Movement int
	// BPM overrides the score's own tempo mark; 0 takes the score's. A groove
	// template sets it: the genre decides the tempo, not the melody's edition.
	BPM int

	Parts []PartMap

	// Groove, when set, loops a percussion score under the melody.
	Groove *GrooveLayer
}

// str/qt/song are shorthand for the recurring instrument assignments, kept so
// the table below reads as repertoire rather than as configuration.
func strings4(v1, v2, va, vc int, n1, n2, na, nc string) []PartMap {
	return []PartMap{
		{Index: v1, Name: n1, Inst: roster.Violin, Display: "Violin I", MaxVoices: 2, Volume: 0.80},
		{Index: v2, Name: n2, Inst: roster.ViolinEnsemble, Display: "Violin II", MaxVoices: 1, Volume: 0.70},
		{Index: va, Name: na, Inst: roster.Viola, Display: "Viola", MaxVoices: 1, Volume: 0.72},
		{Index: vc, Name: nc, Inst: roster.Cello, Display: "Cello", MaxVoices: 1, Volume: 0.78},
	}
}

func voicePiano(voiceName, pianoName string, voiceInst instrument.ID) []PartMap {
	return []PartMap{
		{Index: 0, Name: voiceName, Inst: voiceInst, Display: "Voice", MaxVoices: 1, Volume: 0.82},
		{Index: 1, Name: pianoName, Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 3, Volume: 0.68},
	}
}

// timpani maps an orchestral timpani part onto the tom voice at the written
// pitch (SoundingPitch cancels the tom's own tuning).
func timpani(index int, name string) PartMap {
	return PartMap{Index: index, Name: name, Inst: roster.Tom, Display: "Timpani", MaxVoices: 1,
		SoundingPitch: true, Volume: 0.70}
}

// drumset maps a Beatmo groove score's single drum part.
func drumset(vol float64, kit map[int]KitPiece) []PartMap {
	return []PartMap{{Index: 0, Name: "Drumset", Display: "Drums", Volume: vol, Kit: kit}}
}

// groove loops the whole of a Beatmo groove score (bars 1..bars).
func groove(id string, bars int, vol float64, kit map[int]KitPiece) *GrooveLayer {
	return &GrooveLayer{ScoreID: id, From: 1, To: bars, Parts: drumset(vol, kit)}
}

// Drum-machine and kit flavours for the famous-beat grooves: the GM keys a
// drum-patterns.com grid uses, voiced as the record's era voiced them.
var (
	kitAcoustic = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickAcoustic, Display: "Kick"},
		38: {Inst: roster.SnareAcoustic, Display: "Snare"},
		37: {Inst: roster.Sidestick, Display: "Side stick"},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.55},
		49: {Inst: roster.Crash, Display: "Cymbal", Volume: 0.55},
	}
	kitRock = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickRock, Display: "Kick"},
		38: {Inst: roster.SnareFat, Display: "Snare"},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.55},
		49: {Inst: roster.Crash, Display: "Cymbal", Volume: 0.55},
	}
	kit808 = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.Kick808, Display: "808 kick"},
		38: {Inst: roster.Snare, Display: "Snare"},
		39: {Inst: roster.Clap, Display: "Clap"},
		37: {Inst: roster.Rimshot, Display: "Rimshot", Volume: 0.6},
		42: {Inst: roster.HihatLofi, Display: "Hi-hat", Volume: 0.55},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
		49: {Inst: roster.Crash, Display: "Cymbal", Volume: 0.5},
		56: {Inst: roster.Cowbell, Display: "Cowbell", Volume: 0.5},
	}
	kit909 = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickPunchy, Display: "Kick"},
		38: {Inst: roster.SnareCrack, Display: "Snare"},
		39: {Inst: roster.ClapBright, Display: "Clap"},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
		49: {Inst: roster.Ride, Display: "Ride", Volume: 0.5},
	}
	kitLinn = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickTight, Display: "Kick"},
		38: {Inst: roster.SnarePunch, Display: "Snare"},
		39: {Inst: roster.ClapTight, Display: "Clap"},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
		49: {Inst: roster.Crash, Display: "Cymbal", Volume: 0.5},
	}
	kitIndustrial = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickBrutal, Display: "Kick"},
		38: {Inst: roster.SnareCrack, Display: "Snare"},
		39: {Inst: roster.ClapLofi, Display: "Clap"},
		42: {Inst: roster.HihatLofi, Display: "Hi-hat", Volume: 0.55},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
		49: {Inst: roster.Crash, Display: "Cymbal", Volume: 0.5},
		56: {Inst: roster.CowbellDrive, Display: "Cowbell", Volume: 0.5},
	}
	kitIDM = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.KickSnap, Display: "Kick"},
		38: {Inst: roster.SnareDeep, Display: "Snare"},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.5},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.45},
	}
	kitHyper = map[int]KitPiece{
		62: {Inst: roster.Conga, Display: "Congas"},
		63: {Inst: roster.Conga, Display: "Congas", Pitch: 3},
		64: {Inst: roster.Conga, Display: "Congas", Pitch: -5},
		36: {Inst: roster.Kick808, Display: "808 kick"},
		38: {Inst: roster.SnareCrack, Display: "Snare"},
		39: {Inst: roster.ClapBright, Display: "Clap"},
		37: {Inst: roster.Rimshot, Display: "Rimshot", Volume: 0.6},
		42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.5},
		46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.45},
	}
)

// templateSpecs is the menu, in menu order. The grouping that drives the
// template menu's section headers lives in internal/assets/templates.go; the
// order here must agree with it (TestTemplateOrderMatchesAssets).
func templateSpecs() []TemplateSpec {
	return []TemplateSpec{
		// ── Funk · Soul · Jazz ────────────────────────────────────────────
		// Famous beats, encoded from published step transcriptions (rhythm
		// patterns), under original Beatmo tunes.
		{
			Stem: "funky-drummer", ScoreID: "beatmo-cold-coffee",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Clavinet", Inst: roster.Clavinet, Display: "Clavinet", MaxVoices: 1, Volume: 0.72},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-funky-drummer", 2, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickAcoustic, Display: "Kick"},
				38: {Inst: roster.SnareAcoustic, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.55},
			}),
		},
		{
			Stem: "impeach-the-president", ScoreID: "beatmo-brownstone",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Keys", Inst: roster.PianoRhodes, Display: "Keys", MaxVoices: 2, Volume: 0.68},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
				{Index: 2, Name: "Lead", Inst: roster.Vibraphone, Display: "Vibes", MaxVoices: 1, Volume: 0.7},
			},
			Groove: groove("groove-impeach-the-president", 1, 0.82, map[int]KitPiece{
				36: {Inst: roster.KickBoomBap, Display: "Kick"},
				38: {Inst: roster.SnareLofi, Display: "Snare"},
				42: {Inst: roster.HihatLofi, Display: "Hi-hat", Volume: 0.6},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
			}),
		},
		{
			Stem: "superstition-groove", ScoreID: "beatmo-copper-wire",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sax", Inst: roster.Sax, Display: "Sax", MaxVoices: 1, Volume: 0.74},
				{Index: 1, Name: "Clav", Inst: roster.Clavinet, Display: "Clav", MaxVoices: 2, Volume: 0.62},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-superstition", 4, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickAcoustic, Display: "Kick"},
				38: {Inst: roster.SnareAcoustic, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
			}),
		},
		{
			Stem: "hook-and-sling", ScoreID: "beatmo-crescent-city",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sax", Inst: roster.Sax, Display: "Sax", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Organ", Inst: roster.Organ, Display: "Organ", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-hook-and-sling", 4, 0.8, kitAcoustic),
		},
		{
			Stem: "look-ka-py-py", ScoreID: "beatmo-second-line",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Organ", Inst: roster.Organ, Display: "Organ", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-look-ka-py-py", 2, 0.8, kitAcoustic),
		},
		{
			Stem: "synthetic-substitution", ScoreID: "beatmo-velvet-sample",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Trumpet", Inst: roster.TrumpetMellow, Display: "Trumpet", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Rhodes", Inst: roster.PianoRhodes, Display: "Rhodes", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-synthetic-substitution", 2, 0.8, kitAcoustic),
		},
		{
			Stem: "haitian-divorce", ScoreID: "beatmo-island-letter",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectricNeck, Display: "Guitar", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Rhodes", Inst: roster.PianoRhodes, Display: "Rhodes", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-haitian-divorce", 2, 0.8, kitAcoustic),
		},
		{
			Stem: "palm-grease", ScoreID: "beatmo-thrust",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "E-piano", Inst: roster.FmEpiano, Display: "E-piano", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Clavinet", Inst: roster.Clavinet, Display: "Clavinet", MaxVoices: 1, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-palm-grease", 2, 0.8, kitAcoustic),
		},
		{
			Stem: "expensive-shit", ScoreID: "beatmo-shrine",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Horns", Inst: roster.Sax, Display: "Horns", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 1, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-expensive-shit", 2, 0.8, kitAcoustic),
		},
		{
			Stem: "jazz-swing", ScoreID: "beatmo-blue-lantern",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Walking bass", Inst: roster.DoubleBass, Display: "Bass", MaxVoices: 1, Volume: 0.78},
			},
			Groove: groove("groove-jazz-swing", 2, 0.72, map[int]KitPiece{
				51: {Inst: roster.Ride, Display: "Ride"},
				44: {Inst: roster.HihatPedal, Display: "Hi-hat foot", Volume: 0.55},
				35: {Inst: roster.KickAcoustic, Display: "Kick"},
				38: {Inst: roster.SnareAcoustic, Display: "Snare"},
			}),
		},

		// ── Rock · Post-punk ──────────────────────────────────────────────
		{
			Stem: "we-will-rock-you-stomp", ScoreID: "beatmo-stadium-lights",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 2, Volume: 0.68},
				{Index: 1, Name: "Chant", Inst: roster.VoiceAhh, Display: "Chant", MaxVoices: 1, Volume: 0.7},
			},
			Groove: groove("groove-we-will-rock-you", 1, 0.85, map[int]KitPiece{
				36: {Inst: roster.KickDeep, Display: "Stomp"},
				70: {Inst: roster.Clap, Display: "Clap"},
			}),
		},
		{
			Stem: "rosanna-shuffle", ScoreID: "beatmo-harbor-lights",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 2, Volume: 0.66},
				{Index: 1, Name: "Brass", Inst: roster.BrassStab, Display: "Brass", MaxVoices: 1, Volume: 0.66},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-rosanna-shuffle", 2, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickRock, Display: "Kick"},
				38: {Inst: roster.SnareFat, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
				49: {Inst: roster.Crash, Display: "Crash", Volume: 0.45},
			}),
		},
		{
			Stem: "in-the-air-tonight", ScoreID: "beatmo-midnight-radar",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.56},
				{Index: 1, Name: "Lead", Inst: roster.EnsembleLeadDark, Display: "Lead", MaxVoices: 1, Volume: 0.66},
			},
			Groove: groove("groove-in-the-air-tonight", 2, 0.82, map[int]KitPiece{
				36: {Inst: roster.KickDeep, Display: "Kick"},
				38: {Inst: roster.SnareFat, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
				50: {Inst: roster.Tom, Display: "Toms", Pitch: 3},
			}),
		},
		{
			Stem: "levee-breaks-rock", ScoreID: "beatmo-iron-river",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-levee-breaks", 1, 0.86, map[int]KitPiece{
				36: {Inst: roster.KickAcousticHeavy, Display: "Kick"},
				38: {Inst: roster.SnareFat, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
			}),
		},
		{
			Stem: "immigrant-song", ScoreID: "beatmo-north-sea",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceOpera, Display: "Voice", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 1, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-immigrant-song", 8, 0.8, kitRock),
		},
		{
			Stem: "hit-me-with-your-rhythm-stick", ScoreID: "beatmo-stagger-lee",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sax", Inst: roster.Sax, Display: "Sax", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-hit-me-with-your-rhythm-stick", 8, 0.8, kitRock),
		},
		{
			Stem: "dirty-love", ScoreID: "beatmo-crooked-hall",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.FmLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 1, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-dirty-love", 8, 0.8, kitRock),
		},
		{
			Stem: "rock-lobster", ScoreID: "beatmo-tide-pool",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceAlien, Display: "Voice", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Guitar", Inst: roster.GuitarElectricNeck, Display: "Guitar", MaxVoices: 1, Volume: 0.7},
				{Index: 2, Name: "Organ", Inst: roster.Organ, Display: "Organ", MaxVoices: 2, Volume: 0.55},
			},
			Groove: groove("groove-rock-lobster", 4, 0.8, kitLinn),
		},
		{
			Stem: "peek-a-boo", ScoreID: "beatmo-yes-man",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Synth", Inst: roster.LeadChip, Display: "Synth", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-peek-a-boo", 8, 0.8, kitLinn),
		},
		{
			Stem: "scentless-apprentice", ScoreID: "beatmo-perfume-factory",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 2, Volume: 0.7},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
				{Index: 2, Name: "Feedback", Inst: roster.GuitarElectricNeck, Display: "Feedback", MaxVoices: 1, Volume: 0.55},
			},
			Groove: groove("groove-scentless-apprentice", 4, 0.8, kitRock),
		},
		{
			Stem: "iron-man", ScoreID: "beatmo-cast-iron",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarElectric, Display: "Guitar", MaxVoices: 2, Volume: 0.7},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
				{Index: 2, Name: "Lead", Inst: roster.GuitarElectricNeck, Display: "Lead", MaxVoices: 1, Volume: 0.55},
			},
			Groove: groove("groove-iron-man", 8, 0.8, kitRock),
		},

		// ── Industrial · Synth ────────────────────────────────────────────
		{
			Stem: "dig-it", ScoreID: "beatmo-rust-belt",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Hoover", Inst: roster.LeadHoover, Display: "Hoover", MaxVoices: 1, Volume: 0.6},
				{Index: 1, Name: "Stab", Inst: roster.BrassStab, Display: "Stab", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.FmBassDrive, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-dig-it", 2, 0.8, kitIndustrial),
		},
		{
			Stem: "achtung", ScoreID: "beatmo-parade-ground",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Horns", Inst: roster.FrenchHornLoud, Display: "Horns", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Choir", Inst: roster.VoiceAhh, Display: "Choir", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-achtung", 8, 0.8, kitIndustrial),
		},
		{
			Stem: "down-in-it", ScoreID: "beatmo-static-room",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.ScifiLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.FmBassDrive, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-down-in-it", 4, 0.8, kitIndustrial),
		},
		{
			Stem: "being-boiled", ScoreID: "beatmo-boiling-point",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.FmLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Sequence", Inst: roster.MoogBass, Display: "Sequence", MaxVoices: 1, Volume: 0.8},
				{Index: 2, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
			},
			Groove: groove("groove-being-boiled", 1, 0.8, kit808),
		},
		{
			Stem: "1000-knives", ScoreID: "beatmo-paper-lantern",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Koto", Inst: roster.Koto, Display: "Koto", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.FmBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-1000-knives", 1, 0.8, kit808),
		},
		{
			Stem: "raised-by-snakes", ScoreID: "beatmo-snake-oil",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.LeadChip, Display: "Lead", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
				{Index: 2, Name: "Pad", Inst: roster.ModularPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
			},
			Groove: groove("groove-raised-by-snakes", 8, 0.8, kitLinn),
		},
		{
			Stem: "problemes-damour", ScoreID: "beatmo-love-letters",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.FmLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Chords", Inst: roster.PwmPad, Display: "Chords", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.FmBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-problemes-damour", 8, 0.8, kit808),
		},
		{
			Stem: "e-mc2", ScoreID: "beatmo-relativity",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.EnsembleLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Sequencer", Inst: roster.MoogBass, Display: "Sequencer", MaxVoices: 1, Volume: 0.72},
				{Index: 2, Name: "Pad", Inst: roster.ModularPad, Display: "Pad", MaxVoices: 2, Volume: 0.48},
			},
			Groove: groove("groove-e-mc2", 8, 0.8, kit808),
		},
		{
			Stem: "blue-monday-synthpop", ScoreID: "beatmo-cold-signal",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sequence", Inst: roster.MoogBass, Display: "Sequence", MaxVoices: 1, Volume: 0.74},
				{Index: 1, Name: "Lead", Inst: roster.EnsembleLead, Display: "Lead", MaxVoices: 1, Volume: 0.68},
			},
			Groove: groove("groove-blue-monday", 2, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickElectro, Display: "Kick"},
				38: {Inst: roster.Snare, Display: "Snare"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
			}),
		},

		// ── Hip-hop · Electro ─────────────────────────────────────────────
		{
			Stem: "planet-rock-electro", ScoreID: "beatmo-satellite-kids",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.ScifiLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Bass", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.82},
			},
			Groove: groove("groove-planet-rock", 1, 0.8, map[int]KitPiece{
				36: {Inst: roster.Kick808, Display: "808 kick"},
				38: {Inst: roster.Snare, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				56: {Inst: roster.Cowbell, Display: "Cowbell", Volume: 0.5},
				70: {Inst: roster.Clap, Display: "Clap"},
			}),
		},
		{
			Stem: "needle-to-the-groove", ScoreID: "beatmo-cut-and-paste",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.ScifiLead, Display: "Lead", MaxVoices: 1, Volume: 0.66},
				{Index: 1, Name: "Stab", Inst: roster.BrassStab, Display: "Stab", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-needle-to-the-groove", 1, 0.8, kit808),
		},
		{
			Stem: "egypt-egypt", ScoreID: "beatmo-pyramid-scheme",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sitar", Inst: roster.Sitar, Display: "Sitar", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Robot", Inst: roster.VoiceAlien, Display: "Robot", MaxVoices: 1, Volume: 0.55},
				{Index: 2, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-egypt-egypt", 1, 0.8, kit808),
		},
		{
			Stem: "buffalo-gals", ScoreID: "beatmo-square-dance",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Whistle", Inst: roster.Flute, Display: "Whistle", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Stab", Inst: roster.BrassStab, Display: "Stab", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.FmBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-buffalo-gals", 8, 0.8, kitLinn),
		},
		{
			Stem: "clear", ScoreID: "beatmo-glass-city",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.FmLead, Display: "Lead", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Arp", Inst: roster.LeadChip, Display: "Arp", MaxVoices: 1, Volume: 0.48},
				{Index: 2, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-clear", 8, 0.8, kit808),
		},
		{
			Stem: "you-got-me", ScoreID: "beatmo-late-pass",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceAhh, Display: "Voice", MaxVoices: 1, Volume: 0.66},
				{Index: 1, Name: "Rhodes", Inst: roster.PianoRhodes, Display: "Rhodes", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.GhostBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-you-got-me", 8, 0.8, kitAcoustic),
		},
		{
			// Subdiv 32: the hi-hat rolls are 32nds and sixteenth triplets.
			Stem: "trap-808", ScoreID: "beatmo-marble-floors",
			From: 1, To: 8, Subdiv: 32,
			Parts: []PartMap{
				{Index: 0, Name: "Flute", Inst: roster.Flute, Display: "Flute", MaxVoices: 1, Volume: 0.72},
				{Index: 1, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.84},
			},
			Groove: groove("groove-trap", 2, 0.82, map[int]KitPiece{
				36: {Inst: roster.Kick808, Display: "808 kick"},
				38: {Inst: roster.SnareCrack, Display: "Snare"},
				39: {Inst: roster.ClapTight, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.6},
			}),
		},

		// ── House · Techno ────────────────────────────────────────────────
		{
			Stem: "on-and-on", ScoreID: "beatmo-first-record",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Organ", Inst: roster.Organ, Display: "Organ", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-on-and-on", 8, 0.8, kit808),
		},
		{
			Stem: "can-you-feel-it", ScoreID: "beatmo-deep-end",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Rhodes", Inst: roster.PianoRhodes, Display: "Rhodes", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.SubBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-can-you-feel-it", 8, 0.8, kit909),
		},
		{
			Stem: "no-way-back", ScoreID: "beatmo-no-exit",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Acid", Inst: roster.BassAcid, Display: "Acid", MaxVoices: 1, Volume: 0.74},
				{Index: 1, Name: "Stab", Inst: roster.Organ, Display: "Stab", MaxVoices: 2, Volume: 0.55},
			},
			Groove: groove("groove-no-way-back", 8, 0.8, kit808),
		},
		{
			Stem: "french-kiss", ScoreID: "beatmo-sixteen-minutes",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "E-piano", Inst: roster.FmEpiano, Display: "E-piano", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Whisper", Inst: roster.VoiceWhisper, Display: "Whisper", MaxVoices: 1, Volume: 0.5},
				{Index: 3, Name: "Bass", Inst: roster.MoogBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-french-kiss", 1, 0.8, kit808),
		},
		{
			Stem: "dominatrix-sleeps-tonight", ScoreID: "beatmo-chrome-heart",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Glock", Inst: roster.Glockenspiel, Display: "Glock", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Voice", Inst: roster.VoiceAhh, Display: "Voice", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.FmBass, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-dominatrix-sleeps-tonight", 8, 0.8, kitLinn),
		},
		{
			Stem: "voodoo-ray", ScoreID: "beatmo-black-magic",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceSoprano, Display: "Voice", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Acid", Inst: roster.BassAcid, Display: "Acid", MaxVoices: 1, Volume: 0.72},
				{Index: 2, Name: "Pad", Inst: roster.ModularPad, Display: "Pad", MaxVoices: 2, Volume: 0.48},
			},
			Groove: groove("groove-voodoo-ray", 4, 0.8, kit808),
		},
		{
			Stem: "north-american-scum", ScoreID: "beatmo-scum-city",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Lead", Inst: roster.LeadSupersaw, Display: "Lead", MaxVoices: 1, Volume: 0.6},
				{Index: 1, Name: "Stab", Inst: roster.BrassStab, Display: "Stab", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-north-american-scum", 4, 0.8, kitLinn),
		},
		{
			Stem: "da-funk-house", ScoreID: "beatmo-chrome-strut",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Acid", Inst: roster.BassAcid, Display: "Acid", MaxVoices: 1, Volume: 0.74},
				{Index: 1, Name: "Chords", Inst: roster.FmEpiano, Display: "Chords", MaxVoices: 2, Volume: 0.6},
				{Index: 2, Name: "Lead", Inst: roster.LeadSupersaw, Display: "Lead", MaxVoices: 1, Volume: 0.62},
			},
			Groove: groove("groove-da-funk", 4, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickPunchy, Display: "Kick"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				45: {Inst: roster.Tom, Display: "Toms", Pitch: -3},
				47: {Inst: roster.Tom, Display: "Toms", Pitch: -1},
			}),
		},
		{
			Stem: "uk-garage-2step", ScoreID: "beatmo-late-train",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Organ", Inst: roster.Organ, Display: "Organ", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Sub", Inst: roster.Bass808, Display: "Sub bass", MaxVoices: 1, Volume: 0.82},
				{Index: 2, Name: "Vocal chop", Inst: roster.VoiceAhh, Display: "Vocal chop", MaxVoices: 1, Volume: 0.6},
			},
			Groove: groove("groove-uk-garage", 2, 0.8, map[int]KitPiece{
				36: {Inst: roster.Kick, Display: "Kick"},
				37: {Inst: roster.Sidestick, Display: "Rim"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				47: {Inst: roster.Tom, Display: "Tom", Pitch: -1},
			}),
		},
		{
			// The one famous MELODY: a public-domain folk song (the Tetris
			// theme), transcribed from a cited public-domain sheet, doubled two
			// octaves down on an acid bass.
			Stem: "tetris-techno", ScoreID: "trad-korobeiniki",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Melody", Inst: roster.LeadChip, Display: "Lead", MaxVoices: 1, Volume: 0.68},
				{Index: 0, Name: "Melody", Inst: roster.BassAcid, Display: "Acid bass", MaxVoices: 1, Volume: 0.7, Octave: -2},
			},
			Groove: groove("groove-techno", 1, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickElectro, Display: "Kick"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.6},
			}),
		},

		// ── Breaks · IDM ──────────────────────────────────────────────────
		{
			Stem: "amen-break-jungle", ScoreID: "beatmo-night-bus",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Reese", Inst: roster.BassReese, Display: "Reese bass", MaxVoices: 1, Volume: 0.78},
				{Index: 1, Name: "Pad", Inst: roster.ModularPad, Display: "Pad", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Piano", Inst: roster.PianoFelt, Display: "Piano", MaxVoices: 1, Volume: 0.66},
			},
			Groove: groove("groove-amen-break", 4, 0.82, map[int]KitPiece{
				36: {Inst: roster.DnbKick, Display: "Kick"},
				38: {Inst: roster.SnareCrack, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Ride hat", Volume: 0.55},
				46: {Inst: roster.HihatOpenBright, Display: "Open hat", Volume: 0.5},
			}),
		},
		{
			Stem: "flim", ScoreID: "beatmo-film-grain",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Music box", Inst: roster.MusicBox, Display: "Music box", MaxVoices: 1, Volume: 0.7},
				{Index: 1, Name: "Piano", Inst: roster.PianoFelt, Display: "Piano", MaxVoices: 2, Volume: 0.55},
				{Index: 2, Name: "Sub", Inst: roster.SubBass, Display: "Sub", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-flim", 8, 0.8, kitIDM),
		},
		{
			Stem: "xtal", ScoreID: "beatmo-quartz",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceSoprano, Display: "Voice", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Pad", Inst: roster.GlassArmonica, Display: "Pad", MaxVoices: 2, Volume: 0.48},
				{Index: 2, Name: "Sub", Inst: roster.SubBass, Display: "Sub", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-xtal", 2, 0.8, kitIDM),
		},
		{
			Stem: "idioteque", ScoreID: "beatmo-bunker",
			From: 1, To: 6, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Bells", Inst: roster.FmBell, Display: "Bells", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Pad", Inst: roster.PwmPad, Display: "Pad", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Sub", Inst: roster.SubBass, Display: "Sub", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-idioteque", 3, 0.8, kitIDM),
		},
		{
			Stem: "when-i-rule-the-world", ScoreID: "beatmo-crown-glass",
			From: 1, To: 6, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Glock", Inst: roster.Glockenspiel, Display: "Glock", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Supersaw", Inst: roster.LeadSupersaw, Display: "Supersaw", MaxVoices: 2, Volume: 0.5},
				{Index: 2, Name: "Bass", Inst: roster.BassWobble, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-when-i-rule-the-world", 3, 0.8, kitHyper),
		},

		// ── Latin · Global ────────────────────────────────────────────────
		{
			Stem: "reggaeton-dembow", ScoreID: "beatmo-luna-llena",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Guitar", Inst: roster.GuitarNylon, Display: "Guitar", MaxVoices: 1, Volume: 0.62},
				{Index: 1, Name: "Lead", Inst: roster.Marimba, Display: "Marimba", MaxVoices: 1, Volume: 0.7},
				{Index: 2, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.84},
			},
			Groove: groove("groove-reggaeton", 1, 0.82, map[int]KitPiece{
				36: {Inst: roster.KickPunchy, Display: "Kick"},
				40: {Inst: roster.SnareCrack, Display: "Snare"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
			}),
		},
		{
			Stem: "baile-funk", ScoreID: "beatmo-morro-azul",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Stabs", Inst: roster.BrassStab, Display: "Stabs", MaxVoices: 2, Volume: 0.6},
				{Index: 1, Name: "Vocal chop", Inst: roster.VoiceAhh, Display: "Vocal chop", MaxVoices: 1, Volume: 0.64},
				{Index: 2, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.84},
			},
			Groove: groove("groove-baile-funk", 1, 0.84, map[int]KitPiece{
				36: {Inst: roster.Kick808, Display: "808 kick"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
			}),
		},
		{
			Stem: "dancehall-riddim", ScoreID: "beatmo-coral-bay",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Pluck", Inst: roster.GuitarSteel, Display: "Pluck", MaxVoices: 1, Volume: 0.6},
				{Index: 1, Name: "Lead", Inst: roster.SteelDrum, Display: "Steel drum", MaxVoices: 1, Volume: 0.7},
				{Index: 2, Name: "808", Inst: roster.Bass808, Display: "808 bass", MaxVoices: 1, Volume: 0.84},
			},
			Groove: groove("groove-dancehall", 2, 0.8, map[int]KitPiece{
				36: {Inst: roster.KickPunchy, Display: "Kick"},
				37: {Inst: roster.Sidestick, Display: "Rim"},
				38: {Inst: roster.SnareCrack, Display: "Snare"},
				39: {Inst: roster.Clap, Display: "Clap"},
				42: {Inst: roster.Hihat, Display: "Hi-hat", Volume: 0.55},
			}),
		},
		{
			Stem: "salsa-clave", ScoreID: "beatmo-malecon",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 1, Volume: 0.64},
				{Index: 1, Name: "Trumpet", Inst: roster.Trumpet, Display: "Trumpet", MaxVoices: 1, Volume: 0.72},
				{Index: 2, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-salsa", 2, 0.78, map[int]KitPiece{
				75: {Inst: roster.Sidestick, Display: "Clave"},
				76: {Inst: roster.Rimshot, Display: "Cáscara", Volume: 0.6},
				62: {Inst: roster.Conga, Display: "Conga"},
				63: {Inst: roster.CongaOpen, Display: "Conga open"},
				56: {Inst: roster.Cowbell, Display: "Campana", Volume: 0.6},
			}),
		},
		{
			Stem: "cumbia", ScoreID: "beatmo-arena-y-sal",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Accordion", Inst: roster.Accordion, Display: "Accordion", MaxVoices: 1, Volume: 0.72},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-cumbia", 2, 0.8, map[int]KitPiece{
				73: {Inst: roster.Shaker, Display: "Guacharaca"},
				64: {Inst: roster.CongaTumba, Display: "Llamador"},
				63: {Inst: roster.CongaOpen, Display: "Alegre"},
				36: {Inst: roster.KickAcoustic, Display: "Bombo"},
			}),
		},
		{
			Stem: "merengue", ScoreID: "beatmo-brisa",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Sax", Inst: roster.Sax, Display: "Sax", MaxVoices: 1, Volume: 0.72},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-merengue", 2, 0.8, map[int]KitPiece{
				73: {Inst: roster.Shaker, Display: "Güira"},
				64: {Inst: roster.CongaTumba, Display: "Tambora"},
				76: {Inst: roster.Rimshot, Display: "Tambora rim", Volume: 0.55},
				36: {Inst: roster.KickAcoustic, Display: "Bombo"},
			}),
		},
		{
			Stem: "vallenato", ScoreID: "beatmo-sabana",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Accordion", Inst: roster.Accordion, Display: "Accordion", MaxVoices: 1, Volume: 0.74},
				{Index: 1, Name: "Bass", Inst: roster.BassGuitar, Display: "Bass", MaxVoices: 1, Volume: 0.8},
			},
			Groove: groove("groove-vallenato", 1, 0.8, map[int]KitPiece{
				73: {Inst: roster.Shaker, Display: "Guacharaca"},
				65: {Inst: roster.TomHighAcoustic, Display: "Caja", Pitch: 3},
				66: {Inst: roster.TomHighAcoustic, Display: "Caja"},
			}),
		},

		// ── Baroque ───────────────────────────────────────────────────────
		{
			Stem: "bach-brandenburg-3", ScoreID: "bach-brandenburg-3",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Violin 1", Inst: roster.Violin, Display: "Violin I", MaxVoices: 2, Volume: 0.80},
				{Index: 3, Name: "Viola 1", Inst: roster.Viola, Display: "Viola", MaxVoices: 1, Volume: 0.72},
				{Index: 6, Name: "Violoncello 1", Inst: roster.Cello, Display: "Cello", MaxVoices: 1, Volume: 0.78},
				{Index: 10, Name: "Harpsichord", Inst: roster.Harpsichord, Display: "Harpsichord", MaxVoices: 2, Volume: 0.60},
			},
		},
		{
			Stem: "bach-brandenburg-4", ScoreID: "bach-brandenburg-4",
			From: 1, To: 24, Subdiv: 16,
			Parts: []PartMap{
				{Index: 1, Name: "Flute 1", Inst: roster.Flute, Display: "Recorder I", MaxVoices: 1, Volume: 0.80},
				{Index: 2, Name: "Flute 2", Inst: roster.FluteBreathy, Display: "Recorder II", MaxVoices: 1, Volume: 0.74},
				{Index: 0, Name: "Principal Violin", Inst: roster.Violin, Display: "Solo Violin", MaxVoices: 1, Volume: 0.78},
				{Index: 8, Name: "Continuo", Inst: roster.Cello, Display: "Continuo", MaxVoices: 1, Volume: 0.72},
			},
		},
		{
			Stem: "bach-kyrie-bwv232", ScoreID: "bach-kyrie-bwv232",
			From: 1, To: 8, Subdiv: 16,
			Parts: []PartMap{
				{Index: 5, Name: "Violin 1", Inst: roster.Violin, Display: "Violin I", MaxVoices: 2, Volume: 0.78},
				{Index: 2, Name: "Oboe d’amore 1", Inst: roster.Oboe, Display: "Oboe d'amore", MaxVoices: 1, Volume: 0.74},
				{Index: 0, Name: "Traverso 1", Inst: roster.Flute, Display: "Traverso", MaxVoices: 1, Volume: 0.70},
				{Index: 13, Name: "Continuo", Inst: roster.Cello, Display: "Continuo", MaxVoices: 1, Volume: 0.76},
			},
		},

		// ── Classical ─────────────────────────────────────────────────────
		{
			Stem: "beethoven-fifth", ScoreID: "beethoven-fifth",
			From: 1, To: 22, Subdiv: 16,
			Parts: append(strings4(13, 14, 15, 16, "Violin 1", "Violin 2", "Viola", "Violoncello"),
				timpani(12, "C, G Timpani")),
		},
		{
			Stem: "beethoven-ode-to-joy", ScoreID: "beethoven-ode-to-joy",
			// The theme's first statement: unharmonised cellos and basses.
			From: 92, To: 107, Subdiv: 16,
			Parts: []PartMap{
				{Index: 34, Name: "Violoncello", Inst: roster.Cello, Display: "Cello", MaxVoices: 1, Volume: 0.82},
				{Index: 35, Name: "Contrabass", Inst: roster.DoubleBass, Display: "Contrabass", MaxVoices: 1, Volume: 0.76},
				{Index: 8, Name: "Bassoon 2", Inst: roster.Bassoon, Display: "Bassoon", MaxVoices: 1, Volume: 0.66},
				timpani(19, "D, A Timpani"),
			},
		},
		{
			Stem: "haydn-london", ScoreID: "haydn-london",
			// The Allegro proper; measures 1..16 are the slow introduction and
			// bar 17 is silent in the strings, which enter in bar 18.
			From: 18, To: 33, Subdiv: 16,
			Parts: append(strings4(13, 14, 15, 16, "Violin 1", "Violin 2", "Viola", "Violoncello"),
				timpani(12, "Timpani")),
		},
		{
			Stem: "schubert-fifth", ScoreID: "schubert-fifth",
			// Bars 1-2 are the woodwinds' upbeat figure; the strings enter in 3.
			From: 3, To: 18, Subdiv: 16,
			Parts: strings4(7, 8, 9, 10, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "haydn-quartet-op76", ScoreID: "haydn-quartet-op76",
			From: 1, To: 8, Subdiv: 16,
			Parts: strings4(0, 1, 2, 3, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "mozart-quartet-k387", ScoreID: "mozart-quartet-k387",
			From: 1, To: 10, Subdiv: 16,
			Parts: strings4(0, 1, 2, 3, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "beethoven-quartet-op18", ScoreID: "beethoven-quartet-op18",
			From: 1, To: 8, Subdiv: 16,
			Parts: strings4(0, 1, 2, 3, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},

		// ── Romantic ──────────────────────────────────────────────────────
		{
			Stem: "brahms-first-finale", ScoreID: "brahms-first-finale",
			// The Allegro's main theme, not the slow introduction.
			From: 62, To: 77, Subdiv: 16,
			Parts: strings4(19, 20, 21, 22, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "dvorak-american", ScoreID: "dvorak-american",
			From: 1, To: 8, Subdiv: 16,
			Parts: strings4(0, 1, 2, 3, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "borodin-quartet-2", ScoreID: "borodin-quartet-2",
			From: 1, To: 8, Subdiv: 16,
			Parts: strings4(0, 1, 2, 3, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "beach-gaelic", ScoreID: "beach-gaelic",
			From: 1, To: 16, Subdiv: 16,
			Parts: strings4(20, 21, 22, 23, "Violin 1", "Violin 2", "Viola", "Violoncello"),
		},
		{
			Stem: "boulanger-printemps", ScoreID: "boulanger-printemps",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 1, Name: "Flute 1", Inst: roster.Flute, Display: "Flute", MaxVoices: 1, Volume: 0.78},
				{Index: 28, Name: "Violin 1", Inst: roster.Violin, Display: "Violin I", MaxVoices: 1, Volume: 0.74},
				{Index: 32, Name: "Violin 2", Inst: roster.ViolinEnsemble, Display: "Violin II", MaxVoices: 1, Volume: 0.68},
				{Index: 35, Name: "Viola", Inst: roster.Viola, Display: "Viola", MaxVoices: 1, Volume: 0.70},
				{Index: 25, Name: "Celesta", Inst: roster.Celesta, Display: "Celesta", MaxVoices: 2, Volume: 0.62},
				{Index: 22, Name: "Cymbal", Display: "Cymbal", Volume: 0.45},
				{Index: 24, Name: "Triangle", Display: "Triangle", Volume: 0.5},
			},
		},

		// ── Lied & Song ───────────────────────────────────────────────────
		{
			Stem: "schubert-erlkonig", ScoreID: "schubert-erlkonig",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 1, Name: "Piano", Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 3, Volume: 0.72},
				{Index: 0, Name: "Clarinet in C", Inst: roster.VoiceOpera, Display: "Voice", MaxVoices: 1, Volume: 0.82},
			},
		},
		{
			// Subdiv 32, not 16: the piano writes continuous sextuplets — six
			// notes per beat, which four 16th-note steps cannot hold. At 16 a
			// third of the notes collided onto an occupied step.
			Stem: "schubert-ave-maria", ScoreID: "schubert-ave-maria",
			From: 1, To: 8, Subdiv: 32,
			Parts: voicePiano("Singstimme Voice", "Pianoforte", roster.VoiceSoprano),
		},
		{
			Stem: "schubert-standchen", ScoreID: "schubert-standchen",
			From: 1, To: 20, Subdiv: 16,
			Parts: voicePiano("Voice", "Piano", roster.VoiceSoprano),
		},
		{
			Stem: "clara-schumann-op13-1", ScoreID: "clara-schumann-op13-1",
			From: 1, To: 16, Subdiv: 16,
			Parts: voicePiano("Voice", "Piano", roster.VoiceSoprano),
		},
		{
			Stem: "hensel-italien", ScoreID: "hensel-italien",
			From: 1, To: 16, Subdiv: 16,
			Parts: voicePiano("Singstimme Voice", "Pianoforte", roster.VoiceSoprano),
		},

		// ── French & Americas ─────────────────────────────────────────────
		{
			Stem: "faure-apres-un-reve", ScoreID: "faure-apres-un-reve",
			From: 1, To: 16, Subdiv: 16,
			Parts: voicePiano("Voice", "Piano", roster.VoiceOpera),
		},
		{
			Stem: "satie-air-du-rat", ScoreID: "satie-air-du-rat",
			From: 1, To: 22, Subdiv: 16,
			Parts: voicePiano("Chant", "Piano", roster.VoiceWhisper),
		},
		{
			Stem: "liliuokalani-aloha-oe", ScoreID: "liliuokalani-aloha-oe",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.VoiceSoprano, Display: "Voice", MaxVoices: 1, Volume: 0.82},
				{Index: 1, Name: "Women", Inst: roster.VoiceAhh, Display: "Women", MaxVoices: 1, Volume: 0.62},
				{Index: 2, Name: "Men", Inst: roster.VoiceAhh, Display: "Men", MaxVoices: 1, Volume: 0.60},
				{Index: 3, Name: "Piano", Inst: roster.GuitarNylon, Display: "Guitar", MaxVoices: 3, Volume: 0.66},
			},
		},
		{
			Stem: "gonzaga-espanha-e-brasil", ScoreID: "gonzaga-espanha-e-brasil",
			From: 1, To: 16, Subdiv: 16,
			Parts: []PartMap{
				{Index: 0, Name: "Voice", Inst: roster.Flute, Display: "Melody", MaxVoices: 1, Volume: 0.80},
				{Index: 1, Name: "Piano", Inst: roster.GuitarNylon, Display: "Guitar", MaxVoices: 3, Volume: 0.70},
			},
		},
	}
}
