//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fakeGridLayer is a draw-only participant that records draw order.
type fakeGridLayer struct {
	id      string
	z       int
	vis     bool
	drawLog *[]string
}

func (f *fakeGridLayer) ID() string         { return f.id }
func (f *fakeGridLayer) ZIndex() int        { return f.z }
func (f *fakeGridLayer) Visible() bool      { return f.vis }
func (f *fakeGridLayer) Draw(*ebiten.Image) { *f.drawLog = append(*f.drawLog, f.id) }

func TestGridTreeDrawsAscendingZ(t *testing.T) {
	tr := NewGridTree()
	var log []string
	tr.RegisterLayer(&fakeGridLayer{id: "hi", z: 60, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "lo", z: 10, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "mid", z: 40, vis: true, drawLog: &log})
	tr.SetBounds(image.Rect(0, 0, 100, 100))

	dst := ebiten.NewImage(100, 100)
	tr.Draw(dst)

	want := []string{"lo", "mid", "hi"}
	if len(log) != 3 || log[0] != want[0] || log[1] != want[1] || log[2] != want[2] {
		t.Fatalf("draw order: got %v want %v", log, want)
	}
}

func TestGridTreeSkipsInvisibleLayers(t *testing.T) {
	tr := NewGridTree()
	var log []string
	tr.RegisterLayer(&fakeGridLayer{id: "shown", z: 10, vis: true, drawLog: &log})
	tr.RegisterLayer(&fakeGridLayer{id: "hidden", z: 20, vis: false, drawLog: &log})
	tr.SetBounds(image.Rect(0, 0, 50, 50))
	tr.Draw(ebiten.NewImage(50, 50))
	if len(log) != 1 || log[0] != "shown" {
		t.Fatalf("expected only 'shown' drawn, got %v", log)
	}
}

// fakeGridZone implements Zone with controllable hit areas + visibility.
type fakeGridZone struct {
	id        string
	areas     []HitArea
	needs     bool
	layoutHit int
}

func (z *fakeGridZone) ID() string                       { return z.id }
func (z *fakeGridZone) Layout(image.Rectangle)           { z.layoutHit++; z.needs = false }
func (z *fakeGridZone) Update()                          {}
func (z *fakeGridZone) HitAreas() []HitArea              { return z.areas }
func (z *fakeGridZone) Draw(*ebiten.Image)               {}
func (z *fakeGridZone) NeedsLayout() bool                { return z.needs }
func (z *fakeGridZone) Invalidate()                      { z.needs = true }
func (z *fakeGridZone) HandleKey(ebiten.Key) InputResult { return InputIgnored }
func (z *fakeGridZone) HandleChars([]rune) InputResult   { return InputIgnored }

func TestGridTreePublishesHitAreasOnLayout(t *testing.T) {
	tr := NewGridTree()
	z := &fakeGridZone{id: "z1", needs: true, areas: []HitArea{
		{Rect: image.Rect(10, 10, 30, 30), ZIndex: GZCanvas, Tag: "a"},
	}}
	tr.RegisterZone(z, GZCanvas)
	tr.SetZoneRect("z1", image.Rect(0, 0, 100, 100))
	tr.layoutPass()

	hits := tr.HitIndexRef().At(20, 20)
	if len(hits) != 1 || hits[0].Tag != "a" {
		t.Fatalf("expected hit 'a' at (20,20), got %+v", hits)
	}
}

func TestGridTreeInvisibleZoneClearsHitAreas(t *testing.T) {
	tr := NewGridTree()
	vis := true
	z := &fakeGridZone{id: "z1", needs: true, areas: []HitArea{
		{Rect: image.Rect(0, 0, 50, 50), ZIndex: GZSidebar, Tag: "panel"},
	}}
	tr.RegisterZoneVisible(z, GZSidebar, func() bool { return vis })
	tr.SetZoneRect("z1", image.Rect(0, 0, 50, 50))
	tr.layoutPass()
	if len(tr.HitIndexRef().At(10, 10)) != 1 {
		t.Fatal("expected hit area while visible")
	}
	vis = false
	tr.layoutPass()
	if got := tr.HitIndexRef().At(10, 10); len(got) != 0 {
		t.Fatalf("hidden zone must publish no hit areas, got %+v", got)
	}
}
