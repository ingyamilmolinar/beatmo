//go:build !test && !js

package audio

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestP5_RenderRealTimeFactor is the performance gate for the one-declarative-
// pipeline refactor (spec P5): rendering EVERY builtin instrument one-shot must
// be dramatically faster than real time, or live param edits and instrument
// switches stall the trigger path. The floor is deliberately conservative
// (20× aggregate — dev machines measure far higher) so CI variance never
// flakes it; override with BEATMO_RTF_MIN for local tightening. Aggregate
// (total audio seconds / total wall seconds) rather than per-instrument so
// one cold-cache outlier cannot flake the gate; the worst single instrument
// is Logf'd for visibility.
func TestP5_RenderRealTimeFactor(t *testing.T) {
	minRTF := 20.0
	if s := os.Getenv("BEATMO_RTF_MIN"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			minRTF = v
		}
	}
	Reset()
	ResetInstruments()

	var audioSec, wallSec float64
	worstID, worstRTF := "", math.Inf(1)
	for _, id := range BuiltinInstrumentIDs {
		start := time.Now()
		buf, sr := RenderInstrumentOneShotRaw(id)
		el := time.Since(start).Seconds()
		if len(buf) == 0 || sr <= 0 {
			t.Fatalf("%q rendered empty (len=%d sr=%d)", id, len(buf), sr)
		}
		aSec := float64(len(buf)) / float64(sr)
		audioSec += aSec
		wallSec += el
		if el > 0 {
			if rtf := aSec / el; rtf < worstRTF {
				worstRTF, worstID = rtf, id
			}
		}
	}
	if wallSec <= 0 {
		t.Fatal("zero wall time — clock did not advance")
	}
	aggregate := audioSec / wallSec
	t.Logf("aggregate RTF %.0fx over %d instruments (%.1fs audio in %.2fs); worst %q %.0fx",
		aggregate, len(BuiltinInstrumentIDs), audioSec, wallSec, worstID, worstRTF)
	if aggregate < minRTF {
		t.Errorf("aggregate render real-time factor %.1fx below floor %.0fx — the pipeline got slower", aggregate, minRTF)
	}
}
