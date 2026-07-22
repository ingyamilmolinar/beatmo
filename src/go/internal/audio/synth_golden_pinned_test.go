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
	"accordion":            {44100, 0x036684763d7878d9},
	"banjo":                {33075, 0xb86b95f174e182a1},
	"bass-808":             {33075, 0xb77e326e7b2e65ee},
	"bass-acid":            {33075, 0xbea7cea02feac4bd},
	"bass-fm":              {33075, 0x6f0c2727af3dc541},
	"bass-guitar":          {33075, 0x5a165d74ac2c6994},
	"bassoon":              {44100, 0x62d29ae4b2bf1779},
	"bass-reese":           {33075, 0xe2ccf3d356455062},
	"bass-wobble":          {33075, 0x3318473319fd95ef},
	"brass-stab":           {33075, 0xff1e6812cf312723},
	"celesta":              {44100, 0x4b5e327b34dcfd97},
	"cello":                {44100, 0x935475378afe8c60},
	"cello-warm":           {44100, 0xd822990ba08333f3},
	"clap-1":               {5512, 0xdb062cb5305e2365},
	"clap":                 {22050, 0x5c9380e43b2cafce},
	"clap-2":               {6615, 0xa2f9c6f4edc3ee3e},
	"clap-tight":           {5512, 0xf5df356fddc81c6e},
	"clarinet":             {44100, 0xfe9106fbbe930c3e},
	"clavinet":             {33075, 0xc2e17c39832c2b46},
	"conga":                {11025, 0x0d18b05e1385e53a},
	"conga-open":           {11025, 0xe15392ee2f456b0c},
	"conga-tumba":          {11025, 0xa5265516aee5bd71},
	"cowbell-1":            {6615, 0x537e25732f24f548},
	"cowbell":              {17640, 0x34bd4e72b04571d7},
	"cowbell-2":            {8820, 0xb1afdb5b489009ba},
	"crash":                {66150, 0xa7db070c9bc82e55},
	"dnb-kick":             {22050, 0xd621d79ad8e6f2b5},
	"double-bass":          {44100, 0xbe00945f914b4f2d},
	"ensemble-lead":        {44100, 0x466466c669087e99},
	"ensemble-lead-dark":   {44100, 0x750fa79542fc2732},
	"flute":                {44100, 0xb9a408f69a253f69},
	"flute-breathy":        {44100, 0x193740b32ebcb586},
	"fm-bass-1":            {22050, 0xe6b807aca774b4c3},
	"fm-bass":              {33075, 0xc2e25a9f24d12766},
	"fm-bell-1":            {33075, 0x300027be3997a7ad},
	"fm-bell":              {44100, 0xea8097eb9c03509f},
	"fm-epiano-1":          {33075, 0x9c8932fd083cff79},
	"fm-epiano":            {44100, 0xdd07eaf806c85e9e},
	"fm-lead-1":            {15434, 0x2a071a2e78a37c7c},
	"fm-lead":              {22050, 0x7f1237d0da820c5e},
	"fm-pluck":             {11025, 0x7e30cbab1804409f},
	"fm-pluck-1":           {6615, 0xb19e0eab30b722a7},
	"french-horn":          {44100, 0x67dc41361b32e428},
	"french-horn-loud":     {44100, 0x308c94c15ef401a9},
	"gamelan":              {44100, 0x740011c288fb55a9},
	"ghost-bass":           {44100, 0x79e1cb9ec150c9c4},
	"glass-armonica":       {55125, 0x1ee22676b31348b9},
	"glass-wash":           {55125, 0x4470f324615f2bae},
	"glockenspiel":         {44100, 0x56f60a6cca758423},
	"guitar-electric":      {33075, 0x7aac10c420afb7d0},
	"guitar-electric-neck": {33075, 0xc191b16a436ad242},
	"guitar-nylon":         {33075, 0x64fc3278f1e2cd7b},
	"guitar-nylon-bright":  {33075, 0xe58f400ea80248d6},
	"guitar-steel":         {33075, 0xfaa07c333f042127},
	"guitar-steel-warm":    {33075, 0x1257f2e17e911b31},
	"harmonium":            {44100, 0xe2d541a3da8eefe1},
	"harp":                 {44100, 0x98bbd7391b049510},
	"harpsichord":          {44100, 0x575a0971b75e932b},
	"high-tom-organic":     {19845, 0xb31127f68ea99171},
	"hihat":                {11025, 0xf0c39bd8c8192146},
	"hihat-1":              {15434, 0xf4d1fe136a35c90b},
	"hihat-2":              {4410, 0xa2fd6c9810b88f29},
	"hihat-pedal":          {3307, 0x53934574b344f142},
	"kalimba":              {33075, 0x36b833b8e2d85508},
	"kick-1":               {6615, 0xdffda245c8db743b},
	"kick-2":               {11025, 0xd2768974b854193c},
	"kick":                 {22050, 0x37657e6eb663c5eb},
	"kick-808":             {33075, 0xfac6a0f5f0422603},
	"kick-acoustic":        {28665, 0x1c4f14d63e2c2972},
	"kick-deep":            {35280, 0x9b674c43c296aeef},
	"kick-electro":         {11025, 0xce975e4b64fb9b82},
	"kick-punchy":          {11025, 0x14e2f90a9bf113f2},
	"kick-tight":           {6615, 0xe38a2d2a53273829},
	"koto":                 {44100, 0x028f4500a823c949},
	"lead-chip":            {33075, 0xa11fcda253b71d54},
	"lead-hoover":          {44100, 0xd2896ff300dc7d17},
	"lead-supersaw":        {44100, 0xaf9f5c206582556e},
	"mandolin":             {33075, 0x4ee727a4ac25b1fc},
	"marimba":              {33075, 0xcaa271571bc4c3fa},
	"modular":              {22050, 0xe9a8619fbc957e59},
	"modular-pad":          {22050, 0xad6695deaf016731},
	"moog-bass":            {33075, 0x0ae733a4c601a317},
	"music-box":            {44100, 0x1a2f963b50b3bc0d},
	"muted-trumpet":        {44100, 0x6795a81fe1f18f8c},
	"oboe":                 {44100, 0xca8ee30005e02054},
	"oboe-full":            {44100, 0x2478840acd5c6371},
	"organ":                {44100, 0x72b0d509e86c2903},
	"organ-church":         {44100, 0x0f1371bf3c5a06bc},
	"pan-flute":            {44100, 0xb2a72921545f8181},
	"piano-felt":           {44100, 0x4d0862958da83221},
	"piano-grand":          {44100, 0x752ad08901d8337d},
	"piano-rhodes":         {44100, 0xfce9e37af7594df7},
	"piccolo":              {44100, 0xdd3d490fb6419096},
	"pizz-strings":         {33075, 0x2921b25eb4647bcb},
	"pwm-pad":              {55125, 0x7a4e92fed3d05c65},
	"raw-kick":             {15434, 0xc7f909b01191a499},
	"ride":                 {110250, 0xd6397ac45cf3cc65},
	"rimshot":              {13230, 0x4c00bcc102a84525},
	"sax":                  {44100, 0x4a98bc107fc7b509},
	"scifi-lead":           {44100, 0x9a8f5a803301346a},
	"shaker":               {13230, 0x6d9ec9345e4e172a},
	"sidestick":            {11025, 0x3ec30ced219e7ed2},
	"singing-bowl":         {66150, 0x1a3403ecc24c5a25},
	"sitar":                {44100, 0xe4489d2ce98b1788},
	"snare-1":              {11025, 0x7b9ac59f5a3bc745},
	"snare-2":              {13230, 0x2c300dc035678802},
	"snare":                {44100, 0x21052f62164ba936},
	"snare-ghost":          {8820, 0x127d61221e5303bf},
	"steel-drum":           {44100, 0xff15ce556e3a709c},
	"sub-bass-1":           {33075, 0x739b69824e3927b5},
	"sub-bass":             {44100, 0x17566a2c9cd4c81a},
	"theremin":             {44100, 0x168415781a8b9502},
	"tom-1":                {6615, 0xa693718cc417c9ec},
	"tom-2":                {15434, 0xe6d2cb065d530e02},
	"tom":                  {22050, 0xb7f1f76b146256cb},
	"trombone":             {44100, 0x462f2d030c925f3f},
	"trumpet":              {44100, 0x75e96fe0f61bff62},
	"trumpet-mellow":       {44100, 0x17db9294fca39729},
	"tuba":                 {44100, 0xb96374e2b65e8c58},
	"tubular-bells":        {66150, 0x66a77ae13b397feb},
	"ukulele":              {33075, 0x10e5eb6242ac64a4},
	"vibraphone":           {55125, 0x313280633917735d},
	"viola":                {44100, 0x5153e3d81a2613cc},
	"viola-pad":            {44100, 0x3d0436aa5ddd07e3},
	"violin":               {44100, 0x58db1388f0ce2e06},
	"violin-ensemble":      {44100, 0xd21090269406dc46},
	"voice-ahh":            {55125, 0x393990562a9b0fe2},
	"voice-alien":          {55125, 0x3147d05c7bb7dc68},
	"voice-opera":          {55125, 0xb0a6704a4ca4ca68},
	"voice-soprano":        {44100, 0xda046912c7b247b4},
	"voice-whisper":        {44100, 0x73d0311d403c274a},
	"xylophone":            {22050, 0xc773bdc98878ff54},
	"zgump-kick":           {15434, 0x46c50af8e7c862f9},
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
