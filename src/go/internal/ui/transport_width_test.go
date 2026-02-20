package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure transport buttons remain usable even when labels widen the rack.
func TestTransportWidthStableWithLongLabels(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	// Use 800px width — realistic minimum for desktop.
	dv := NewDrumView(image.Rect(0, 0, 800, 240), graph, testLogger)
	dv.Rows[0].Name = "ExtremelyLongRowNameToStressTransportWidth"
	dv.recalcButtons()
	if dv.playBtn.Rect().Dx() < 16 || dv.stopBtn.Rect().Dx() < 16 {
		t.Fatalf("transport buttons shrunk too small: play=%d stop=%d", dv.playBtn.Rect().Dx(), dv.stopBtn.Rect().Dx())
	}
}
