//go:build !test && !js

package audio

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// template_master_mix_test.go — master-mix headroom guard. The combined mix must
// keep headroom below the soft-clip knee (CHAIN_SPEC.SoftClipThreshold = 0.9) so
// the master compressor/limiter is a safety net, NOT a constantly-slammed
// distortion stage. This is what caused the browser "clipping when played
// together": WebAudio omitted the per-voice headroom (VoiceHeadroom=0.18) that
// native applies, so voices summed ~5x hot and saturated the limiter. The fix
// (audio.js applies CHAIN_SPEC.voiceHeadroom) is regression-guarded in the
// browser by chain_spec_parity / master_headroom.browser.test.js; this test
// locks the NATIVE side and the arrangement-level "is the mix too hot" budget.

// allHitsPeak schedules ONE hit of every (instrument,volume) at t=0 — the
// worst-case aligned downbeat — and returns the pre-limiter master peak.
func allHitsPeak(t *testing.T, insts []struct {
	id  string
	vol float64
}) float64 {
	t.Helper()
	ResetInstruments()
	sr := sampleRate
	m := &mixer{workBuf: make([]float64, blockSize), voiceTemp: make([]float64, blockSize), masterBuf: make([]float64, blockSize), postEQBuf: make([]float64, blockSize), instSlots: make(map[string]int)}
	for _, in := range insts {
		v := newRecipeAwareVoice(in.id, 120, sr)
		if v == nil {
			continue
		}
		m.Schedule(in.id, &scaledVoice{v: v, gain: in.vol}, 0)
	}
	StartOutputCapture()
	readMixerSamples(m, sr) // 1s captures the full transient
	return analyzeAudio(StopOutputCapture(), sr).peak
}

func TestTemplateMasterMix(t *testing.T) {
	// Templates are authoritative JSON circuits. Worst case: every instrument
	// fires on the same downbeat. The pre-limiter master peak must stay below the
	// 0.9 soft-clip knee so the limiter is a safety net, not a constantly-slammed
	// distortion stage. Same all-hits-on-the-downbeat strategy as the startup
	// circuit below; synth params + insert effects are applied as on import.
	for _, tp := range assets.Templates() {
		d := parseTemplateDoc(t, tp.Genre, tp.Bytes)
		peak := renderTemplateAllHitsPeak(t, d)
		headroomDB := 20 * math.Log10(0.9/math.Max(peak, 1e-9))
		t.Logf("%-9s all-hits-downbeat pre-limiter peak=%.3f  (%.1f dB below 0.90)", tp.Genre, peak, headroomDB)
		if peak > 0.9 {
			t.Errorf("%s: aligned-downbeat peak %.3f > 0.9 — clips the master when everything hits together (raise per-voice headroom / lower volumes)", tp.Genre, peak)
		}
	}

	// Startup circuit: worst-case all-instruments-on-the-downbeat peak.
	var doc struct {
		Instruments []struct {
			ID     string  `json:"id"`
			Volume float64 `json:"volume"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(assets.StartupDemoJSON, &doc); err != nil {
		t.Fatalf("startup demo: %v", err)
	}
	insts := make([]struct {
		id  string
		vol float64
	}, 0, len(doc.Instruments))
	for _, in := range doc.Instruments {
		vol := in.Volume
		if vol == 0 {
			vol = 1
		}
		insts = append(insts, struct {
			id  string
			vol float64
		}{in.ID, vol})
	}
	peak := allHitsPeak(t, insts)
	t.Logf("startup  all-hits-downbeat pre-limiter peak=%.3f (%d instruments)", peak, len(insts))
	if peak > 0.9 {
		t.Errorf("startup circuit: aligned-downbeat peak %.3f > 0.9 — clips the master when everything hits together", peak)
	}
}
