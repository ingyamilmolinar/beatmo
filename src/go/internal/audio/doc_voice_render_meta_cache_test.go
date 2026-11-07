//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The key must cover everything renderDocIntoCache reads, render metadata and voice FX included: two documents
// with one block but another amplitude must not share a voice, or what plays depends on what loaded first.
func TestVoiceCacheFollowsRenderMetadata(t *testing.T) {
	resetLibraryForTest(t)
	globalVoiceCache.Clear()
	const id = roster.Kick
	peak := func() float64 {
		buf, _ := RenderInstrumentOneShot(id)
		p := 0.0
		for _, v := range buf {
			p = math.Max(p, math.Abs(float64(v)))
		}
		return p
	}
	d, _ := InstrumentDoc(id)
	before := peak() // caches the voice at the shipped amplitude
	d.Render.Amplitude *= 0.5
	if err := SetInstrumentDoc(id, d); err != nil {
		t.Fatal(err)
	}
	if got := peak(); math.Abs(got-before*0.5) > before*0.01 {
		t.Fatalf("amplitude halved in the document, rendered peak %v -> %v (served the cached voice)", before, got)
	}
	n0, _ := RenderInstrumentOneShot(id)
	d.Render.Seconds *= 2
	if err := SetInstrumentDoc(id, d); err != nil {
		t.Fatal(err)
	}
	if n1, _ := RenderInstrumentOneShot(id); len(n1) == len(n0) {
		t.Fatalf("render.seconds doubled, render length unchanged (%d): served the cached voice", len(n1))
	}
}
