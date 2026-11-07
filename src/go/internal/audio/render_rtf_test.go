//go:build !test && !js

package audio

import (
	"math"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every built-in must render far faster than real time, or live edits and instrument switches stall the trigger path.
// Aggregate, and the thread's CPU time rather than wall time, so neither one cold outlier nor other packages' load
// flakes it; BEATMO_RTF_MIN tightens it locally.
func TestP5_RenderRealTimeFactor(t *testing.T) {
	minRTF := 20.0
	if s := os.Getenv("BEATMO_RTF_MIN"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v > 0 {
			minRTF = v
		}
	}
	Reset()
	ResetInstruments()

	runtime.LockOSThread() // the renders run on this goroutine (cgo stays on its thread)
	defer runtime.UnlockOSThread()
	var audioSec, renderSec float64
	worstID, worstRTF := "", math.Inf(1)
	for _, id := range roster.All {
		start := renderCPUSeconds()
		buf, sr := RenderInstrumentOneShotRaw(id)
		el := renderCPUSeconds() - start
		if len(buf) == 0 || sr <= 0 {
			t.Fatalf("%q rendered empty (len=%d sr=%d)", id, len(buf), sr)
		}
		aSec := float64(len(buf)) / float64(sr)
		audioSec += aSec
		renderSec += el
		if el > 0 {
			if rtf := aSec / el; rtf < worstRTF {
				worstRTF, worstID = rtf, string(id)
			}
		}
	}
	if renderSec <= 0 {
		t.Fatal("zero render time — clock did not advance")
	}
	aggregate := audioSec / renderSec
	t.Logf("aggregate RTF %.0fx over %d instruments (%.1fs audio in %.2fs); worst %q %.0fx",
		aggregate, len(roster.All), audioSec, renderSec, worstID, worstRTF)
	if aggregate < minRTF {
		t.Errorf("aggregate render real-time factor %.1fx below floor %.0fx — the pipeline got slower", aggregate, minRTF)
	}
}
