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

// boundsRecordingLayer records the Bounds() of the dst image it was drawn into.
type boundsRecordingLayer struct {
	id      string
	z       int
	clip    bool
	gotRect *image.Rectangle
}

func (l *boundsRecordingLayer) ID() string             { return l.id }
func (l *boundsRecordingLayer) ZIndex() int            { return l.z }
func (l *boundsRecordingLayer) Visible() bool          { return true }
func (l *boundsRecordingLayer) ClipToBounds() bool     { return l.clip }
func (l *boundsRecordingLayer) Draw(dst *ebiten.Image) { *l.gotRect = dst.Bounds() }

func TestGridTreeClipsBoundsOptInLayers(t *testing.T) {
	tr := NewGridTree()
	screen := ebiten.NewImage(200, 200)
	bounds := image.Rect(0, 0, 200, 100) // grid pane = top half
	tr.SetBounds(bounds)

	var clipped, unclipped image.Rectangle
	tr.RegisterLayer(&boundsRecordingLayer{id: "content", z: 10, clip: true, gotRect: &clipped})
	tr.RegisterLayer(&boundsRecordingLayer{id: "overlay", z: 20, clip: false, gotRect: &unclipped})

	tr.Draw(screen)

	if clipped != bounds {
		t.Errorf("clip-opt-in layer should receive bounds-clipped image: got %v want %v", clipped, bounds)
	}
	if unclipped != screen.Bounds() {
		t.Errorf("clip-opt-out layer should receive full screen: got %v want %v", unclipped, screen.Bounds())
	}
}
