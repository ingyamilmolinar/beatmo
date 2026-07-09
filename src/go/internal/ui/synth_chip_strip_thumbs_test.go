//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// preseedNilPoolThumbs installs a pool-less synthStageThumbs on dv BEFORE the
// first ensureStageThumbs call, so render jobs stay pending until an explicit
// drainForTest — deterministic (no pool-worker race) and no registry pool /
// synthMirror is lazily constructed by the thumbs path (goleak hygiene).
func preseedNilPoolThumbs(dv *DrumView) *synthStageThumbs {
	th := &synthStageThumbs{}
	dv.stageThumbs = th
	return th
}

// TestSynthChipStrip_ThumbsRecoverFromEmptyChipEnsure pins the real-app bug
// where the FIRST ensure ran on a frame before the synth layout populated
// instEditorChips: the empty waves + unchanged params hash froze the cache
// empty forever, so no watermark ever drew in the shipped binary (native +
// WASM) while every test (which laid out first) passed. The async gate keys
// on the STORED chip count (wantN), so the 0-chip request must be superseded
// once chips exist.
func TestSynthChipStrip_ThumbsRecoverFromEmptyChipEnsure(t *testing.T) {
	g := newModularSynthTabGame(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	th := preseedNilPoolThumbs(dv)
	// Simulate the pre-layout frame: no chips yet.
	dv.instEditorChips = dv.instEditorChips[:0]
	dv.ensureStageThumbs(inst)
	th.drainForTest()
	// Now the layout runs and chips exist; the same hash must NOT freeze the
	// cache empty.
	layoutSynthTab(t, g)
	if len(dv.instEditorChips) == 0 {
		t.Fatalf("layout produced no chips")
	}
	dv.ensureStageThumbs(inst)
	th.drainForTest()
	if w := dv.stageThumbs.waveFor(0); len(w) == 0 {
		t.Fatalf("thumb cache stayed frozen-empty after chips appeared")
	}
}

// TestSynthChipStrip_WatermarksRespondToParams renders the chip strip through
// the fingerprint harness, flips osc_type sine->saw (which reshapes every
// stage thumbnail), and asserts the painted picture changed — proving the
// per-chip waveform watermarks follow the live synth params once the
// (debounced, off-thread in production) render job lands.
func TestSynthChipStrip_WatermarksRespondToParams(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	if inst == "" {
		t.Fatalf("no resolved synth instrument")
	}
	th := preseedNilPoolThumbs(dv)
	render := func() int {
		dv.ensureStageThumbs(inst)
		th.drainForTest()
		return fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthChipStrip(img) })
	}
	// Warm-up render: first draw primes lazy caption-fit/font caches, so the
	// baseline fingerprint is stable and the assertion isolates the watermark.
	_ = render()
	a := render()
	audio.SetInstrumentParam(inst, "osc_type", 1) // sine -> saw: every thumb changes
	b := render()
	if a == b {
		t.Fatalf("chip watermarks did not follow a param change (fingerprint=%d)", a)
	}
}
