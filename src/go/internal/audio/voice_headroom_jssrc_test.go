package audio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// voice_headroom_jssrc_test.go locks the WebAudio per-voice headroom fix. The
// browser path MUST attenuate every voice by CHAIN_SPEC.voiceHeadroom before the
// voices sum, exactly like native (engine_stop.go renderVoiceIntoInstBuf applies
// mixHeadroom=VoiceHeadroom=0.18). Without it, N voices sum ~5x hot and slam the
// master compressor/limiter into audible clipping when a full circuit plays —
// the "clipping when played together" bug. This is a source-contract guard so it
// runs in the fast suite (no browser/WASM needed); the live-node parity lives in
// chain_spec_parity.browser.test.js.
func TestWebAudioAppliesVoiceHeadroom(t *testing.T) {
	path := filepath.Join("..", "..", "..", "js", "audio.js")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("audio.js not readable (%v) — skipping JS source guard", err)
	}
	src := string(b)
	// Both per-voice paths must ramp the anti-pop gain to the headroom constant.
	const want = "linearRampToValueAtTime(CHAIN_SPEC.voiceHeadroom, when + 0.005)"
	if n := strings.Count(src, want); n < 2 {
		t.Errorf("audio.js: per-voice fade ramps to voiceHeadroom %d times, want >= 2 (render + sample paths)", n)
	}
	// The old hot ramp (to unity) must be gone — that's the regression.
	if strings.Contains(src, "linearRampToValueAtTime(1, when + 0.005)") {
		t.Errorf("audio.js: a per-voice anti-pop gain still ramps to 1.0 (no headroom) — voices will sum hot and clip the master")
	}
}
