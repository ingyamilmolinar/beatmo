package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// kitRowBinder adapts *DrumView to the audio.KitRowBinder interface so
// audio.ApplyKit can rebind row instruments without the audio package
// learning about DrumView. The binder defers to SetInstrumentForRow so
// the existing onRowInstrumentChanged sync notifier fires — kit swaps
// flow through the same EQ-panel update path as a single SetInstrument
// call. Mix state (volume/pan/sends/EQ/effects) is intentionally
// untouched.
type kitRowBinder struct{ dv *DrumView }

func (b kitRowBinder) RowCount() int {
	if b.dv == nil {
		return 0
	}
	return len(b.dv.Rows)
}

func (b kitRowBinder) RowInstrument(idx int) instrument.ID {
	if b.dv == nil || idx < 0 || idx >= len(b.dv.Rows) || b.dv.Rows[idx] == nil {
		return ""
	}
	return b.dv.Rows[idx].Instrument
}

func (b kitRowBinder) RowRole(idx int) instrument.Role {
	if b.dv == nil || idx < 0 || idx >= len(b.dv.Rows) || b.dv.Rows[idx] == nil {
		return instrument.RoleNone
	}
	return b.dv.Rows[idx].Role
}

func (b kitRowBinder) SetRowInstrument(idx int, newID instrument.ID) {
	if b.dv == nil || idx < 0 || idx >= len(b.dv.Rows) || b.dv.Rows[idx] == nil {
		return
	}
	row := b.dv.Rows[idx]
	oldID := row.Instrument
	row.Instrument = newID
	// Trigger the existing instrument-change notification so the EQ
	// panel, sticky bar, and event narrative all observe the swap.
	b.dv.onRowInstrumentChanged(idx, oldID, newID)
}

// ApplyKitToDrumView is the UI-facing entry point. Wraps the DrumView
// in the binder and delegates to audio.ApplyKit. Returns the rebound
// row count for caller telemetry; the EventKitApplied hook fires from
// inside audio.ApplyKit regardless.
//
// A row's settings are never dropped: when the swap would replace any row's
// settings with a kit instrument's own, the user is asked once for the whole
// swap (applyKitChoosing) and the kit applies on the answer (0 is returned
// meanwhile).
func (dv *DrumView) ApplyKit(k audio.Kit) int {
	return dv.applyKitChoosing(k)
}
