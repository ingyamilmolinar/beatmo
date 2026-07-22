package audio

// modular_instruments.go is the CONFIG-FIRST source of truth for modular-engine
// instruments. An instrument here is defined PURELY by data — its param config
// (Seed) plus a little metadata — never by per-instrument code. One row in
// modularInstrumentDefs replaces what used to be ~6 scattered edits (the engine
// voice in ResetInstruments, the recipe descriptor + instrument→recipe binding
// in synth_recipe.go, the playable InstrumentConfig in config.go, the audio.js
// RENDER/RENDER_INFO entries, and the generated id list). The four Go-side
// registrations are derived from this table (see the modularTable* helpers,
// folded into the consuming structures at package-var init); audio.js entries
// are generated from it (modular_instruments.gen.js); the id list is generated
// from it too.
//
// To ADD a modular instrument: append one row. To CLONE one: copy a row, give
// it a new ID/RecipeID, and overlay Seed changes (or use CloneInstrument at
// runtime). To DELETE one: remove the row (or DeleteInstrument at runtime).

// ModularInstrumentDef is the complete, code-free definition of a modular
// instrument. Everything that makes it a distinct instrument is in Seed; the
// rest is registration metadata.
type ModularInstrumentDef struct {
	ID          string       // playable instrument id (e.g. "zgump-kick")
	RecipeID    string       // recipe id (e.g. "synth-modular-kick-zgump")
	Display     string       // UI display name
	Category    string       // recipe category (modularRecipeCategory)
	Seed        RecipeParams // THE CONFIG — the only thing that defines the sound
	Beats       float64      // render buffer window, in beats
	DurationSec float64      // one-shot / sampler capture duration
	Amplitude   float32      // playback amplitude
}

// modularInstrumentDefs is the table. It holds ALL seeded-modular instruments
// shipped by the engine: the configurable-KICK-stage family (every kick is a
// Seed over the source==5 kick voice), the percussion family (congas), and
// the melodic families (bowed/plucked strings, keys, woodwind, brass, bass,
// plus the organ/sax masterpiece set and the modular-pad preset) — all follow
// the identical registration shape. The base "modular" instrument has NO
// Seed (it plays the unparameterized schema identity) and stays hand-written
// in engine_instruments.go / synth_recipe.go / config.go.
var modularInstrumentDefs = []ModularInstrumentDef{
	{ID: "dnb-kick", RecipeID: "synth-modular-kick-dnb", Display: "DnB Kick", Category: modularRecipeCategory, Seed: dnbKickSeed, Beats: 1.0, DurationSec: 0.5, Amplitude: 0.8},
	{ID: "kick-electro", RecipeID: "synth-modular-kick-electro", Display: "Electro Kick", Category: modularRecipeCategory, Seed: electroKickSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	{ID: "kick-808", RecipeID: "synth-modular-kick-808", Display: "808 Kick", Category: modularRecipeCategory, Seed: kick808Seed, Beats: 1.5, DurationSec: 0.75, Amplitude: 0.8},
	{ID: "kick-acoustic", RecipeID: "synth-modular-kick-acoustic", Display: "Acoustic Kick", Category: modularRecipeCategory, Seed: acousticKickSeed, Beats: 1.3, DurationSec: 0.65, Amplitude: 0.8},
	// "Dry Kick" — the display name (id kept as kick-punchy: the id is the stable
	// identifier that templates, import goldens and the legacy migration key off;
	// the codebase renames via display metadata, not the id).
	{ID: "kick-punchy", RecipeID: "synth-modular-kick-punchy", Display: "Dry Kick", Category: modularRecipeCategory, Seed: punchyKickSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	{ID: "zgump-kick", RecipeID: "synth-modular-kick-zgump", Display: "Organic Kick", Category: modularRecipeCategory, Seed: zgumpKickSeed, Beats: 0.7, DurationSec: 0.4, Amplitude: 0.8},
	{ID: "raw-kick", RecipeID: "synth-modular-kick-raw", Display: "Raw Kick", Category: modularRecipeCategory, Seed: rawKickSeed, Beats: 0.7, DurationSec: 0.4, Amplitude: 0.8},
	// The variant-7 modal voice that reads as an organic high tom (was the
	// acoustic kick before it was re-tuned lower/brutaler for a real kick).
	{ID: "high-tom-organic", RecipeID: "synth-modular-hightom-organic", Display: "High Tom Organic", Category: modularRecipeCategory, Seed: highTomOrganicSeed, Beats: 0.9, DurationSec: 0.45, Amplitude: 0.8},

	// ── Batch 2026-07-05: the remaining seeded-modular instruments absorbed
	// from the four scattered registration sites (engine_instruments.go voice,
	// synth_recipe.go descriptor+binding, config.go config). Values are the
	// EXACT previous registrations — byte-identity is gated by
	// TestSynthGoldenByteIdentity. ──
	{ID: "modular-pad", RecipeID: "synth-modular-pad", Display: "Modular Pad", Category: modularRecipeCategory, Seed: modularPadSeed, Beats: 1.0, DurationSec: 1.0, Amplitude: 0.8},
	{ID: "conga", RecipeID: "synth-modular-conga", Display: "Conga", Category: modularRecipeCategory, Seed: congaSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	{ID: "conga-open", RecipeID: "synth-modular-conga-open", Display: "Conga Open", Category: modularRecipeCategory, Seed: congaOpenSeed, Beats: 0.5, DurationSec: 0.5, Amplitude: 0.8},
	// conga-tumba: DurationSec 0.6 ≠ Beats-derived 0.5 — preserved exactly from config.go.
	{ID: "conga-tumba", RecipeID: "synth-modular-conga-tumba", Display: "Conga Tumba", Category: modularRecipeCategory, Seed: congaTumbaSeed, Beats: 0.5, DurationSec: 0.6, Amplitude: 0.8},
	// organ/sax: non-uniform amplitudes preserved exactly (organ 0.65 for
	// sustained-voice headroom under sends; sax 0.78) — from config.go.
	{ID: "organ", RecipeID: "synth-modular-organ", Display: "Organ", Category: modularRecipeCategory, Seed: organSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.65},
	{ID: "sax", RecipeID: "synth-modular-sax", Display: "Saxophone", Category: modularRecipeCategory, Seed: saxSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.78},

	// ── Batch 2026-07-05 (part 2): the 28 melodic instruments — bowed strings,
	// plucked strings, keys, woodwind, brass, and bass families — absorbed
	// from the same four scattered registration sites. Values are the EXACT
	// previous registrations — byte-identity is gated by
	// TestSynthGoldenByteIdentity. ──
	// Bowed strings family — LFO→pitch vibrato showcase.
	{ID: "violin", RecipeID: "synth-modular-violin", Display: "Violin", Category: modularRecipeCategory, Seed: violinSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "violin-ensemble", RecipeID: "synth-modular-violin-ensemble", Display: "Violin Ensemble", Category: modularRecipeCategory, Seed: violinEnsembleSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "cello", RecipeID: "synth-modular-cello", Display: "Cello", Category: modularRecipeCategory, Seed: celloSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "cello-warm", RecipeID: "synth-modular-cello-warm", Display: "Cello Warm", Category: modularRecipeCategory, Seed: celloWarmSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "organ-church", RecipeID: "synth-modular-organ-church", Display: "Church Organ", Category: modularRecipeCategory, Seed: organChurchSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "scifi-lead", RecipeID: "synth-modular-scifi-lead", Display: "Sci-Fi Lead", Category: modularRecipeCategory, Seed: sciFiLeadSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// Plucked strings — Karplus-Strong.
	{ID: "guitar-nylon", RecipeID: "synth-modular-guitar-nylon", Display: "Guitar Nylon", Category: modularRecipeCategory, Seed: guitarNylonSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-nylon-bright", RecipeID: "synth-modular-guitar-nylon-bright", Display: "Guitar Nylon Bright", Category: modularRecipeCategory, Seed: guitarNylonBrightSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-steel", RecipeID: "synth-modular-guitar-steel", Display: "Guitar Steel", Category: modularRecipeCategory, Seed: guitarSteelSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-steel-warm", RecipeID: "synth-modular-guitar-steel-warm", Display: "Guitar Steel Warm", Category: modularRecipeCategory, Seed: guitarSteelWarmSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-electric", RecipeID: "synth-modular-guitar-electric", Display: "Guitar Electric", Category: modularRecipeCategory, Seed: guitarElectricSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "guitar-electric-neck", RecipeID: "synth-modular-guitar-electric-neck", Display: "Guitar Electric Neck", Category: modularRecipeCategory, Seed: guitarElectricNeckSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "harp", RecipeID: "synth-modular-harp", Display: "Harp", Category: modularRecipeCategory, Seed: harpSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// Keys — additive piano.
	{ID: "piano-grand", RecipeID: "synth-modular-piano-grand", Display: "Piano Grand", Category: modularRecipeCategory, Seed: pianoGrandSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "piano-felt", RecipeID: "synth-modular-piano-felt", Display: "Piano Felt", Category: modularRecipeCategory, Seed: pianoFeltSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// Woodwind family.
	{ID: "flute", RecipeID: "synth-modular-flute", Display: "Flute", Category: modularRecipeCategory, Seed: fluteSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "flute-breathy", RecipeID: "synth-modular-flute-breathy", Display: "Flute Breathy", Category: modularRecipeCategory, Seed: fluteBreathySeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "oboe", RecipeID: "synth-modular-oboe", Display: "Oboe", Category: modularRecipeCategory, Seed: oboeSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "oboe-full", RecipeID: "synth-modular-oboe-full", Display: "Oboe Full", Category: modularRecipeCategory, Seed: oboeFullSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// Brass family.
	{ID: "trumpet", RecipeID: "synth-modular-trumpet", Display: "Trumpet", Category: modularRecipeCategory, Seed: trumpetSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "trumpet-mellow", RecipeID: "synth-modular-trumpet-mellow", Display: "Trumpet Mellow", Category: modularRecipeCategory, Seed: trumpetMellowSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "french-horn", RecipeID: "synth-modular-french-horn", Display: "French Horn", Category: modularRecipeCategory, Seed: frenchHornSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "french-horn-loud", RecipeID: "synth-modular-french-horn-loud", Display: "French Horn Loud", Category: modularRecipeCategory, Seed: frenchHornLoudSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// Bass guitar (renamed from synth-bass) + synth bass family.
	{ID: "bass-guitar", RecipeID: "synth-modular-bass-guitar", Display: "Bass Guitar", Category: modularRecipeCategory, Seed: bassGuitarSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-acid", RecipeID: "synth-modular-bass-acid", Display: "Acid Bass", Category: modularRecipeCategory, Seed: bassAcidSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-reese", RecipeID: "synth-modular-bass-reese", Display: "Reese Bass", Category: modularRecipeCategory, Seed: bassReeseSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-fm", RecipeID: "synth-modular-bass-fm", Display: "FM Bass DX", Category: modularRecipeCategory, Seed: bassFMSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-808", RecipeID: "synth-modular-bass-808", Display: "808 Bass", Category: modularRecipeCategory, Seed: bass808Seed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	// ── Phase-15 voice/choir family, split by ear review 2026-07-08. Six
	// instruments were built as voice/choir attempts; the user ear-tested all
	// six and ruled that four of them sound like synths, not voices — they
	// were re-categorized under their true categories with new ids (sound
	// byte-identical, only id/RecipeID/Display/category changed):
	//   choir-ahh   -> ensemble-lead      (CatLead)
	//   choir-ooh   -> ensemble-lead-dark (CatLead)
	//   voice-bass  -> ghost-bass         (CatBass)
	//   voice-pad   -> viola-pad          (CatStrings)
	// voice-soprano and voice-whisper stayed in Voice (they get re-tuned
	// separately). Same Beats/Duration shape as organ/sax. Amplitude 0.8 is
	// the fallback; the real level comes from the measure-loudness regen. ──
	{ID: "ensemble-lead", RecipeID: "synth-modular-ensemble-lead", Display: "Ensemble Lead", Category: modularRecipeCategory, Seed: ensembleLeadSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "ensemble-lead-dark", RecipeID: "synth-modular-ensemble-lead-dark", Display: "Ensemble Lead Dark", Category: modularRecipeCategory, Seed: ensembleLeadDarkSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-soprano", RecipeID: "synth-modular-voice-soprano", Display: "Soprano", Category: modularRecipeCategory, Seed: voiceSopranoSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "ghost-bass", RecipeID: "synth-modular-ghost-bass", Display: "Ghost Bass", Category: modularRecipeCategory, Seed: ghostBassSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "viola-pad", RecipeID: "synth-modular-viola-pad", Display: "Viola Pad", Category: modularRecipeCategory, Seed: violaPadSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "voice-whisper", RecipeID: "synth-modular-voice-whisper", Display: "Whisper", Category: modularRecipeCategory, Seed: voiceWhisperSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// voice-ahh: sung male "ahh" tuned against a real reference recording
	// (see voiceAhhSeed). Longer Beats/Duration than the rest of the voice
	// family — the ref is a long sustained sung note with a slow swell.
	{ID: "voice-ahh", RecipeID: "synth-modular-voice-ahh", Display: "Ahh Voice", Category: modularRecipeCategory, Seed: voiceAhhSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},
	// voice-opera: breathier "old-man opera" variant of voice-ahh, ear-
	// approved per the 2026-07-08 round-2 review (see voiceOperaSeed).
	{ID: "voice-opera", RecipeID: "synth-modular-voice-opera", Display: "Opera Voice", Category: modularRecipeCategory, Seed: voiceOperaSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},

	// ── Showcase batch 2026-07-20: 24 config-only instruments (seeds in
	// instrument_seeds_showcase.go). Designed from synthesis principles, not
	// reference-WAV tuned — first drafts for the ear-gate. All pitched (mallets
	// included) → all in melodicRecipeIDs. ──
	// A · Mallets & metals (struck, inharmonic).
	{ID: "marimba", RecipeID: "synth-modular-marimba", Display: "Marimba", Category: modularRecipeCategory, Seed: marimbaSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "vibraphone", RecipeID: "synth-modular-vibraphone", Display: "Vibraphone", Category: modularRecipeCategory, Seed: vibraphoneSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},
	{ID: "glockenspiel", RecipeID: "synth-modular-glockenspiel", Display: "Glockenspiel", Category: modularRecipeCategory, Seed: glockenspielSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "gamelan", RecipeID: "synth-modular-gamelan", Display: "Gamelan", Category: modularRecipeCategory, Seed: gamelanSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// B · Plucked & struck strings (Karplus-Strong).
	{ID: "kalimba", RecipeID: "synth-modular-kalimba", Display: "Kalimba", Category: modularRecipeCategory, Seed: kalimbaSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "sitar", RecipeID: "synth-modular-sitar", Display: "Sitar", Category: modularRecipeCategory, Seed: sitarSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "banjo", RecipeID: "synth-modular-banjo", Display: "Banjo", Category: modularRecipeCategory, Seed: banjoSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "music-box", RecipeID: "synth-modular-music-box", Display: "Music Box", Category: modularRecipeCategory, Seed: musicBoxSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// C · Keys (FM / KS / additive).
	{ID: "piano-rhodes", RecipeID: "synth-modular-piano-rhodes", Display: "Electric Piano", Category: modularRecipeCategory, Seed: pianoRhodesSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "clavinet", RecipeID: "synth-modular-clavinet", Display: "Clavinet", Category: modularRecipeCategory, Seed: clavinetSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "harpsichord", RecipeID: "synth-modular-harpsichord", Display: "Harpsichord", Category: modularRecipeCategory, Seed: harpsichordSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "celesta", RecipeID: "synth-modular-celesta", Display: "Celesta", Category: modularRecipeCategory, Seed: celestaSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// D · Winds & reeds.
	{ID: "clarinet", RecipeID: "synth-modular-clarinet", Display: "Clarinet", Category: modularRecipeCategory, Seed: clarinetSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "tuba", RecipeID: "synth-modular-tuba", Display: "Tuba", Category: modularRecipeCategory, Seed: tubaSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "pan-flute", RecipeID: "synth-modular-pan-flute", Display: "Pan Flute", Category: modularRecipeCategory, Seed: panFluteSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "harmonium", RecipeID: "synth-modular-harmonium", Display: "Harmonium", Category: modularRecipeCategory, Seed: harmoniumSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// E · Synth leads & basses.
	{ID: "lead-supersaw", RecipeID: "synth-modular-lead-supersaw", Display: "Supersaw Lead", Category: modularRecipeCategory, Seed: leadSupersawSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "lead-hoover", RecipeID: "synth-modular-lead-hoover", Display: "Hoover Lead", Category: modularRecipeCategory, Seed: leadHooverSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "lead-chip", RecipeID: "synth-modular-lead-chip", Display: "Chip Lead", Category: modularRecipeCategory, Seed: leadChipSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "bass-wobble", RecipeID: "synth-modular-bass-wobble", Display: "Wobble Bass", Category: modularRecipeCategory, Seed: bassWobbleSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	// F · Experimental / textural.
	{ID: "theremin", RecipeID: "synth-modular-theremin", Display: "Theremin", Category: modularRecipeCategory, Seed: thereminSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "singing-bowl", RecipeID: "synth-modular-singing-bowl", Display: "Singing Bowl", Category: modularRecipeCategory, Seed: singingBowlSeed, Beats: 3.0, DurationSec: 3.0, Amplitude: 0.8},
	{ID: "glass-wash", RecipeID: "synth-modular-glass-wash", Display: "Glass Wash", Category: modularRecipeCategory, Seed: glassWashSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},
	{ID: "voice-alien", RecipeID: "synth-modular-voice-alien", Display: "Alien Choir", Category: modularRecipeCategory, Seed: voiceAlienSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},

	// ── Showcase batch ROUND 2 2026-07-20: 18 more config-only instruments
	// (seeds in instrument_seeds_showcase2.go). All default to A3 (no octave
	// offsets) so every melodic instrument shares the same default pitch. ──
	// A2 · Strings & bowed.
	{ID: "viola", RecipeID: "synth-modular-viola", Display: "Viola", Category: modularRecipeCategory, Seed: violaSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "double-bass", RecipeID: "synth-modular-double-bass", Display: "Double Bass", Category: modularRecipeCategory, Seed: doubleBassSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "pizz-strings", RecipeID: "synth-modular-pizz-strings", Display: "Pizzicato Strings", Category: modularRecipeCategory, Seed: pizzStringsSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	// B2 · World plucked.
	{ID: "mandolin", RecipeID: "synth-modular-mandolin", Display: "Mandolin", Category: modularRecipeCategory, Seed: mandolinSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "ukulele", RecipeID: "synth-modular-ukulele", Display: "Ukulele", Category: modularRecipeCategory, Seed: ukuleleSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "koto", RecipeID: "synth-modular-koto", Display: "Koto", Category: modularRecipeCategory, Seed: kotoSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	// C2 · Metals & bells.
	{ID: "steel-drum", RecipeID: "synth-modular-steel-drum", Display: "Steel Drum", Category: modularRecipeCategory, Seed: steelDrumSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "tubular-bells", RecipeID: "synth-modular-tubular-bells", Display: "Tubular Bells", Category: modularRecipeCategory, Seed: tubularBellsSeed, Beats: 3.0, DurationSec: 3.0, Amplitude: 0.8},
	{ID: "xylophone", RecipeID: "synth-modular-xylophone", Display: "Xylophone", Category: modularRecipeCategory, Seed: xylophoneSeed, Beats: 1.0, DurationSec: 1.0, Amplitude: 0.8},
	// D2 · Winds & brass.
	{ID: "bassoon", RecipeID: "synth-modular-bassoon", Display: "Bassoon", Category: modularRecipeCategory, Seed: bassoonSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "piccolo", RecipeID: "synth-modular-piccolo", Display: "Piccolo", Category: modularRecipeCategory, Seed: piccoloSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "accordion", RecipeID: "synth-modular-accordion", Display: "Accordion", Category: modularRecipeCategory, Seed: accordionSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "trombone", RecipeID: "synth-modular-trombone", Display: "Trombone", Category: modularRecipeCategory, Seed: tromboneSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "muted-trumpet", RecipeID: "synth-modular-muted-trumpet", Display: "Muted Trumpet", Category: modularRecipeCategory, Seed: mutedTrumpetSeed, Beats: 2.0, DurationSec: 2.0, Amplitude: 0.8},
	{ID: "brass-stab", RecipeID: "synth-modular-brass-stab", Display: "Brass Stab", Category: modularRecipeCategory, Seed: brassStabSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	// E2 · Synth & textural.
	{ID: "moog-bass", RecipeID: "synth-modular-moog-bass", Display: "Moog Bass", Category: modularRecipeCategory, Seed: moogBassSeed, Beats: 1.5, DurationSec: 1.5, Amplitude: 0.8},
	{ID: "pwm-pad", RecipeID: "synth-modular-pwm-pad", Display: "PWM Pad", Category: modularRecipeCategory, Seed: pwmPadSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},
	{ID: "glass-armonica", RecipeID: "synth-modular-glass-armonica", Display: "Glass Armonica", Category: modularRecipeCategory, Seed: glassArmonicaSeed, Beats: 2.5, DurationSec: 2.5, Amplitude: 0.8},
}

// ModularInstrumentDefs returns a copy of the table (for tests / tooling that
// generate the audio.js + id entries).
func ModularInstrumentDefs() []ModularInstrumentDef {
	out := make([]ModularInstrumentDef, len(modularInstrumentDefs))
	copy(out, modularInstrumentDefs)
	return out
}

// modularTableVoices (the engine-voice derivation) lives in the native-only
// modular_instruments_native.go because it calls bakedModularRender (a CGo
// path). The descriptors/bindings/configs derivations below are tag-neutral
// data and stay here.

// modularTableDescriptors derives the recipe descriptors from the table.
func modularTableDescriptors() []builtinRecipeDescriptor {
	out := make([]builtinRecipeDescriptor, 0, len(modularInstrumentDefs))
	for _, d := range modularInstrumentDefs {
		out = append(out, builtinRecipeDescriptor{
			ID: d.RecipeID, Display: d.Display, Category: d.Category, Seed: d.Seed,
		})
	}
	return out
}

// modularTableBindings derives the instrument→recipe bindings from the table.
func modularTableBindings() map[string]string {
	out := make(map[string]string, len(modularInstrumentDefs))
	for _, d := range modularInstrumentDefs {
		out[d.ID] = d.RecipeID
	}
	return out
}

// modularTableConfigs derives the playable InstrumentConfigs from the table.
func modularTableConfigs() map[string]InstrumentConfig {
	out := make(map[string]InstrumentConfig, len(modularInstrumentDefs))
	for _, d := range modularInstrumentDefs {
		out[d.ID] = InstrumentConfig{
			ID: d.ID, DurationSec: d.DurationSec, Amplitude: d.Amplitude,
		}
	}
	return out
}
