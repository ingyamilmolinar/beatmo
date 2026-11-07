package audio

// Every value the path between voice and speaker uses on both platforms; cmd/gen-chain-spec emits it into
// chain_spec.gen.js. The DSP is one C implementation on both, so these are the only numbers the sides could
// disagree on. Untagged.

// BEATMO_AUDIO_SAMPLE_RATE overrides the desktop device rate for diagnostics only.
const EngineSampleRate = 48000

// Identity up to the knee, then a smooth tanh approach to full scale.
const SoftClipThreshold = 0.9

// Applied BEFORE voices sum, so N voices on one beat stay below the knee.
const VoiceHeadroom = 0.18

// FadeInMs: every voice's anti-pop fade-in; StopFadeMs: a stopped or muted voice's end; GateFadeMs: the release of a
// melodic note shortened by its Duration.
const (
	FadeInMs   = 5
	StopFadeMs = 20
	GateFadeMs = 10
)

// SetMainVolume clamps to [0, MaxMasterVolume].
const MaxMasterVolume = 1.0

type ChainSpec struct {
	SampleRate        int     `json:"sampleRate"`
	SoftClipThreshold float64 `json:"softClipThreshold"`
	VoiceHeadroom     float64 `json:"voiceHeadroom"`
	FadeInMs          float64 `json:"fadeInMs"`
	StopFadeMs        float64 `json:"stopFadeMs"`
	GateFadeMs        float64 `json:"gateFadeMs"`
	MaxMasterVolume   float64 `json:"maxMasterVolume"`
	// Milliseconds.
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
