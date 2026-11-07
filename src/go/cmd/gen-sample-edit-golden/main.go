// Regenerates PIPE_GOLD in sample_edit_descriptor.browser.test.js: the native full-pipeline fingerprint of kick with a
// trim+gain+reverse edit, in the shape audio.js __testCaptureSynthRender reports. Regenerate when the kick, normalisation or
// BakeSample numerics change. cd src/go && BEATMO_AUDIO_SAMPLE_RATE=48000 go run ./cmd/gen-sample-edit-golden
package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func main() {
	edit := audio.SampleEdit{StartFrac: 0.25, EndFrac: 0.875, GainDB: -6, Reverse: true}
	audio.SetSampleEdit(roster.Kick, edit)
	buf, sr := audio.RenderInstrumentOneShot(roster.Kick)
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
