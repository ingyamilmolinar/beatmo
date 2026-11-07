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

// A corner radius reaching half the shorter side is a pill; the rule is radius <= min(w, h) / 4.
// drawRoundedRect clamps at runtime; intercepting it sees the PRE-clamp request, so a call site that over-asks
// fails here even though the clamp would rescue it.

// Attributed to its call site so a failure is actionable without bisecting.
type pillViolation struct {
	w, h   int
	radius int
	allow  int
	site   string
}

func (v pillViolation) String() string {
	return fmt.Sprintf("%s: %dx%d rect requested radius %d (max %d)", v.site, v.w, v.h, v.radius, v.allow)
}

// The first internal/ui frame past the drawing primitives, the one that chose the radius.
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

// Forwards to the real implementation so caches and later draws behave normally.
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
					// Scenes arm the recorder, whose worker goroutines CloseForTest does not reap; stopped first so goleak passes.
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
						// A few frames populate layout and caches, as the screenshot harness does.
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
