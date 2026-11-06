//go:build !js

package main

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// match_classes.go -- the BROAD kind of every roster instrument, for grading
// `refs match -validate` only (never for ranking: the matcher must find the
// kind from the sound).
//
// "Own family" (buildAuditionFamilies) is strict: a real cello whose closest
// voice is the synth viola counts as a miss. The listener asked for the
// instrument's essence -- is this a bowed string, a brass horn, a struck bar?
// -- so -validate also reports whether the first-ranked voice is the right
// KIND. TestEveryRosterInstrumentHasABroadClass keeps the table complete.

var matchBroadClasses = map[string][]instrument.ID{
	"bowed": {roster.Violin, roster.ViolinEnsemble, roster.Viola, roster.ViolaPad, roster.Cello, roster.CelloWarm, roster.DoubleBass},
	"brass": {roster.Trumpet, roster.TrumpetMellow, roster.MutedTrumpet, roster.FrenchHorn, roster.FrenchHornLoud, roster.Trombone, roster.Tuba, roster.BrassStab},
	"reed":  {roster.Sax, roster.Oboe, roster.OboeFull, roster.Clarinet, roster.Bassoon},
	"flute": {roster.Flute, roster.FluteBreathy, roster.Piccolo, roster.PanFlute},
	"voice": {roster.VoiceSoprano, roster.VoiceAhh, roster.VoiceOpera, roster.VoiceAlien, roster.VoiceWhisper},
	"organ": {roster.Organ, roster.OrganChurch, roster.Harmonium, roster.Accordion},
	"keys":  {roster.PianoGrand, roster.PianoFelt, roster.PianoRhodes, roster.FmEpiano, roster.FmEpianoDrive, roster.Clavinet},
	"plucked": {roster.Harp, roster.Harpsichord, roster.GuitarNylon, roster.GuitarNylonBright, roster.GuitarSteel, roster.GuitarSteelWarm,
		roster.GuitarElectric, roster.GuitarElectricNeck, roster.BassGuitar, roster.Banjo, roster.Mandolin, roster.Ukulele, roster.Sitar, roster.Koto,
		roster.PizzStrings, roster.FmPluck, roster.FmPluckTight},
	"idiophone": {roster.Marimba, roster.Xylophone, roster.Vibraphone, roster.Glockenspiel, roster.Celesta, roster.MusicBox, roster.Kalimba, roster.Gamelan,
		roster.SteelDrum, roster.TubularBells, roster.FmBell, roster.FmBellSoft, roster.SingingBowl, roster.GlassWash, roster.GlassArmonica,
		roster.Cowbell, roster.CowbellDrive, roster.CowbellLofi},
	"synth-lead": {roster.Modular, roster.ModularPad, roster.ScifiLead, roster.EnsembleLead, roster.EnsembleLeadDark, roster.LeadSupersaw,
		roster.LeadHoover, roster.LeadChip, roster.Theremin, roster.PwmPad, roster.FmLead, roster.FmLeadCrush},
	"synth-bass": {roster.FmBass, roster.FmBassDrive, roster.SubBass, roster.SubBassDrive, roster.BassAcid, roster.BassReese, roster.BassFm,
		roster.Bass808, roster.BassWobble, roster.GhostBass, roster.MoogBass},
	"kick": {roster.Kick, roster.KickDeep, roster.KickLofi, roster.KickSnap, roster.KickTight, roster.DnbKick, roster.KickElectro, roster.Kick808,
		roster.KickAcoustic, roster.KickAcousticHeavy, roster.KickPunchy, roster.ZgumpKick, roster.RawKick, roster.KickFat, roster.KickBoomBap,
		roster.KickBrutal, roster.KickRock, roster.KickWood, roster.KickAiry, roster.KickOrganicTight, roster.KickMuted, roster.KickMutedRoom},
	"snare": {roster.Snare, roster.SnareAcoustic, roster.SnareLofi, roster.SnareGhost, roster.SnareCrack, roster.SnareFat, roster.SnareDeep,
		roster.SnarePunch, roster.Rimshot, roster.Sidestick},
	"drum": {roster.Tom, roster.TomHighAcoustic, roster.TomHighDrive, roster.TomLofi, roster.HighTomOrganic, roster.Conga, roster.CongaOpen,
		roster.CongaTumba, roster.CongaOrganic, roster.CongaOrganicDry},
	"cymbal": {roster.Hihat, roster.HihatOpenBright, roster.HihatLofi, roster.HihatPedal, roster.Ride, roster.Crash},
	"noise":  {roster.Clap, roster.ClapBright, roster.ClapTight, roster.ClapLofi, roster.Shaker},
}

var matchBroadClassOf = func() map[instrument.ID]string {
	m := map[instrument.ID]string{}
	for class, ids := range matchBroadClasses {
		for _, id := range ids {
			m[id] = class
		}
	}
	return m
}()

// broadClass is inst's broad kind ("" = unclassified).
func broadClass(inst instrument.ID) string { return matchBroadClassOf[inst] }
