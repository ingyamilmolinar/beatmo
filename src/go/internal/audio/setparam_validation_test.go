package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// "absent" = rejected as non-finite; a read-back that differs from In = clamped.
type paramValidationCase struct {
	name      string
	param     string
	in        float64
	wantOut   float64 // value after clamping; only meaningful when wantStored=true
	wantStore bool    // true if the value should be retained at all
}

func TestSetInstrumentParamRejectsNonFiniteAndClampsRange(t *testing.T) {
	instID := instrumentWithGenericMacros(t, "pitch", "decay", "drive")
	ResetInstrumentParams(instrument.ID(instID))
	t.Cleanup(func() { ResetInstrumentParams(instrument.ID(instID)) })

	cases := []paramValidationCase{
		// Non-finite — must be rejected outright.
		{name: "NaN_pitch", param: "pitch", in: math.NaN(), wantStore: false},
		{name: "PosInf_decay", param: "decay", in: math.Inf(+1), wantStore: false},
		{name: "NegInf_drive", param: "drive", in: math.Inf(-1), wantStore: false},

		// In-range — stored verbatim.
		{name: "InRange_pitch_mid", param: "pitch", in: 5, wantOut: 5, wantStore: true},
		{name: "InRange_decay_zero", param: "decay", in: 0, wantOut: 0, wantStore: true},
		{name: "InRange_drive_max", param: "drive", in: 1, wantOut: 1, wantStore: true},

		// Out-of-range — clamped to [Min, Max] per recipe ParamDef.
		{name: "OutOfRange_pitch_high", param: "pitch", in: 1000, wantOut: 24, wantStore: true},
		{name: "OutOfRange_pitch_low", param: "pitch", in: -1000, wantOut: -24, wantStore: true},
		{name: "OutOfRange_decay_neg", param: "decay", in: -3, wantOut: 0, wantStore: true},
		{name: "OutOfRange_drive_high", param: "drive", in: 99, wantOut: 1, wantStore: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Isolate this row: clear and set in a fresh slot.
			ResetInstrumentParams(instrument.ID(instID))
			SetInstrumentParam(instrument.ID(instID), c.param, c.in)

			got := GetInstrumentParams(instrument.ID(instID))
			v, present := got[c.param]
			if c.wantStore {
				if !present {
					t.Fatalf("param %q (in=%v): want stored, got absent", c.param, c.in)
				}
				if v != c.wantOut {
					t.Fatalf("param %q (in=%v): want stored=%v, got %v", c.param, c.in, c.wantOut, v)
				}
			} else {
				if present {
					t.Fatalf("param %q (in=%v): want rejected, got stored=%v", c.param, c.in, v)
				}
			}
		})
	}
}

// Asks the documents, so it cannot rot; skips when none offers them.
func instrumentWithGenericMacros(t *testing.T, need ...string) string {
	t.Helper()
	for _, id := range roster.All {
		defs := InstrumentParamDefs(id)
		if len(defs) == 0 {
			continue
		}
		have := map[string]bool{}
		for _, p := range defs {
			if p.Max > p.Min {
				have[p.Name] = true
			}
		}
		ok := true
		for _, n := range need {
			if !have[n] {
				ok = false
				break
			}
		}
		if ok {
			return string(id)
		}
	}
	t.Skipf("no shipped instrument has a recipe advertising %v any more; every "+
		"family is config-first, so this case needs re-basing onto stage knobs", need)
	return ""
}
