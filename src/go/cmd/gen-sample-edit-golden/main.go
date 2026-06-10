// gen-sample-edit-golden regenerates the full-pipeline sample-edit golden
// embedded in src/js/sample_edit_descriptor.browser.test.js (PIPE_GOLD). Run:
//
//	cd src/go && AUDIO_SAMPLE_RATE=48000 ../../.tools/go/bin/go run ./cmd/gen-sample-edit-golden
//
// Emits the native dispatcher's full pipeline output fingerprint for "kick"
// with a trim+gain+reverse descriptor: recipe render → normalizeAndScale →
// ApplySampleEditToBuffer (all via audio.RenderInstrumentOneShot, the same
// code path the voice dispatcher takes on a trigger). The fingerprint shape
// (head/tail/peak/rms/length) mirrors audio.js __testCaptureSynthRender so
// the browser test can compare the production WASM pipeline directly.
// Regenerate whenever the kick recipe, normalizeAndScale, or BakeSample
// numerics intentionally change.
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func main() {
	edit := audio.SampleEdit{StartFrac: 0.25, EndFrac: 0.875, GainDB: -6, Reverse: true}
	audio.SetSampleEdit("kick", edit)
	buf, sr := audio.RenderInstrumentOneShot("kick")
	if len(buf) == 0 {
		fmt.Fprintln(os.Stderr, "empty render")
		os.Exit(1)
	}
	var peak, sum float64
	for _, v := range buf {
		a := math.Abs(float64(v))
		if a > peak {
			peak = a
		}
		sum += float64(v) * float64(v)
	}
	rms := math.Sqrt(sum / float64(len(buf)))

	// Mirror __testCaptureSynthRender's fingerprint shape exactly.
	head := buf[:64]
	tailStart := max(len(buf)-256, 0)
	tail := buf[tailStart : tailStart+64]

	out := map[string]any{
		"sr":     sr,
		"length": len(buf),
		"peak":   peak,
		"rms":    rms,
		"head":   head,
		"tail":   tail,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	_ = enc.Encode(out)
}
