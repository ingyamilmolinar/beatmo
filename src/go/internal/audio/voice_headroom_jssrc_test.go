package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What the browser COMPUTES is compared per sample by xplat_audio_stages; these guard what a reader can see in the
// source.

func browserAudioSource(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "js", name))
	if err != nil {
		t.Fatalf("%s not readable: %v", name, err)
	}
	return string(b)
}

// Attenuated BEFORE they sum, as on desktop, or N voices sum ~5× hot and clip the master.
func TestWebAudioAppliesVoiceHeadroom(t *testing.T) {
	src := browserAudioSource(t, "audio.js")
	if !strings.Contains(src, "return CHAIN_SPEC.voiceHeadroom * Math.max(0,") {
		t.Errorf("audio.js voiceLevel no longer applies CHAIN_SPEC.voiceHeadroom to a note's volume")
	}
	if strings.Contains(src, "linearRampToValueAtTime(1, when") {
		t.Errorf("audio.js: a voice gain ramps to 1.0 (no headroom) — voices will sum hot and clip the master")
	}
}

// A copied number is a second source of truth nothing reconciles.
func TestBrowserAudioHasNoMixLiterals(t *testing.T) {
	comment := regexp.MustCompile(`(?m)//.*$|/\*[\s\S]*?\*/`)
	rate := regexp.MustCompile(`\b(44100|48000|22050|96000)\b`)
	for _, name := range []string{"audio.js", "played_voice.js", "dsp_host.js", "dsp_worklet.js", "voice_post.js", "synth_render_worker.js"} {
		code := comment.ReplaceAllString(browserAudioSource(t, name), "")
		if m := rate.FindString(code); m != "" {
			t.Errorf("%s: sample-rate literal %s — use CHAIN_SPEC.sampleRate", name, m)
		}
	}
	src := browserAudioSource(t, "audio.js")
	for _, ref := range []string{"CHAIN_SPEC.sampleRate", "CHAIN_SPEC.fadeInMs", "CHAIN_SPEC.stopFadeMs", "CHAIN_SPEC.gateFadeMs", "CHAIN_SPEC.softClipThreshold", "CHAIN_SPEC.voiceHeadroom"} {
		if !strings.Contains(src, ref) {
			t.Errorf("audio.js never reads %s — where does that value come from now?", ref)
		}
	}
}
