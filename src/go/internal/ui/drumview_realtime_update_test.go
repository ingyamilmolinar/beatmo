package ui

import (
	"image"
	"image/color"
	"io"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Regression: changing a row's instrument (which also changes its color) during
// playback must invalidate the row cache fully so the new color shows across the
// visible window immediately (not only in the newly revealed strip on the right).
func TestDrumView_InstrumentChangeDuringShiftRebuildsRowCache(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "kick", Name: "Kick", Category: "Drums"},
		{ID: "snare", Name: "Snare", Category: "Drums"},
	})

	dv := NewDrumView(image.Rect(0, 0, 480, 240), nil, logger)
	dv.refreshInstruments()
	dv.recalcButtons()
	dv.calcLayout()

	// Make 1px-per-step so a +1 offset produces a small shift eligible for
	// incremental reuse (the bug path).
	dv.SetLength(dv.timelineRect.Dx())
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
		r.CellTypes = make([]model.NodeType, dv.Length)
		for i := range r.Steps {
			r.Steps[i] = true
			r.CellTypes[i] = model.NodeTypeRegular
		}
	}
	dv.SetRowColor(0, color.RGBA{255, 0, 0, 255})

	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.Draw(dst, nil, 0, nil, 0)
	if len(dv.rowCacheGen) == 0 {
		t.Fatalf("expected row cache generation tracking")
	}
	gen0 := dv.rowCacheGen[0]

	// Simulate playback scroll in the same frame as an instrument change.
	dv.Offset = dv.Offset + 1
	dv.markRowsShiftDirty()

	// Switch to a different built-in instrument (avoid registering custom
	// instruments which would leak into other tests via the global catalog).
	nextID := ""
	for _, id := range dv.instOptions {
		if id != "" && id != dv.Rows[0].Instrument {
			nextID = id
			break
		}
	}
	if nextID == "" {
		t.Fatalf("no alternate instrument available")
	}
	dv.SetInstrument(nextID)

	dv.Draw(dst, nil, 0, nil, 0)
	if dv.rowCacheGen[0] == gen0 {
		t.Fatalf("row cache reused sprite after instrument change during shift; gen=%d", gen0)
	}
}
