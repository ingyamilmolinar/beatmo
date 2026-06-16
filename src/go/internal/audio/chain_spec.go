package audio

// chain_spec.go is the SINGLE SOURCE OF TRUTH for the post-voice master/mix
// chain configuration that must be identical on desktop (Go/C) and browser
// (WebAudio JS nodes).
//
// The two platforms implement this chain with completely different DSP:
//   - Desktop: the Go 3-phase mixer (engine_stop.go) + the custom peak-detect
//     Compressor (compressor.go) + a tanh soft-clip before int16 conversion.
//   - Browser: WebAudio nodes in src/js/audio.js — a DynamicsCompressorNode and
//     a WaveShaper limiter.
//
// Two implementations can never be bit-identical, but they CAN — and must —
// share the same CONFIGURATION (compressor threshold/ratio/attack/release/knee,
// soft-clip threshold, per-voice headroom) so the platforms sound the same.
// Historically they drifted (compressor -3/2.5 desktop vs -6/4 browser; desktop
// had a soft-clip the browser lacked), which is exactly the audible
// desktop↔browser difference this spec exists to prevent.
//
// Flow of truth:
//   chain_spec.go (these consts)
//     → compressor.go / engine_stop.go read them (desktop)
//     → cmd/gen-chain-spec emits src/js/chain_spec.gen.js (browser reads it)
//     → chain_spec_test.go (Go) + chain_spec_parity.browser.test.js (browser)
//       assert the live values on each platform equal the spec, and a
//       gen-and-diff drift test keeps chain_spec.gen.js from going stale.
//
// This file carries NO build tag so it compiles under every GOOS/-tags combo
// (the desktop mixer that consumes it is !test && !js, but the spec itself and
// its tests/generator must be buildable everywhere).

// Master compressor configuration. Desktop: compressor.go NewCompressor.
// Browser: audio.js ensureCompressor (attack/release are seconds there, i.e.
// these ms values ÷ 1000).
//
// NOTE on the algorithm gap: the Go compressor is a custom peak-detecting
// envelope follower; the browser uses WebAudio's DynamicsCompressorNode (an
// RMS-ish algorithm). Matching these parameters makes the two as close as the
// two algorithms allow — full bit-parity is impossible by construction, which
// is why Layer-3 full-mix parity is correlation-graded, not exact.
const (
	CompressorThresholdDB = -3.0
	CompressorRatio       = 2.5
	CompressorAttackMs    = 5.0
	CompressorReleaseMs   = 80.0
	CompressorKneeDB      = 3.0
)

// SoftClipThreshold is the absolute level above which the master output is
// gently saturated with tanh instead of hard-clamped. Desktop: engine_stop.go
// final conversion loop. Browser: the soft-clip region baked into the
// WaveShaper limiter curve (audio.js ensureLimiter). out = T*tanh(x/T) for
// |x| > T, then a safety clamp to [-1, 1].
const SoftClipThreshold = 0.9

// VoiceHeadroom is the per-voice gain applied BEFORE accumulation in the
// desktop mixer (engine_stop.go Phase 1) to leave room for overlapping voices.
// Exposed here so the value lives in one place and the browser parity surface
// can reason about it. (Distinct from the legacy exported MixHeadroom=0.25 in
// config.go, which the browser does not consume; the value the desktop mixer
// actually applies is this one.)
//
// CRITICAL — this MUST be applied on BOTH platforms. The browser (audio.js)
// ramps every voice's anti-pop gain to CHAIN_SPEC.voiceHeadroom; omitting it
// makes N voices sum ~5x hot and slam the master limiter into audible clipping
// when a full circuit plays (the 2026-06 browser-clipping bug). Guarded by
// TestWebAudioAppliesVoiceHeadroom (source) + TestTemplateMasterMix (headroom).
// See docs/master-mix-dsp.md.
const VoiceHeadroom = 0.18

// TargetSampleRate is the sample rate the cross-platform parity fixtures and
// the offline full-mix render pin to, matching the browser's typical
// AudioContext.sampleRate so synth voices render the same length + content on
// both sides. The desktop runtime default (44100) is unrelated to parity; the
// fixtures and OfflineFullMix force this value.
const TargetSampleRate = 48000

// ChainSpec is a flat, serializable snapshot of the canonical chain
// configuration. cmd/gen-chain-spec marshals it into src/js/chain_spec.gen.js
// and the Go/browser parity tests compare live values against it.
type ChainSpec struct {
	CompressorThresholdDB float64 `json:"compressorThresholdDb"`
	CompressorRatio       float64 `json:"compressorRatio"`
	CompressorAttackMs    float64 `json:"compressorAttackMs"`
	CompressorReleaseMs   float64 `json:"compressorReleaseMs"`
	CompressorKneeDB      float64 `json:"compressorKneeDb"`
	SoftClipThreshold     float64 `json:"softClipThreshold"`
	VoiceHeadroom         float64 `json:"voiceHeadroom"`
	TargetSampleRate      int     `json:"targetSampleRate"`
}

// CanonicalChainSpec returns the single source-of-truth chain configuration.
func CanonicalChainSpec() ChainSpec {
	return ChainSpec{
		CompressorThresholdDB: CompressorThresholdDB,
		CompressorRatio:       CompressorRatio,
		CompressorAttackMs:    CompressorAttackMs,
		CompressorReleaseMs:   CompressorReleaseMs,
		CompressorKneeDB:      CompressorKneeDB,
		SoftClipThreshold:     SoftClipThreshold,
		VoiceHeadroom:         VoiceHeadroom,
		TargetSampleRate:      TargetSampleRate,
	}
}
