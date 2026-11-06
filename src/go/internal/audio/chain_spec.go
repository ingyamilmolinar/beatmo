package audio

// chain_spec.go is the SINGLE SOURCE OF TRUTH for every value the audio path
// between a voice and the speaker uses on both platforms. Desktop reads these
// constants; cmd/gen-chain-spec emits them into src/js/chain_spec.gen.js for
// audio.js and the beatmo-dsp worklet (TestChainSpecGenFileIsCurrent keeps it
// current). The DSP itself is one implementation (the C engine: fx_host.c,
// mix_dsp.c) on both platforms, so these are the only numbers the two sides
// could disagree on — and xplat_audio_stages.browser.test.js compares what
// each side actually renders, stage by stage.
//
// This file carries NO build tag so it compiles under every GOOS/-tags combo.

// EngineSampleRate is the rate the engine runs at on both platforms: the desktop
// device (BEATMO_AUDIO_SAMPLE_RATE overrides it for diagnostics) and the
// browser's AudioContext and every voice render.
const EngineSampleRate = 48000

// SoftClipThreshold is the master soft clip's knee: identity up to it, then a
// smooth tanh approach to full scale (mix_dsp.c soft_clip, both platforms).
const SoftClipThreshold = 0.9

// VoiceHeadroom is the per-voice gain applied BEFORE voices sum, so N voices
// on one beat stay below the soft-clip knee (docs/master-mix-dsp.md).
const VoiceHeadroom = 0.18

// FadeInMs is every voice's linear fade-in (anti-pop), StopFadeMs the linear
// fade a stopped or muted voice ends with, GateFadeMs the release of a
// melodic note shortened by its node's Duration.
const (
	FadeInMs   = 5
	StopFadeMs = 20
	GateFadeMs = 10
)

// MaxMasterVolume is the top of the master volume range (the transport
// slider's full scale); SetMainVolume clamps to [0, MaxMasterVolume].
const MaxMasterVolume = 1.0

// ChainSpec is a flat, serializable snapshot of the shared configuration.
// cmd/gen-chain-spec marshals it into src/js/chain_spec.gen.js.
type ChainSpec struct {
	SampleRate        int     `json:"sampleRate"`
	SoftClipThreshold float64 `json:"softClipThreshold"`
	VoiceHeadroom     float64 `json:"voiceHeadroom"`
	FadeInMs          float64 `json:"fadeInMs"`
	StopFadeMs        float64 `json:"stopFadeMs"`
	GateFadeMs        float64 `json:"gateFadeMs"`
	MaxMasterVolume   float64 `json:"maxMasterVolume"`
	// Residency (residency.go): the release threshold and hold, and each
	// effect's memory, in milliseconds.
	ReleaseQuietDB float64            `json:"releaseQuietDB"`
	ReleaseHoldMs  float64            `json:"releaseHoldMs"`
	EffectMemoryMs map[string]float64 `json:"effectMemoryMs"`
}

// CanonicalChainSpec returns the single source-of-truth configuration.
func CanonicalChainSpec() ChainSpec {
	return ChainSpec{
		SampleRate:        EngineSampleRate,
		SoftClipThreshold: SoftClipThreshold,
		VoiceHeadroom:     VoiceHeadroom,
		FadeInMs:          FadeInMs,
		StopFadeMs:        StopFadeMs,
		GateFadeMs:        GateFadeMs,
		MaxMasterVolume:   MaxMasterVolume,
		ReleaseQuietDB:    ReleaseQuietDB,
		ReleaseHoldMs:     ReleaseHoldSec * 1000,
		EffectMemoryMs:    effectMemoryMs(),
	}
}

func effectMemoryMs() map[string]float64 {
	out := make(map[string]float64, len(effectMemorySec))
	for t, sec := range effectMemorySec {
		out[string(t)] = sec * 1000
	}
	return out
}
