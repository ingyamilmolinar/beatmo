//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTimelineBandKeycapsShareOneSizeDesktop guards the desktop counter band:
// the lock/free keycap, the beat counter and the len −/+ pair are ONE row of
// equal keycaps — the lock and the ± are squares as tall as the counter's
// keycap, sharing its vertical centre — and none of them reaches down into the
// timeline ruler, which keeps its full width. (The lock used to be a 44 px
// gold block filling the left of the timeline, and the ± tall slabs spanning
// the whole header.)
func TestTimelineBandKeycapsShareOneSizeDesktop(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	dv := g.drum

	bc := dv.beatCounterRect
	tl := dv.timelineRect
	if bc.Empty() || tl.Empty() {
		t.Fatalf("beatCounterRect=%v timelineRect=%v — need both laid out", bc, tl)
	}
	keyH := TextHeight() + 2*beatCounterPillPadY + genGeomKeycapWallDepth
	if keyH > bc.Dy() {
		keyH = bc.Dy()
	}
	keyY := bc.Min.Y + (bc.Dy()-keyH)/2
	for _, c := range []struct {
		name string
		r    image.Rectangle
	}{
		{"track", dv.trackBtn().Rect()},
		{"len-", dv.lenDecBtn.Rect()},
		{"len+", dv.lenIncBtn.Rect()},
	} {
		if c.r.Empty() {
			t.Fatalf("%s rect is empty", c.name)
		}
		if c.r.Dx() != c.r.Dy() || c.r.Dy() != keyH || c.r.Min.Y != keyY {
			t.Errorf("%s = %v, want a %dpx square at y=%d (the counter keycap)", c.name, c.r, keyH, keyY)
		}
		if c.r.Max.Y > tl.Min.Y {
			t.Errorf("%s bottom %d reaches into the ruler (top %d)", c.name, c.r.Max.Y, tl.Min.Y)
		}
		if c.r.Overlaps(bc) {
			t.Errorf("%s %v overlaps the beat counter %v", c.name, c.r, bc)
		}
	}
	if tl.Min.X != dv.trackBtn().Rect().Min.X {
		t.Errorf("ruler starts at x=%d, want the full width from the lock's left %d", tl.Min.X, dv.trackBtn().Rect().Min.X)
	}
}

// TestTrackButtonKeycapIsRoundedAndNotWhite guards the visual treatment: the
// lock/free keycap is an ordinary rounded control keycap (a legacy
// ButtonStyle with Radius 0 draws square corners) and its icon is never
// the glaring near-white on-surface — both the tracking and free states use
// calmer tokens.
func TestTrackButtonKeycapIsRoundedAndNotWhite(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	z := g.drum.transportZone

	if z.trackBtn.SquareCorners {
		t.Errorf("track button must use the control radius, not square corners")
	}
	if st, ok := z.trackBtn.Style.(ButtonStyle); !ok || st.Radius != RadiusControl {
		t.Errorf("track button style %+v must carry the control radius", z.trackBtn.Style)
	}

	// Tracking (locked) state: icon must not be the near-white on-surface.
	z.SetFollow(true)
	if rgbaEq(z.trackBtn.IconColor, colTextPrimary) {
		t.Errorf("locked lock icon is near-white colTextPrimary; want a calmer token")
	}
	// Free state: also not near-white.
	z.SetFollow(false)
	if rgbaEq(z.trackBtn.IconColor, colTextPrimary) {
		t.Errorf("free lock icon is near-white colTextPrimary; want a calmer token")
	}
}

// trackTouchRect is the lock keycap's EFFECTIVE touch target: its timeline
// hit area expanded exactly as HitIndex.At expands a Touch area. The visible
// keycap is compact; the finger target is what must meet TouchMinTarget.
func trackTouchRect(t *testing.T, dv *DrumView) image.Rectangle {
	t.Helper()
	if dv.timelineZone == nil {
		t.Fatal("timeline zone missing")
	}
	dv.timelineZone.RefreshButtonHitAreas()
	for _, a := range dv.timelineZone.HitAreas() {
		if a.Tag != "timeline-track" {
			continue
		}
		r := a.Rect
		if a.Touch {
			r = expandRect(r, TouchMinTarget())
			if !a.ClipRect.Empty() {
				r = r.Intersect(a.ClipRect)
			}
		}
		return r
	}
	t.Fatal("no timeline-track hit area registered")
	return image.Rectangle{}
}
