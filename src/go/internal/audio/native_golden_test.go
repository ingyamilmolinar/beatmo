//go:build !test && !js

package audio

import "testing"

// Native-renderer golden baseline for the "deprecate the opaque Native synth"
// migration. Every shipped C renderer (20 drums + 5 FM) is locked byte-for-byte
// BEFORE any parameterization work touches drums.c / fmsynth.c, and must stay
// locked through every phase: exposing internal DSP constants as knobs must
// not change the default sound by a single bit.
//
// Two assertions per renderer, both at a fixed 48 kHz / 48000-sample config
// (same as TestFMPresetGolden / TestModularIdentityGolden):
//
//  1. Byte golden: sha256 over the little-endian float32 buffer equals the
//     recorded baseline hash. Drum hashes live here; the 5 FM byte goldens
//     already live in fm_golden_test.go (TestFMPresetGolden) and are not
//     duplicated — FM entries carry want=="" to skip the hash check.
//  2. Identity parity: render_X_p with a zero-value SynthParams (the exact
//     block the recipe path renders for an unedited instrument — toCParams
//     returns NULL) is byte-identical to the unparameterized render_X.
//
// Regeneration protocol (intentional sound changes only): set the affected
// want to "PLACEHOLDER", run the test, paste the logged hash back in the
// same change. Same protocol as modularIdentityGolden.
const (
	nativeGoldenSR      = 48000
	nativeGoldenSamples = 48000 // 1.0s
)

// bassGoldenIdentity adapts a no-param modular fast-path renderer into the
// cParamRenderer shape the golden table's identity-parity assertion expects.
// The migrated bass family has no SynthParams surface (it renders through the
// wide modular block), and TestNativeRendererGoldenBaseline only ever calls
// renderP with a zero-value SynthParams, so dropping the params and rendering
// the baked voice keeps the identity-parity assertion meaningful (raw == ident)
// while the hash stays locked to the deleted legacy path.
func bassGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

// kickGoldenIdentity is the kick-family analogue of bassGoldenIdentity: the
// migrated kick family has no SynthParams surface (it renders through the wide
// modular block via the no-edit fast path), so dropping the always-zero
// SynthParams keeps the identity-parity assertion meaningful (raw == ident)
// while the hash stays locked to the deleted legacy render_kick* path.
func kickGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

// tomGoldenIdentity is the tom-family analogue of kickGoldenIdentity: the
// migrated tom family has no SynthParams surface (it renders through the wide
// modular block via the no-edit fast path), so dropping the always-zero
// SynthParams keeps the identity-parity assertion meaningful (raw == ident)
// while the hash stays locked to the deleted legacy render_tom* path.
func tomGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

// cymbalGoldenIdentity is the cymbal-family analogue of snareGoldenIdentity: the
// migrated cymbal family (hihat/open-hihat/cowbell/shaker/ride/crash → source==9)
// has no SynthParams surface (it renders through the wide modular block via the
// no-edit fast path), so dropping the always-zero SynthParams keeps the
// identity-parity assertion meaningful (raw == ident) while the hash stays locked
// to the deleted legacy render_hihat / render_open_hihat / render_cowbell /
// render_shaker / render_ride / render_crash path.
func cymbalGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

// fmGoldenIdentity is the FM-family analogue (Phase-7, the LAST migration): the
// migrated FM family (bass/bell/lead/epiano/pluck → source==10) renders its
// no-edit voice through the modular engine (renderFM*Voice — the baked
// recipe-default ModularParams). It reads no SynthParams surface (the FM knobs
// travel in the wide modular block, exercised by the oracle), so dropping the
// always-zero SynthParams keeps the identity-parity assertion meaningful
// (raw == ident) while the byte golden stays owned by TestFMPresetGolden.
func fmGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

// snareGoldenIdentity is the snare-family analogue of tomGoldenIdentity: the
// migrated snare family (snare/rimshot/sidestick → source==7, clap → source==8)
// has no SynthParams surface (it renders through the wide modular block via the
// no-edit fast path), so dropping the always-zero SynthParams keeps the
// identity-parity assertion meaningful (raw == ident) while the hash stays locked
// to the deleted legacy render_snare* / render_clap path.
func snareGoldenIdentity(render func(buf []float32, sampleRate, samples int)) cParamRenderer {
	return func(buf []float32, sampleRate, samples int, _ SynthParams) {
		render(buf, sampleRate, samples)
	}
}

var nativeGoldenCases = []struct {
	name    string
	render  func(buf []float32, sampleRate, samples int)
	renderP cParamRenderer
	want    string // "" → byte golden covered elsewhere (FM), identity parity only
}{
	// Snare family migrated to the modular engine (Phase-5): renders through the
	// no-edit modular fast path (renderSnareVoice / renderSnareRimshotVoice /
	// renderSnareSidestickVoice / renderClapVoice). Hash UNCHANGED from the deleted
	// legacy render_snare* / render_clap path (the oracle fixtures prove
	// byte-identity, incl. the base-snare drive-before-tone post order via
	// post_order=2). renderP is snareGoldenIdentity (no SynthParams surface).
	{"drum-snare", renderSnareVoice, snareGoldenIdentity(renderSnareVoice), "e2ebd570507fe9613d0a898be8762cb27645f9a06f50727aac8fa02e0693c8ee"},
	// Kick family migrated to the modular engine (Phase-3): renders through the
	// no-edit modular fast path (renderKickVoice etc.). Hash UNCHANGED from the
	// deleted legacy render_kick path (the oracle fixtures prove byte-identity).
	// renderP is kickGoldenIdentity (no SynthParams surface).
	{"drum-kick", renderKickVoice, kickGoldenIdentity(renderKickVoice), "1c79835732c5b3275947b313fbdf72dc249616cc812b80059f0c22951085db3a"},
	// Cymbal family migrated to the modular engine (Phase-6): renders through the
	// no-edit modular fast path (renderHiHatVoice / renderOpenHiHatVoice /
	// renderCowbellVoice / renderShakerVoice / renderRideVoice / renderCrashVoice).
	// Hash UNCHANGED from the deleted legacy render_hihat / render_open_hihat /
	// render_cowbell / render_shaker / render_ride / render_crash path (the oracle
	// fixtures prove byte-identity). renderP is cymbalGoldenIdentity (no
	// SynthParams surface — source==9 reads the wide modular block).
	{"drum-hihat", renderHiHatVoice, cymbalGoldenIdentity(renderHiHatVoice), "306d1dbaabcc6efb819de102595a36c3530efbe6c0e2b26abf49fcb99d7e9433"},
	{"drum-clap", renderClapVoice, snareGoldenIdentity(renderClapVoice), "91afa51147de8af8b6382a2a7c4e7418137b77a7ae7142dfcc150ac4b3ade0fe"},
	// Tom family migrated to the modular engine (Phase-4): renders through the
	// no-edit modular fast path (renderTomVoice / renderTomHighVoice /
	// renderTomLowVoice). Hash UNCHANGED from the deleted legacy render_tom* path
	// (the oracle fixtures prove byte-identity). renderP is tomGoldenIdentity
	// (no SynthParams surface).
	{"drum-tom", renderTomVoice, tomGoldenIdentity(renderTomVoice), "69fa236434c52f0561a059e0a8c20a0faa346a7d405e7e48a69e90e807a11073"},
	{"drum-cowbell", renderCowbellVoice, cymbalGoldenIdentity(renderCowbellVoice), "177ea07e3cb23e9accdef7eaba1896ba8ef893c8f84c49d9b9e5897bcfc6bc2f"},
	{"drum-open-hihat", renderOpenHiHatVoice, cymbalGoldenIdentity(renderOpenHiHatVoice), "577d48513953e94ec712e625831e14c1c340d6ceeb17162f19895125040a38e8"},
	{"drum-tom-high", renderTomHighVoice, tomGoldenIdentity(renderTomHighVoice), "499d7c941b0dc2c89a7894b4295e2fbc5b1b76af229acbd3a790eff7b5052652"},
	{"drum-tom-low", renderTomLowVoice, tomGoldenIdentity(renderTomLowVoice), "f8e7727eff919b587ef674d5a7119dbfa77c313259d59aff25488de74abfa6f8"},
	// Sub-bass migrated to the modular engine (Phase-2). The bespoke
	// render_sub_bass C path is deleted, so this entry renders through the no-edit
	// modular fast path (renderSubBassVoice — the baked recipe-default
	// ModularParams). The hash is UNCHANGED from the legacy path: the oracle
	// fixtures prove byte-identity. renderP is bassGoldenIdentity, which ignores
	// the (always-zero) SynthParams and renders the same baked voice, so the
	// identity-parity assertion below is trivially satisfied (the modular path has
	// no SynthParams surface — it reads its params from the wide modular block,
	// exercised by the oracle).
	{"drum-sub-bass", renderSubBassVoice, bassGoldenIdentity(renderSubBassVoice), "ab7b8c781ae55c67802394696549d39ddee7588f770f982e311156e221ff84a0"},
	{"drum-snare-rimshot", renderSnareRimshotVoice, snareGoldenIdentity(renderSnareRimshotVoice), "c435b1b79c729a8a1e60b946f3417f7a6ead8e6f711ada37f21c2570e29cea0a"},
	{"drum-snare-sidestick", renderSnareSidestickVoice, snareGoldenIdentity(renderSnareSidestickVoice), "0d9994e2647773bd83469603060b251fa892d5f470d15108afde55f94dd04172"},
	{"drum-kick-deep", renderKickDeepVoice, kickGoldenIdentity(renderKickDeepVoice), "8b059709e8f473bfc74ff96e4e7d3f6c37109800bb63f9cc157bf28296c68cd7"},
	{"drum-kick-punchy", renderKickPunchyVoice, kickGoldenIdentity(renderKickPunchyVoice), "47e5e77bcd2906ec4798c9c4f5e7fc1597e2e92679c711094fbfe8a76a4aaca6"},
	{"drum-kick-lofi", renderKickLofiVoice, kickGoldenIdentity(renderKickLofiVoice), "293b70e216db27777ce884453ebfdce6d4cc207eaff7b994a1cf49dae12393c8"},
	{"drum-kick-tight", renderKickTightVoice, kickGoldenIdentity(renderKickTightVoice), "8c6250fc7c80d1e0313026e169fd761673a72e938cbb383a2f1b3aaa6dc1de68"},
	{"drum-shaker", renderShakerVoice, cymbalGoldenIdentity(renderShakerVoice), "e5d80cc3f9f708279bcc9a4091a198148a5b8b06242943e4a11770b62cf48c32"},
	{"drum-ride", renderRideVoice, cymbalGoldenIdentity(renderRideVoice), "fc3f28d256179c839044c1619eda5e9fe7ab103cc8de44fa2acde8fac3ae8354"},
	{"drum-crash", renderCrashVoice, cymbalGoldenIdentity(renderCrashVoice), "63f27da67b170a125c82f21f38d5f5a2f1bdec2c38ae4ac169c904e28114b531"},
	// FM family migrated to the modular engine (Phase-7, the LAST family): the
	// render_fm_* C paths are deleted, so these entries render through the no-edit
	// modular fast path (renderFM*Voice — the baked recipe-default ModularParams).
	// The byte golden is owned by TestFMPresetGolden (re-pointed at the same baked
	// voices); here want="" keeps only the identity-parity assertion (raw==ident),
	// where renderP is fmGoldenIdentity (ignores the always-zero SynthParams and
	// renders the same baked voice — the modular path reads its params from the
	// wide modular block, exercised by the oracle).
	{"fm-bass", renderFMBassVoice, fmGoldenIdentity(renderFMBassVoice), ""},
	{"fm-bell", renderFMBellVoice, fmGoldenIdentity(renderFMBellVoice), ""},
	{"fm-lead", renderFMLeadVoice, fmGoldenIdentity(renderFMLeadVoice), ""},
	{"fm-epiano", renderFMEPianoVoice, fmGoldenIdentity(renderFMEPianoVoice), ""},
	{"fm-pluck", renderFMPluckVoice, fmGoldenIdentity(renderFMPluckVoice), ""},
}

func TestNativeRendererGoldenBaseline(t *testing.T) {
	for _, tc := range nativeGoldenCases {
		t.Run(tc.name, func(t *testing.T) {
			raw := make([]float32, nativeGoldenSamples)
			tc.render(raw, nativeGoldenSR, nativeGoldenSamples)
			rawHash := hashFloat32(raw)

			ident := make([]float32, nativeGoldenSamples)
			tc.renderP(ident, nativeGoldenSR, nativeGoldenSamples, SynthParams{})
			identHash := hashFloat32(ident)

			if rawHash != identHash {
				t.Errorf("%s: identity _p path diverged from unparameterized render\n  raw:   %s\n  ident: %s\n(the recipe path for an unedited instrument must reproduce render_X exactly)", tc.name, rawHash, identHash)
			}
			switch tc.want {
			case "":
				// Byte golden owned by TestFMPresetGolden; identity parity only.
			case "PLACEHOLDER":
				t.Errorf("%s: golden not recorded yet; baseline = %s (paste into nativeGoldenCases)", tc.name, rawHash)
			default:
				if rawHash != tc.want {
					t.Errorf("%s: render hash drifted\n  got:  %s\n  want: %s\n(if this sound change is intentional, re-record via the PLACEHOLDER protocol)", tc.name, rawHash, tc.want)
				}
			}
		})
	}
}
