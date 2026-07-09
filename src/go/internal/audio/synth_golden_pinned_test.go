//go:build !test && !js

package audio

import (
	"sort"
	"testing"
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
// Re-pinned 2026-07-06 (snare family only): snare/snare-1/snare-2/snare-ghost
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
// cowbell-1/tom-1/fm-*-1 that inherit their base id's amp) exactly the set of
// checksums that moved here — no C/DSP change, no unrelated instrument's
// sound actually changed. Only sax and the two Phase-15 voice-family blocks
// carry their own more specific comments below.
// Re-keyed 2026-07-08 (Phase-15 voice/choir re-categorization, NOT a sound
// change): the user ear-tested the six new voice/choir instruments and ruled
// four of them synths, not voices. choir-ahh/choir-ooh/voice-bass/voice-pad
// were renamed to ensemble-lead/ensemble-lead-dark/ghost-bass/viola-pad and
// moved out of Voice; the same {n, bits} values are re-keyed under the new
// ids (byte-identical renders — see per-id comments below). voice-soprano and
// voice-whisper are untouched.
var synthGoldens = map[string]struct {
	n    int
	bits uint64
}{
	"bass-808":    {33075, 0x16988587817c9322},
	"bass-acid":   {33075, 0xfd93d57740e729c3},
	"bass-fm":     {33075, 0xceb49b62d1dd526d},
	"bass-guitar": {33075, 0x182b42e2dd29deb3},
	"bass-reese":  {33075, 0xccfaf302bab86551},
	"cello":       {44100, 0xc4bb2f1a01b70bc8},
	"cello-warm":  {44100, 0xa0db7dff1b0cb1c6},
	"clap":        {22050, 0x5c9380e43b2cafce},
	"clap-1":      {5512, 0xdb062cb5305e2365},
	"clap-2":      {6615, 0xa2f9c6f4edc3ee3e},
	"clap-tight":  {5512, 0xf5df356fddc81c6e},
	"conga":       {11025, 0x0d18b05e1385e53a},
	"conga-open":  {11025, 0xe15392ee2f456b0c},
	"conga-tumba": {11025, 0xa5265516aee5bd71},
	"cowbell":     {17640, 0xa0bbe7a5ee20d404},
	"cowbell-1":   {6615, 0x96ac8f02a0129623},
	"cowbell-2":   {8820, 0x4bd6702b43af33e0},
	"crash":       {66150, 0xa7db070c9bc82e55},
	"dnb-kick":    {22050, 0xd621d79ad8e6f2b5},
	// ensemble-lead(-dark) were born as choir-ahh/choir-ooh; renamed and
	// re-categorized to CatLead per the 2026-07-08 ear review (byte-identical
	// render — see the Phase-15 rename note above).
	"ensemble-lead":      {44100, 0x5bf0a2c97a526e29},
	"ensemble-lead-dark": {44100, 0xb89abc44e27e0bcd},
	"flute":              {44100, 0x31ef6d1d600a271f},
	"flute-breathy":      {44100, 0xa4224c7ac22d9c0b},
	"fm-bass":            {33075, 0xf2d31f19ddf4f266},
	"fm-bass-1":          {22050, 0x1ccbb79346109198},
	"fm-bell":            {44100, 0xa949abee62b5df80},
	"fm-bell-1":          {33075, 0x8d9a2907a91a4299},
	"fm-epiano":          {44100, 0x0a2dfd2a79e8f5f2},
	"fm-epiano-1":        {33075, 0x33b34e5164391876},
	"fm-lead":            {22050, 0x49763e4e5db0ac11},
	"fm-lead-1":          {15434, 0xb5c775b24d444724},
	"fm-pluck":           {11025, 0x7c46abf891c1eb1a},
	"fm-pluck-1":         {6615, 0x8aa60eb7a202c1bd},
	"french-horn":        {44100, 0xa62575efe5e32542},
	// french-horn-loud re-pinned 2026-07-08: its seed's gen1 noise gain was
	// trimmed 0.04->0.03 (breath-noise rebalance) without a re-pin; the render
	// is deterministic at the new bytes.
	"french-horn-loud":   {44100, 0xbfc922cfe4d93108},
	// ghost-bass was born as voice-bass; renamed and re-categorized to CatBass
	// per the 2026-07-08 ear review (byte-identical render).
	"ghost-bass":           {44100, 0xf07dee8098473c2c},
	"guitar-electric":      {33075, 0x778aa7285ba74aa7},
	"guitar-electric-neck": {33075, 0xb240a49aaf61b360},
	"guitar-nylon":         {33075, 0xbabe9778e66875bd},
	"guitar-nylon-bright":  {33075, 0xe58f400ea80248d6},
	"guitar-steel":         {33075, 0xfa9de2fdaa8531d8},
	"guitar-steel-warm":    {33075, 0x1257f2e17e911b31},
	"harp":                 {44100, 0x0bcfa2ee2f87ae61},
	"high-tom-organic":     {19845, 0xb31127f68ea99171},
	"hihat":                {11025, 0xf0c39bd8c8192146},
	"hihat-1":              {15434, 0xf4d1fe136a35c90b},
	"hihat-2":              {4410, 0xa2fd6c9810b88f29},
	"hihat-pedal":          {3307, 0x53934574b344f142},
	"kick":                 {22050, 0x37657e6eb663c5eb},
	"kick-1":               {6615, 0xdffda245c8db743b},
	"kick-2":               {11025, 0xd2768974b854193c},
	"kick-808":             {33075, 0x536edb3ed04ff55a},
	"kick-acoustic":        {28665, 0x1c4f14d63e2c2972},
	"kick-deep":            {35280, 0x5fc2d77ff9ba716a},
	"kick-electro":         {11025, 0xce975e4b64fb9b82},
	"kick-punchy":          {11025, 0x4469513e2556bdad},
	"kick-tight":           {6615, 0xe38a2d2a53273829},
	"modular":              {22050, 0x1f7462a0840e6c0f},
	"modular-pad":          {22050, 0x7d02dad675752138},
	"oboe":                 {44100, 0xeb26eab6c4e1e37e},
	"oboe-full":            {44100, 0x4041aa3c16f49be1},
	"organ":                {44100, 0xa012c79fb0782e32},
	"organ-church":         {44100, 0x0f1371bf3c5a06bc},
	"piano-felt":           {44100, 0x41f5ca014bd6edb2},
	"piano-grand":          {44100, 0x0dee4b9db95c5aeb},
	"raw-kick":             {15434, 0x6f76ef1ac70fb08a},
	"ride":                 {110250, 0xd6397ac45cf3cc65},
	"rimshot":              {13230, 0xc975928875779f65},
	// sax re-pinned 2026-07-05: intentional sound change — the retuned bari-sax
	// seed (hybrid additive + formant saw) is now BAKED into the table Render
	// (bakedModularRender), so the one-shot golden covers the seeded voice
	// instead of the bare modular tone (TestSaxNoEditFastPathMatchesRecipe).
	"sax":            {44100, 0x3ca4a63d468cb258},
	"scifi-lead":     {44100, 0x9a8f5a803301346a},
	"shaker":         {13230, 0x6d9ec9345e4e172a},
	"sidestick":      {11025, 0x3ec30ced219e7ed2},
	"snare":          {44100, 0x21052f62164ba936},
	"snare-1":        {11025, 0x7b9ac59f5a3bc745},
	"snare-2":        {13230, 0x2c300dc035678802},
	"snare-ghost":    {8820, 0x127d61221e5303bf},
	"sub-bass":       {44100, 0x17566a2c9cd4c81a},
	"sub-bass-1":     {33075, 0x739b69824e3927b5},
	"tom":            {22050, 0xa775b23432dded3d},
	"tom-1":          {6615, 0x3315d65e2423647e},
	"tom-2":          {15434, 0x584537ffd75d2b62},
	"trumpet":        {44100, 0x9aec45bfb8822dc8},
	"trumpet-mellow": {44100, 0xcbac93913d85b408},
	// viola-pad was born as voice-pad; renamed and re-categorized to
	// CatStrings per the 2026-07-08 ear review (byte-identical render).
	"viola-pad":       {44100, 0xf03b7ce4bf5f1a7c},
	"violin":          {44100, 0xa33f1205b1b24581},
	"violin-ensemble": {44100, 0xfee54ce3d5d9ebd9},
	// Phase-15 voice family — pinned at initial seed values; re-pin after
	// ear-tuning. Only voice-soprano/voice-whisper remain here; the other
	// four Phase-15 ids were re-categorized as synths (see above).
	"voice-soprano": {44100, 0xc8f47cd6db351ab7},
	// voice-ahh: sung male "ahh" tuned against a real reference recording
	// 2026-07-08 (see voiceAhhSeed). 55125 samples = its 2.5 s DurationSec.
	"voice-ahh":     {55125, 0x393990562a9b0fe2},
	// voice-opera: breathier "old-man opera" variant of voice-ahh, ear-
	// approved 2026-07-08 round-2 review (see voiceOperaSeed). Reconstruction
	// verified md5-identical to the approved audition WAV before pinning.
	"voice-opera":   {55125, 0xb0a6704a4ca4ca68},
	"voice-whisper": {44100, 0x73d0311d403c274a},
	"zgump-kick":    {15434, 0x858c338aa5592b63},
}

// TestSynthGoldenByteIdentity asserts every built-in instrument still renders the
// exact pinned bytes. The safety net gating the pipeline refactor (P0).
func TestSynthGoldenByteIdentity(t *testing.T) {
	Reset()
	ResetInstruments()
	ids := append([]string(nil), BuiltinInstrumentIDs...)
	sort.Strings(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
		want, ok := synthGoldens[id]
		if !ok {
			t.Errorf("%q has no pinned golden — add it (an intentional new instrument?)", id)
			continue
		}
		n, bits := synthRenderChecksum(id)
		if n != want.n || bits != want.bits {
			t.Errorf("%q render changed: got {%d, 0x%016x} want {%d, 0x%016x}", id, n, bits, want.n, want.bits)
		}
	}
	for id := range synthGoldens {
		if !seen[id] {
			t.Errorf("pinned golden %q is no longer a built-in instrument (removed/renamed?)", id)
		}
	}
}
