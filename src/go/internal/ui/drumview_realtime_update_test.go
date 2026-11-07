package ui

import (
	"image"
	"image/color"
	"io"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A mid-playback instrument change (and so colour) must fully invalidate the row cache, not just the new strip.
func TestDrumView_InstrumentChangeDuringShiftRebuildsRowCache(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: roster.Kick, Name: "Kick", Group: "Drums"},
		{ID: roster.Snare, Name: "Snare", Group: "Drums"},
	})

	dv := NewDrumView(image.Rect(0, 0, 480, 240), nil, logger)
	dv.refreshInstruments()
	dv.recalcButtons()
	dv.calcLayout()

	// 1 px per step, so a +1 offset is a small shift eligible for incremental reuse.
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

	// A built-in: a custom one would leak into other tests through the global catalog.
	nextID := ""
	for _, id := range dv.instOptions {
		if id != "" && id != dv.Rows[0].Instrument {
			nextID = string(id)
			break
		}
	}
	if nextID == "" {
		t.Fatalf("no alternate instrument available")
	}
	dv.SetInstrument(instrument.ID(nextID))

	dv.Draw(dst, nil, 0, nil, 0)
	if dv.rowCacheGen[0] == gen0 {
		t.Fatalf("row cache reused sprite after instrument change during shift; gen=%d", gen0)
	}
}
