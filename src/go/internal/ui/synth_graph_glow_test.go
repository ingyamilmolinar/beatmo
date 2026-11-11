//go:build test

package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// The trigger glow (spec 2026-10-24 §4.7) shines for as long as the note
// sounds — its own envelope — and never hides the resting picture.

func TestGlowFollowsTheNoteEnvelope(t *testing.T) {
	const sr = 48000
	pcm := make([]float32, sr)
	for i := range pcm {
		tt := float64(i) / sr
		pcm[i] = float32(0.8 * math.Exp(-tt/0.1) * math.Sin(2*math.Pi*220*tt))
	}
	env := soundview.New(pcm, sr, 1).Envelope()
	start := time.Unix(5000, 0)
	now := start
	g := &synthGlow{now: func() time.Time { return now }}
	if g.envelopeGlow(&env) != 0 {
		t.Fatal("no trigger: no glow")
	}
	g.start()
	for _, ms := range []int{0, 40, 99} {
		now = start.Add(time.Duration(ms) * time.Millisecond)
		if k := g.envelopeGlow(&env); k != 1 {
			t.Fatalf("at %d ms the glow is %.3f, want full for the first 100 ms", ms, k)
		}
	}
	now = start.Add(200 * time.Millisecond)
	k := g.envelopeGlow(&env)
	want := math.Exp(-(0.2 - 0.01) / 0.1) // the envelope re its peak, linear
	if k <= 0 || math.Abs(k-want) > 0.08 {
		t.Fatalf("at 200 ms the glow is %.3f, want the note's own level ≈ %.3f", k, want)
	}
	prev := k
	for ms := 220; ms <= 440; ms += 20 {
		now = start.Add(time.Duration(ms) * time.Millisecond)
		if k := g.envelopeGlow(&env); k > prev+1e-9 || k < 0 {
			t.Fatalf("at %d ms the glow %.3f rose above %.3f or went negative", ms, k, prev)
		} else {
			prev = k
		}
	}
	now = start.Add(600 * time.Millisecond) // past -40 dB (0.46 s)
	if k := g.envelopeGlow(&env); k != 0 || g.glowing {
		t.Fatalf("the glow must end once the note is 40 dB down (got %.3f, glowing=%v)", k, g.glowing)
	}
}

func TestTriggerFromAMutedRowDoesNotGlow(t *testing.T) {
	g := newViolinMacroGame(t)
	layoutSynthFor(t, g, channel.Of(roster.Violin))
	fire := func() bool {
		g.drum.graphGlow().glowing = false
		g.hlCh <- struct {
			row, idx int
			info     model.BeatInfo
		}{row: 0, idx: 0, info: model.BeatInfo{NodeID: g.drum.Rows[0].Origin}}
		g.drainAndDecayHighlights()
		return g.drum.graphGlow().glowing
	}
	g.drum.Rows[0].Muted = true
	if fire() {
		t.Fatal("a muted row's trigger made the graph glow (nothing was heard)")
	}
	g.drum.Rows[0].Muted = false
	if !fire() {
		t.Fatal("an audible row's trigger must make the graph glow")
	}
}

// A glowing frame costs one blit: the glow is baked once per picture and
// theme, never re-tessellated each frame (with a beat playing the glow is on
// almost every frame).
func TestGlowIsBakedOncePerPicture(t *testing.T) {
	g := newDialGame(t)
	pipe, _ := manualGraphPipe(t, g)
	drawGraphPane(g)
	pipe.drainForTest()
	drawGraphPane(g)
	start := time.Unix(9000, 0)
	glow := g.drum.graphGlow()
	glow.now = func() time.Time { return start.Add(30 * time.Millisecond) }
	glow.glowStart, glow.glowing = start, true
	for i := 0; i < 20; i++ {
		drawGraphPane(g)
	}
	if b := g.drum.synp().glow.builds; b != 1 {
		t.Fatalf("20 glowing frames baked the glow %d times, want once", b)
	}
	ApplyTheme(theme.Doc{Preset: "midnight"})
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	drawGraphPane(g)
	if b := g.drum.synp().glow.builds; b != 2 {
		t.Fatalf("a theme change must re-bake the glow once (builds %d, want 2)", b)
	}
}
