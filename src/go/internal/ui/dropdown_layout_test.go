package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Ensure category click does not leave an empty spacer row and the popup height is sane.
func TestInstrumentDropdownNoEmptySpacer(t *testing.T) {
	logger := testLogger
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "kick-long-name-1", Name: "Kick Long Name One", Category: "Kick Drums (WAV)", Source: "wav"},
		{ID: "kick-long-name-2", Name: "Kick Long Name Two", Category: "Kick Drums (WAV)", Source: "wav"},
		{ID: "kick-long-name-3", Name: "Kick Long Name Three", Category: "Kick Drums (WAV)", Source: "wav"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 420, 260), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories")
	}
	dv.instCategoryBtns[0].OnClick()
	if dv.instMenuMode != "instruments" {
		t.Fatalf("not in instruments mode after category click")
	}
	if len(dv.instMenuBtns) < 2 {
		t.Fatalf("expected back + at least one option, got %d", len(dv.instMenuBtns))
	}
	if dv.instMenuBtns[1].Text == "" {
		t.Fatalf("first instrument button empty (spacer detected)")
	}
	if dv.instMenuScroll.Visible < 1 {
		t.Fatalf("visible rows should be at least 1")
	}
	if dv.instMenuFullRect.Dy() < dv.rowHeight()*3 { // back + search + 1 row
		t.Fatalf("popup height too small: %d", dv.instMenuFullRect.Dy())
	}
}

// Ensure popup width is sufficient and transport controls remain usable.
func TestTransportNotShrunkByWideLabels(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 800, 260), graph, testLogger)
	dv.Rows[0].Name = "ExtremelyLongRowNameThatStretchesTheLabelColumn"
	dv.recalcButtons()
	if dv.playBtn().Rect().Dx() < 16 || dv.stopBtn().Rect().Dx() < 16 {
		t.Fatalf("transport buttons shrunk: play=%d stop=%d", dv.playBtn().Rect().Dx(), dv.stopBtn().Rect().Dx())
	}
	if dv.instMenuFullRect.Dx() < 200 {
		t.Fatalf("menu width too small: %d", dv.instMenuFullRect.Dx())
	}
}
