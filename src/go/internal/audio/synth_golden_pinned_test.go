//go:build !test && !js

package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// synthGoldens pins the deterministic one-shot render checksum {sampleCount,
// bitChecksum} for every built-in instrument. This is the byte-identity net for
// the config-declarative-pipeline refactor: any refactor step that changes a
// single output sample of any instrument fails here. Regenerate ONLY on an
// intentional sound change: go test -run TestSynthGoldenIdentityCapture -v
// ./internal/audio/ 2>&1 | grep GOLDEN
// Re-pinned 2026-07-05 (blanket bake pass): the 32 seeded modular melodic /
// percussion instruments (violin…organ) now bake their seed into the
// instrument-table Render via bakedModularRender, so RenderInstrumentOneShotRaw
// (the Sampler-preview/capture one-shot) covers the SEEDED voice instead of the
// bare ~220 Hz modular tone. These goldens therefore moved from the bare render
// to the seeded render (trumpet/trumpet-mellow/bass-guitar seeds also corrected
// to their honest ParamDef-clamped values — zero audible change). Guarded by
// TestModularNoEditFastPathMatchesRecipe (one-shot == recipe render, per id).
// Re-pinned 2026-07-06 (snare family only): snare/snare-crack/snare-lofi/snare-ghost
// adopt the FAT-BOTTOM drum-snare retune (snareVariantSpecs push lits synced
// to the new C/recipe defaults, so the no-edit fast path now renders the
// retuned sound); rimshot re-pinned after reverting the stray C tone-mix bump
// (1.12→1.0) back to its shipped sound.
// Re-pinned 2026-07-08 (Phase-15 voice family onboarding — bulk, NOT a sound
// change to existing instruments): `instrumentLoudnessAmp` (measure-loudness)
// auto-calibrates every instrument's playback amplitude off the MEDIAN
// rms/peak across the WHOLE registered population (L = medianNorm*0.8, then
// amp = L/rmsNorm per id). Registering the six new voice/choir ids shifted
// that population median, which shifted the computed amp for every other
// non-capped instrument by a sub-dB amount — inaudible, but the checksum here
// is byte-exact, so every affected instrument's golden moved too. Verified:
// diffing the regenerated instrument_loudness_gen.go against its pre-regen
// content shows the changed-amp id set is (modulo id-alias variants like
// cowbell-drive/tom-high-drive/fm-*-drive that inherit their base id's amp) exactly the set of
// checksums that moved here — no C/DSP change, no unrelated instrument's
// sound actually changed. Only sax and the two Phase-15 voice-family blocks
// carry their own more specific comments below.
// Re-pinned 2026-10-03 (config-only engine refactor Phase C, task C2): 9 of the
// FM instruments. The FM voice used to narrow its knobs with explicit (float)
// casts, and GCC -O2 elided some of them (native rounding depended on the
// optimizer); the knobs are now read as exact doubles. Change <= 3.6e-7 per
// sample (~1-3 float ULP, about -130 dB).
//
// Re-pinned 2026-10-02 (config-only engine refactor Phase A: modular_params
// float → double). 111 ids moved, EXACTLY the set TestRenderParityCompare
// reported changed. Proof the switch itself is exact: with every param rounded
// back to float32 (renderModularRoundParamsToFloat32) all 150 renders are
// bit-identical to the pre-switch dump. The remaining change is the seed values
// now arriving exactly instead of float32-rounded — ≤2.3e-5 waveform drift on
// long ringing tones, and kick-boom-bap's 30 ms hold landing one sample later.
// Then (same window, final-review fix) every READ of a param was widened too —
// mp_get/locals/helper params/fm_preset/adsr_t/synth_params were float, so ~111
// fields were still rounded to float32 at use (TestModularParamsAreNeverNarrowed
// pins 0 implicit narrowings). 98 more ids moved: envelopes ≤0.003 dB, pitch
// ±0.0000 cents, the difference sitting at one ADSR stage crossing. "modular"
// (the NULL fast path) moved once more when mp_get's fallback literals became
// doubles, which keeps NULL == identity-params byte-exact.
// Re-pinned 2026-10-02 (kick fullness pass, INTENTIONAL): 12 kicks retuned
// louder/punchier/fuller at equal peak after user A/B (dnb-kick, kick, kick-808,
// kick-acoustic, kick-deep, kick-electro, kick-lofi, kick-punchy, kick-snap,
// kick-tight, raw-kick, zgump-kick) and 10 new kicks added (kick-fat,
// kick-acoustic-heavy and the variant-8 PHYSICAL kicks). The new rows moved the
// loudness population median, so the 85 non-capped non-kick instruments moved by
// that sub-dB amplitude only — verified: every changed non-kick id is uncapped
// in instrument_loudness_gen.go and no capped instrument's checksum moved.
// Re-pinned 2026-08-22 (reference-audio-corpus tuning round 1, INTENTIONAL
// sound changes): bassoon, cello, harp, kalimba, oboe (+ oboe-full, which
// clones oboeSeed's gen1), and tubular-bells (full rebuild) were re-tuned
// against `refs diagnose`'s per-axis measurements to close listener-verdict
// gaps recorded in internal/audio/testdata/ear_verdicts.jsonl -- see
// docs/superpowers/specs/2026-08-22-verdict-to-metric-mapping.md and
// .superpowers/sdd/2026-08-19-reference-audio-corpus/tuning-round-1-report.md.
// No other instrument's golden moved; verified via `git stash` isolation that
// only these seven ids' checksums changed relative to this branch's baseline.
// Re-keyed 2026-07-08 (Phase-15 voice/choir re-categorization, NOT a sound
// change): the user ear-tested the six new voice/choir instruments and ruled
// four of them synths, not voices. choir-ahh/choir-ooh/voice-bass/voice-pad
// were renamed to ensemble-lead/ensemble-lead-dark/ghost-bass/viola-pad and
// moved out of Voice; the same {n, bits} values are re-keyed under the new
// ids (byte-identical renders — see per-id comments below). voice-soprano and
// voice-whisper are untouched.
// Re-pinned 2026-09-28 (INTENTIONAL, ride + the four hi-hats only): ride rebuilt
// as a modal plate against its jazz-ride reference (gen_cymbal_modal in
// modular_stages.c) and lengthened 1.0s -> 1.8s to carry the ring; hihat,
// hihat-open-bright, hihat-lofi and hihat-pedal rebuilt on the same plate with
// collision rattle against an acoustic closed hi-hat, then re-pinned to the takes
// the user picked by ear (closed "H4 tight", open "O2" with a quiet legacy
// metal layer; the lofi/pedal ids share hihatSeed). New 2026-09-29: snare-deep and
// tom-high-acoustic (organic layered modal head + driven-plate wires). No other
// golden moved.
var synthGoldens = map[instrument.ID]struct {
	n    int
	bits uint64
}{
	roster.Accordion:          {48000, 0x8368832bf176ce5e},
	roster.Banjo:              {36000, 0x5c0010e9f1ae9e29},
	roster.Bass808:            {36000, 0x3d92af2d6f49a54c},
	roster.BassAcid:           {36000, 0xe7d4a8266f01038c},
	roster.BassFm:             {36000, 0xdf93e155683ccf96},
	roster.BassGuitar:         {36000, 0x268f584b9ceece30},
	roster.BassReese:          {36000, 0x4f5e7ad49dae0ec8},
	roster.BassWobble:         {36000, 0x6599d385fbb6f06e},
	roster.Bassoon:            {48000, 0x7edf4edba6928aff},
	roster.BrassStab:          {36000, 0x6ad3af8ffc740e4e},
	roster.Celesta:            {48000, 0x43aa3c708f4fb2e1},
	roster.Cello:              {48000, 0xe454b408cca141ff},
	roster.CelloWarm:          {48000, 0xc3dd294e1ec968af},
	roster.Clap:               {24000, 0xdbc5a3e61b996645},
	roster.ClapBright:         {6000, 0x70b9f09962a8c491},
	roster.ClapLofi:           {7200, 0x35921ee4c5ff01a1},
	roster.ClapTight:          {6000, 0x07a8a183b0c84f36},
	roster.Clarinet:           {48000, 0xc0f12eae58fffa24},
	roster.Clavinet:           {36000, 0xb647596ebe26d7ba},
	roster.Conga:              {12000, 0xce3ed2996d6705b6},
	roster.CongaOpen:          {12000, 0x48f7a7bca343abbd},
	roster.CongaOrganic:       {38400, 0x062ffa17befc589c},
	roster.CongaOrganicDry:    {38400, 0xacb17fc683b15759},
	roster.CongaTumba:         {12000, 0x7b6e083992e4f1d8},
	roster.Cowbell:            {19200, 0x0f57270c96b54f9c},
	roster.CowbellDrive:       {7200, 0x649f4b134ebfb82c},
	roster.CowbellLofi:        {9600, 0xeeb009acc6d64fbc},
	roster.Crash:              {72000, 0x12120f57b5a652ff},
	roster.DnbKick:            {24000, 0x990f9c6793ba4aeb},
	roster.DoubleBass:         {48000, 0xf6c78cd5e2a9faff},
	roster.EnsembleLead:       {48000, 0xcb0e4859d3628b48},
	roster.EnsembleLeadDark:   {48000, 0xfccaa540395c0a02},
	roster.Flute:              {48000, 0x4393e8c6c0ab19dc},
	roster.FluteBreathy:       {48000, 0xf24a4c2fefa9e408},
	roster.FmBass:             {36000, 0xf2263013bf2675b4},
	roster.FmBassDrive:        {24000, 0xe2343f45c4fc2e60},
	roster.FmBell:             {48000, 0x9754f16f0f183694},
	roster.FmBellSoft:         {36000, 0xdc5bb9592083d2f7},
	roster.FmEpiano:           {48000, 0x55005c382044c808},
	roster.FmEpianoDrive:      {36000, 0x220cf63508bd9730},
	roster.FmLead:             {24000, 0xb53fad81c177f20e},
	roster.FmLeadCrush:        {16800, 0x67381a76076dbb17},
	roster.FmPluck:            {12000, 0x195865bfed74f8f2},
	roster.FmPluckTight:       {7200, 0x5f737b992cf17e35},
	roster.FrenchHorn:         {48000, 0xf3ab6b6594cb2d01},
	roster.FrenchHornLoud:     {48000, 0xc00cc84a77102d5d},
	roster.Gamelan:            {48000, 0xe91f8106b8c6ed08},
	roster.GhostBass:          {48000, 0x105697ce13c22432},
	roster.GlassArmonica:      {60000, 0x0c5bdd25911541f1},
	roster.GlassWash:          {60000, 0xa59ed34eae8a1c21},
	roster.Glockenspiel:       {48000, 0xca63a1dce1f7ac47},
	roster.GuitarElectric:     {36000, 0x2d040e7bb45b0ed8},
	roster.GuitarElectricNeck: {36000, 0x4c26b085e59b2d75},
	roster.GuitarNylon:        {36000, 0x4b3c38e92e710122},
	roster.GuitarNylonBright:  {36000, 0x44d71a44485e3422},
	roster.GuitarSteel:        {36000, 0x023ba1d656ed6116},
	roster.GuitarSteelWarm:    {36000, 0x13448b1d2314aaf9},
	roster.Harmonium:          {48000, 0x10eccf8f5fd1f4a9},
	roster.Harp:               {48000, 0xde349788a37ece32},
	roster.Harpsichord:        {48000, 0x38c59b72db78275e},
	roster.HighTomOrganic:     {21600, 0x625952d074d3b120},
	roster.Hihat:              {12000, 0x27e1ab853ab144de},
	roster.HihatLofi:          {4800, 0xd2309782c366bf82},
	roster.HihatOpenBright:    {16800, 0x93d6c14fb3126f0f},
	roster.HihatPedal:         {3600, 0xe731aeaf4f715c63},
	roster.Kalimba:            {36000, 0xe50985121b6fb76e},
	roster.Kick:               {24000, 0x6e45e1f487456d5e},
	roster.Kick808:            {36000, 0x6bd253957fb58463},
	roster.KickAcoustic:       {31200, 0x96d998e5c01df7f9},
	roster.KickAcousticHeavy:  {31200, 0x3970fee68faf349d},
	roster.KickAiry:           {24000, 0x20022d0201e5251e},
	roster.KickBoomBap:        {24000, 0x9de07a6a93e38c24},
	roster.KickBrutal:         {24000, 0x612d75a76f34dae4},
	roster.KickDeep:           {38400, 0x19aa8d873aed2632},
	roster.KickElectro:        {19200, 0x88b62790a908db40},
	roster.KickFat:            {24000, 0x90894630bceaec07},
	roster.KickLofi:           {12000, 0x8fc6fb760319f161},
	roster.KickMuted:          {24000, 0xc4cd307008111d2e},
	roster.KickMutedRoom:      {24000, 0x02780284c45ae387},
	roster.KickOrganicTight:   {24000, 0x41b98b247e2f0e43},
	roster.KickPunchy:         {12000, 0x160688cf5947a7d2},
	roster.KickRock:           {24000, 0xa110016d823ac450},
	roster.KickSnap:           {12000, 0xd4882305d50f7047},
	roster.KickTight:          {7200, 0xd8e8ce6b52fce823},
	roster.KickWood:           {24000, 0x7f220cfde47dc7ef},
	roster.Koto:               {48000, 0x8493fdd08b490370},
	roster.LeadChip:           {36000, 0xb5136907314d9bf6},
	roster.LeadHoover:         {48000, 0xe6fc649c49ceb4ad},
	roster.LeadSupersaw:       {48000, 0xedea4156b2365a9b},
	roster.Mandolin:           {36000, 0xdbb04e2b0ee8a3c1},
	roster.Marimba:            {36000, 0x6618933cdc34885d},
	roster.Modular:            {24000, 0xe21a3a540f3e9779},
	roster.ModularPad:         {24000, 0x3123d25a65731a2a},
	roster.MoogBass:           {36000, 0x038f2d02c45dbde7},
	roster.MusicBox:           {48000, 0xb95836131d01beac},
	roster.MutedTrumpet:       {48000, 0x6cba4cc3570b4ce5},
	roster.Oboe:               {48000, 0xc4df4f2186afa4c1},
	roster.OboeFull:           {48000, 0x195356cead56eb47},
	roster.Organ:              {48000, 0x082a2d709cde5ccf},
	roster.OrganChurch:        {48000, 0x4c79461388ecd694},
	roster.PanFlute:           {48000, 0xf95f809cd98c47a7},
	roster.PianoFelt:          {48000, 0x9544a8e5e77de7eb},
	roster.PianoGrand:         {48000, 0x9e9c1e0fa5596656},
	roster.PianoRhodes:        {48000, 0xb970c026fabc3c8e},
	roster.Piccolo:            {48000, 0x1c292dd035b53b58},
	roster.PizzStrings:        {36000, 0x26ac51ea45595321},
	roster.PwmPad:             {60000, 0x8dcb0790528017d1},
	roster.RawKick:            {16800, 0xd5f5b990bc2b8d3d},
	roster.Ride:               {86400, 0xd6312cf1ee78b301},
	roster.Rimshot:            {14400, 0xe753508fb8782670},
	roster.Sax:                {48000, 0x0e24230aa6021d31},
	roster.ScifiLead:          {48000, 0x83054709823b8098},
	roster.Shaker:             {14400, 0x8a91b38a1bc5e421},
	roster.Sidestick:          {12000, 0x7dce50117dc6989a},
	roster.SingingBowl:        {72000, 0xeb022def20591abc},
	roster.Sitar:              {48000, 0x5125332c02ed5b8f},
	roster.Snare:              {48000, 0xb8c0a015938c6ff1},
	roster.SnareAcoustic:      {67200, 0xefc8434f4e6cdda2},
	roster.SnareCrack:         {12000, 0xda8851610f7bb9c4},
	roster.SnareDeep:          {67200, 0x3b2251186c50bf02},
	roster.SnareFat:           {48000, 0x5b8f006376e261b9},
	roster.SnareGhost:         {9600, 0xc77e6668b4ca9bec},
	roster.SnareLofi:          {14400, 0xc0f808b51f2be12d},
	roster.SnarePunch:         {43200, 0x65fc2aa27e1afd3f},
	roster.SteelDrum:          {48000, 0x426141a3119e5394},
	roster.SubBass:            {48000, 0x5bc704ef07a047f7},
	roster.SubBassDrive:       {36000, 0x86039ed873ebc410},
	roster.Theremin:           {48000, 0xe5c4d5a10973bd09},
	roster.Tom:                {24000, 0xbff346d1ae230201},
	roster.TomHighAcoustic:    {67200, 0xfd1f1ea3d701896b},
	roster.TomHighDrive:       {7200, 0x0af7e2331d434a81},
	roster.TomLofi:            {16800, 0x5cf3da4e95d02d20},
	roster.Trombone:           {48000, 0xd90da067d236f9f9},
	roster.Trumpet:            {48000, 0xe1b14e0253806cc1},
	roster.TrumpetMellow:      {48000, 0xeb58381edb27e123},
	roster.Tuba:               {48000, 0x3557c87eccc80a9b},
	roster.TubularBells:       {72000, 0xd2fb8a90a579779f},
	roster.Ukulele:            {36000, 0x2b782b6456bb4c06},
	roster.Vibraphone:         {60000, 0xb969c3fd6c5b27e6},
	roster.Viola:              {48000, 0xde767767c5e314f0},
	roster.ViolaPad:           {48000, 0x1cd2a30c50bd72c1},
	roster.Violin:             {48000, 0xe16eb6d397a4c34c},
	roster.ViolinEnsemble:     {48000, 0xb0992e5b6f3bc838},
	roster.VoiceAhh:           {60000, 0xae8da2253d806c1b},
	roster.VoiceAlien:         {60000, 0x6f4d7dc7721196ee},
	roster.VoiceOpera:         {60000, 0x3bca036e1842bf0e},
	roster.VoiceSoprano:       {48000, 0x4b21d62ef7f20c69},
	roster.VoiceWhisper:       {48000, 0xf6ed5645bd195e72},
	roster.Xylophone:          {24000, 0x367259232b4c81f4},
	roster.ZgumpKick:          {16800, 0x950b42adba52e0c8},
}

// TestSynthGoldenByteIdentity asserts every built-in instrument still renders the
// exact pinned bytes. The safety net gating the pipeline refactor (P0).
// Re-pinned 2026-10-02 (98 uncapped instruments, gain only): sub-bass and
// sub-bass-drive became config rows (their VOICE stayed byte-identical), which
// put them into the loudness measurement — sub-bass now plays at the normalised
// level (0.8 -> 0.5155) and the larger set moved the median target, rescaling
// every uncapped amplitude by ~+0.3%. Verified a pure scalar: each changed render
// equals old x (newAmp/oldAmp) to 1.1e-7 of peak; the 52 capped ones are unchanged.
// Re-pinned 2026-09-01 (whole table): `make measure-loudness` was re-run after
// the bowed-string and kalimba retunes. The loudness table is a RELATIVE
// normalisation -- its target level L is derived from the whole instrument set
// -- so retuning any instrument nudges every amplitude, and these goldens
// incorporate ConfigForInstrument().Amplitude. With the OLD table only the 4
// bowed strings differ; the rest of this churn is the re-normalisation.
// The re-measure was not optional: the table had gone stale and pinned
// tubular-bells at 0.4122 against a measured 0.8608, playing it 6.4 dB below
// every other instrument in the game. Guarded now by TestLoudnessTableIsNotStale.
//
// Re-pinned 2026-09-01 (kalimba): genN_gain was never applied to the eight
// TRANSPLANTED generator voices (src 3-10) -- declared, seeded on 24
// instruments, read by nothing. kalimba is the only one that seeds a non-unity
// value: gen1_gain 0.4, with a comment saying the Karplus-Strong pluck is
// "pulled back" so the additive bar partials dominate. It had been running at
// full level, which is why the listener heard "a plucked string" instead of
// "the wooden marimba feel". Wiring it moved exactly this one checksum; the
// other 23 seed gen_gain=1 and are byte-identical (the float multiply is
// exact at unity). Found by TestSeedParamsAreLiveInTheirOwnInstrumentContext.
//
// Re-pinned 2026-09-01 (second pass, same four): the vibrato enabled above was
// METRONOMIC, which the listener caught immediately -- double-bass moved from
// "sounds like the real thing" to "the synth one has some vibrato or something
// that goes up and down in time". VibratoFingerprint.Jitter measured 0.000
// against the corpus double-bass reference's 0.411. modular_vibrato_phase now
// integrates a jittered rate (two slow incommensurate sines) with a third
// modulating depth, all deterministic so renders stay reproducible. Measured
// jitter after: cello 0.66, viola 0.10, violin 0.10, double-bass 0.10.
// Guarded by TestBowedStringVibratoIsNotMetronomic.
//
// Re-pinned 2026-09-01 (bowed strings only): cello / viola / violin /
// double-bass adopt the waveguide VIBRATO fix. The oscillator dispatch in
// modular.c is an if/else-if chain whose physical-model branch sits ahead of
// the vibrato branch, so `lfo_target:1` was silently dead on every osc_type
// 7-11 voice -- these four configured vibrato (3.6-4.4 Hz, 2.5-3.0 cents) and
// rendered dead-straight tones. modular_apply_vibrato_delay now runs inside
// that branch, gated on depth>0 && rate>0 so every other instrument stays
// byte-identical (confirmed: exactly these four checksums moved). Measured
// after the fix, the rendered vibrato matches the configured values to within
// 0.1 cent on all four.
// Re-pinned 2026-09-12 (six voices, two independent causes):
//
//	cello / viola / violin / double-bass — the bowed-string waveguide. Two
//	changes land together in render_bowed_string: the delay lines became
//	FRACTIONAL (they were rounded to integers, which left every note a roughly
//	constant ~1.2 samples short, so the sharpening grew with frequency and
//	reached +25 cents at the top of the violin's range), and the bow ramp-in
//	became long enough to select the right MODE. The old ~25 ms ramp drove the
//	nonlinear stick-slip oscillator into a higher limit cycle over wide bands —
//	cello sounded a TWELFTH high across node pitch +1..+6, viola/violin/
//	double-bass across -11..-7 — with the notated fundamental absent from the
//	spectrum entirely. Score-derived circuits therefore leapt register
//	mid-phrase, which is what made them sound dissonant rather than mistuned.
//
//	voice-ahh / voice-opera — unrelated: pitchenv_decay 0.7 -> 0.08, so the
//	pitch scoop no longer holds the note ~65 cents flat through its body.
//
// Guards: TestSelfOscillatingVoicesHoldTheirFundamental (mode lock),
// TestEveryPitchedInstrumentSoundsInTune (cents), and
// TestTemplateNodesSoundAtTheirNotatedPitch (committed templates end to end).
//
// Re-pinned 2026-10-25 (seven voices, one cause): the amp release that the
// ADSR caps at half the voice now RAMPS over that half. It kept the full
// release time, so a release longer than half the note left the voice loud
// at its end, where it hard-cut (modular-pad: −10 dB at the cut). Exactly the
// seven shipped documents whose amp_release exceeds half their voice at
// 120 BPM moved — ensemble-lead-dark, glass-armonica, glass-wash, modular-pad,
// pwm-pad, tubular-bells, viola-pad; every other voice is byte-identical.
// Guard: TestALongReleaseFadesOutInsideTheNote.
//
// Re-pinned 2026-10-27 (99 voices, one cause): a voice is render.seconds long
// at every tempo (render.beats deleted; seconds := the old beats × 0.5, what
// this capture's 120 BPM played — not one sample count moved). measure-loudness
// had measured each level over the old render.seconds, the WEB length, so it
// was re-run over the real one: exactly the 99 instruments whose
// render.amplitude moved changed bits (median 0.06 dB, at most 3 dB).
// Re-pinned 2026-10-27 (all 150, one cause): the engine runs at 48 kHz on
// both platforms (chain_spec.go EngineSampleRate; the browser always rendered
// at 48 kHz), so this one-shot renders at 48 kHz instead of 44.1 kHz — every
// sample count is ×48000/44100 and every checksum moved. No document changed.
func TestSynthGoldenByteIdentity(t *testing.T) {
	Reset()
	ResetInstruments()
	ids := append([]instrument.ID(nil), roster.All...)
	slices.Sort(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[string(id)] = true
		want, ok := synthGoldens[id]
		if !ok {
			t.Errorf("%q has no pinned golden — add it (an intentional new instrument?)", id)
			continue
		}
		n, bits := synthRenderChecksum(string(id))
		if n != want.n || bits != want.bits {
			t.Errorf("%q render changed: got {%d, 0x%016x} want {%d, 0x%016x}", id, n, bits, want.n, want.bits)
		}
	}
	for id := range synthGoldens {
		if !seen[string(id)] {
			t.Errorf("pinned golden %q is no longer a built-in instrument (removed/renamed?)", id)
		}
	}
}
