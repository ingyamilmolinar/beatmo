package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Keeps the audio package free of DrumView. Through SetInstrumentForRow so onRowInstrumentChanged fires; mix state
// is untouched.
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
	b.dv.onRowInstrumentChanged(idx, oldID, newID)
}

// A row's settings are never dropped: if the swap would replace any, the user is asked once for the whole swap and
// the kit applies on the answer (0 is returned meanwhile).
func (dv *DrumView) ApplyKit(k audio.Kit) int {
	return dv.applyKitChoosing(k)
}
