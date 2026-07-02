//go:build test

package ui

import (
	"image"
	"strconv"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// TestSynthMirror_FallsBackWhenBudgetExhausted reproduces the desktop crash
// where opening the Synth tab panicked with
// "async.Registry.MustGet(synth.preview): registry budget exhausted".
// newSynthMirror is created lazily mid-session (when the Synth tab is built),
// so it MUST gracefully fall back to a private pool — like eventlogger.format
// and hooks.fanout already do — instead of panicking and killing the app.
func TestSynthMirror_FallsBackWhenBudgetExhausted(t *testing.T) {
	reg := async.DefaultRegistry()
	// Free the shared preview slot so we exercise the exhaustion path even if a
	// prior test in this suite warmed it.
	_ = reg.Release(synthMirrorPoolName)

	// Saturate the shared worker budget with throwaway 1-worker pools.
	var filler []string
	for {
		bs := reg.BudgetStats()
		if bs.Used >= bs.Budget {
			break
		}
		name := "test.budget.filler." + strconv.Itoa(len(filler))
		if _, err := reg.Get(name, async.Options{MaxConcurrent: 1, QueueSize: 1, Name: name}); err != nil {
			break
		}
		filler = append(filler, name)
	}
	t.Cleanup(func() {
		for _, n := range filler {
			_ = reg.Release(n)
		}
	})

	// With the budget exhausted, construction must not panic.
	var m *synthMirror
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("newSynthMirror panicked on exhausted budget: %v", r)
			}
		}()
		m = newSynthMirror()
	}()
	t.Cleanup(m.closeForTest)

	// The fallback pool must still produce a render.
	rendered := 0
	m.request("kick", "h1", func(string) []float64 { rendered++; return []float64{0.5, -0.5} })
	m.drainForTest()
	if rendered != 1 || len(m.pcmForTest()) != 2 {
		t.Fatalf("fallback mirror did not render: rendered=%d pcm=%d", rendered, len(m.pcmForTest()))
	}
}

func TestSynthMirror_CachesByParamsHash(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	renders := 0
	render := func(inst string) []float64 { renders++; return []float64{0.1, 0.2} }
	m.request("kick", "hashA", render)
	m.drainForTest()
	m.request("kick", "hashA", render)
	m.drainForTest()
	if renders != 1 {
		t.Fatalf("expected 1 render for identical hash, got %d", renders)
	}
	m.request("kick", "hashB", render)
	m.drainForTest()
	if renders != 2 {
		t.Fatalf("expected 2 renders after hash change, got %d", renders)
	}
}

func TestSynthMirror_CoalescesRapidRequests(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	renders := 0
	render := func(inst string) []float64 { renders++; return []float64{1} }
	for i := 0; i < 10; i++ {
		m.request("kick", "h", render)
	}
	m.drainForTest()
	if renders != 1 {
		t.Fatalf("rapid identical requests must coalesce, got %d", renders)
	}
}

func TestSynthMirror_MovesPCMToGhostOnChange(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	m.request("kick", "h1", func(string) []float64 { return []float64{1, 2, 3} })
	m.drainForTest()
	if got := m.pcmForTest(); len(got) != 3 {
		t.Fatalf("want pcm len 3, got %d", len(got))
	}
	m.request("kick", "h2", func(string) []float64 { return []float64{4, 5} })
	m.drainForTest()
	if got := m.ghostForTest(); len(got) != 3 {
		t.Fatalf("previous pcm must become ghost, got len %d", len(got))
	}
	if got := m.pcmForTest(); len(got) != 2 {
		t.Fatalf("want new pcm len 2, got %d", len(got))
	}
}

func TestSynthMirror_ConsumeReadyClears(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	m.request("kick", "h1", func(string) []float64 { return []float64{1, 2} })
	m.drainForTest()
	if !m.consumeReady() {
		t.Fatalf("expected ready=true after a completed render")
	}
	if m.consumeReady() {
		t.Fatalf("ready must be one-shot (cleared after first consume)")
	}
}

// TestSynthMirrorChecksum_ReactsToContent is the Go-side coverage for the
// synthMirrorPCMChecksum bridge export the browser reactivity test relies on:
// empty → 0, stable for identical content, and DIFFERENT when the PCM differs
// (so a frozen render can't masquerade as a reactive one).
func TestSynthMirrorChecksum_ReactsToContent(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	dv := &DrumView{synthMirror: m}

	if got := dv.synthMirrorPCMChecksum(); got != 0 {
		t.Fatalf("empty mirror checksum = %d, want 0", got)
	}

	m.request("kick", "h1", func(string) []float64 { return []float64{0.1, 0.2, 0.3} })
	m.drainForTest()
	a := dv.synthMirrorPCMChecksum()
	if a == 0 {
		t.Fatal("non-empty mirror must have a nonzero checksum")
	}
	if b := dv.synthMirrorPCMChecksum(); a != b {
		t.Fatalf("checksum unstable for identical content: %d vs %d", a, b)
	}

	m.request("kick", "h2", func(string) []float64 { return []float64{0.9, -0.4, 0.2} })
	m.drainForTest()
	if c := dv.synthMirrorPCMChecksum(); c == a {
		t.Fatalf("checksum did not change with different content: still %d", c)
	}
}

func TestDrawSynthMirror_RendersTraceWithGhost(t *testing.T) {
	m := newSynthMirror()
	t.Cleanup(m.closeForTest)
	// First render becomes the ghost after the second.
	m.request("kick", "g1", func(string) []float64 {
		w := make([]float64, 64)
		for i := range w {
			w[i] = 0.8
		}
		return w
	})
	m.drainForTest()
	m.request("kick", "g2", func(string) []float64 {
		w := make([]float64, 64)
		for i := range w {
			w[i] = -0.6
		}
		return w
	})
	m.drainForTest()

	rect := image.Rect(0, 0, 80, 60)

	// Ink with both ghost + live present.
	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	both := collectFilledRects(t, func() {
		drawSynthMirror(dst, rect, m)
	})
	if len(both) == 0 {
		t.Fatalf("drawSynthMirror produced no ink")
	}

	// Ink with only a single (live) render, no ghost.
	m2 := newSynthMirror()
	t.Cleanup(m2.closeForTest)
	m2.request("kick", "s1", func(string) []float64 {
		w := make([]float64, 64)
		for i := range w {
			w[i] = -0.6
		}
		return w
	})
	m2.drainForTest()
	dst2 := ebiten.NewImage(rect.Dx(), rect.Dy())
	single := collectFilledRects(t, func() {
		drawSynthMirror(dst2, rect, m2)
	})
	if len(both) <= len(single) {
		t.Fatalf("ghost+live (%d) must produce more ink than single render (%d)", len(both), len(single))
	}
}
