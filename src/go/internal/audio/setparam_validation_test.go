package audio

import (
	"math"
	"testing"
)

// Phase-1 safety net for the upcoming unified SetParam boundary (Phase 4).
//
// State today: every external param setter accepts any float64. NaN, +Inf,
// -Inf, and out-of-range values all flow through to the manager and the
// platform callback. The decay=0 → NaN bug fixed via an ad-hoc C-side clamp
// (drums.c:1736-1745) is the canonical symptom; the actual fix belongs at
// the API boundary so every setter inherits the same guarantee.
//
// Target after Phase 4: each row of the table below must pass — non-finite
// values are dropped, in-range values are stored verbatim, out-of-range
// values are clamped to [Min, Max]. The test is named ...Wanted_AfterPhase4
// so a failure here on the current branch is expected and the failing rows
// describe the post-fix contract.

// paramValidationCase describes one mutation we feed through SetInstrumentParam
// and the value we expect GetInstrumentParams to read back. "absent" means the
// key should NOT appear in the stored map (because the input was rejected as
// non-finite); a present key whose value differs from In means the input was
// clamped.
type paramValidationCase struct {
	name      string
	param     string
	in        float64
	wantOut   float64 // value after clamping; only meaningful when wantStored=true
	wantStore bool    // true if the value should be retained at all
}

func TestSetInstrumentParamRejectsNonFiniteAndClampsRange(t *testing.T) {
	// Reset state so prior tests can't contaminate this one. ResetInstrumentParams
	// is the public clear; we call it both at start and via t.Cleanup so a
	// failure mid-table doesn't leak.
	const instID = "kick" // bound to "drum-kick" which advertises pitch [-24,24], decay [0,4], drive [0,1], body [0,1]
	ResetInstrumentParams(instID)
	t.Cleanup(func() { ResetInstrumentParams(instID) })

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
			ResetInstrumentParams(instID)
			SetInstrumentParam(instID, c.param, c.in)

			got := GetInstrumentParams(instID)
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
