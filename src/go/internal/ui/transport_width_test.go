package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure transport buttons remain usable even when labels widen the rack.
func TestTransportWidthStableWithLongLabels(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 240), graph, testLogger)
	dv.Rows[0].Name = "ExtremelyLongRowNameToStressTransportWidth"
	dv.recalcButtons()
	if dv.playBtn.Rect().Dx() < 24 || dv.stopBtn.Rect().Dx() < 24 {
		t.Fatalf("transport buttons shrunk too small: play=%d stop=%d", dv.playBtn.Rect().Dx(), dv.stopBtn.Rect().Dx())
	}
}
