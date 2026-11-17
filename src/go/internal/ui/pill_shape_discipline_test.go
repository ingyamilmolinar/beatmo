//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// pill_shape_discipline_test.go — the anti-pill invariant.
//
// A control whose corner radius reaches half its shorter side IS a pill. The
// app used to draw several that way by accident: RadiusMD (12 px) applied to a
// 22–26 px tall control lands at or past the pill threshold, which is how the
// beat counter and the synth action bar ended up fully rounded.
//
// The invariant: no rounded rect may request a radius greater than a QUARTER of
// its shorter side.
//
//	radius <= min(w, h) / 4
//
// drawRoundedRect enforces this at runtime by clamping, so a pill cannot reach
// the screen. This test enforces it at the SOURCE by intercepting the
// drawRoundedRect func var — it therefore observes the PRE-clamp requested
// radius and fails a call site that over-asks even though the clamp would have
// rescued it. Clamp = runtime guarantee; this test = call sites stay honest.
//
// See docs/superpowers/specs/2026-07-27-unified-button-geometry-design.md.

// pillViolation records one over-rounded draw request, attributed to the
// call site that made it so a failure is actionable without bisecting.
type pillViolation struct {
	w, h   int
	radius int
	allow  int
	site   string
}

func (v pillViolation) String() string {
	return fmt.Sprintf("%s: %dx%d rect requested radius %d (max %d)", v.site, v.w, v.h, v.radius, v.allow)
}

// callerSite walks up the stack past the drawing primitives to name the first
// frame in internal/ui that actually chose the radius.
func callerSite() string {
	var pcs [12]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		fr, more := frames.Next()
		base := filepath.Base(fr.File)
		switch base {
		case "drawing.go", "pill_shape_discipline_test.go", "components.go", "button_flat.go", "render.go":
			// primitives — keep walking to the caller that picked the radius
		default:
			if strings.HasSuffix(base, ".go") {
				return fmt.Sprintf("%s:%d", base, fr.Line)
			}
		}
		if !more {
			return "unknown"
		}
	}
}

// capturePillViolations swaps in an interceptor for drawRoundedRect, runs fn,
// and returns every draw request that exceeded the anti-pill ceiling. The
// interceptor forwards to the real implementation so rendering still happens
// (caches, sprite atlases and downstream draws all behave normally).
func capturePillViolations(t *testing.T, fn func()) []pillViolation {
	t.Helper()
	orig := drawRoundedRect
	seen := map[pillViolation]bool{}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if !r.Empty() && radius > 0 {
			if allow := maxRoundedRadius(r.Dx(), r.Dy()); radius > allow {
				seen[pillViolation{w: r.Dx(), h: r.Dy(), radius: radius, allow: allow, site: callerSite()}] = true
			}
		}
		orig(dst, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })

	fn()

	out := make([]pillViolation, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].radius != out[j].radius {
			return out[i].radius > out[j].radius
		}
		return out[i].h < out[j].h
	})
	return out
}

// TestNoPillShapedControls sweeps every scene in the catalog at both profiles,
// draws a frame, and fails on any rounded rect that asked for more than a
// quarter of its shorter side.
func TestNoPillShapedControls(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		profile := "desktop"
		w, h := 1280, 720
		if mobile {
			profile, w, h = "mobile", 390, 844
		}
		t.Run(profile, func(t *testing.T) {
			withSmallScreen(t, mobile)

			for _, s := range sceneCatalog {
				if mobile && !s.Mobile {
					continue
				}
				s := s
				t.Run(s.Name, func(t *testing.T) {
					g := New(testLogger)
					t.Cleanup(g.CloseForTest)
					// Several scenes arm the recorder, which spawns pipeline
					// worker goroutines that CloseForTest does not reap. Stop it
					// before teardown so this sweep doesn't trip goleak.
					t.Cleanup(func() {
						if g.IsRecording() {
							_, _ = g.StopRecording()
						}
					})
					g.Layout(w, h)

					run := RunScene
					if mobile {
						run = RunSceneMobile
					}
					if err := run(g, s.Name); err != nil {
						t.Fatalf("RunScene(%q): %v", s.Name, err)
					}

					screen := ebiten.NewImage(w, h)
					bad := capturePillViolations(t, func() {
						// Settle: layout + caches populate over a few frames,
						// exactly as the screenshot harness does.
						for i := 0; i < 4; i++ {
							_ = g.Update()
							g.Draw(screen)
						}
					})
					for _, v := range bad {
						t.Errorf("over-rounded draw: %s — use RadiusControl for controls, "+
							"RadiusContainer for panels; radius must be <= min(w,h)/4", v)
					}
				})
			}
		})
	}
}
