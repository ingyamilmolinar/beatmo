package audio

import "testing"

var synthParamSupportedIDs = []string{"snare", "kick", "hihat", "clap", "tom", "cowbell"}

// Synth-tab redesign (2026-05-16) dropped attack + color; the API now
// returns the 6-knob superset matching synth_params.h.
var synthParamExpectedNames = []string{
	"pitch", "decay", "tone", "drive", "body", "brightness",
}

func TestSynthParamDefsSupportedInstruments(t *testing.T) {
	for _, id := range synthParamSupportedIDs {
		params := SynthParamDefs(id)
		if len(params) != 6 {
			t.Errorf("SynthParamDefs(%q) length=%d, want 6", id, len(params))
			continue
		}
		for i, name := range synthParamExpectedNames {
			if params[i].Name != name {
				t.Errorf("SynthParamDefs(%q)[%d].Name=%q, want %q", id, i, params[i].Name, name)
			}
		}
	}
}

func TestSynthParamDefsRangesValid(t *testing.T) {
	// Every supported instrument's params must have Min < Max.
	// (Default is intentionally allowed to sit outside [Min, Max] —
	// 0 is the synth's "use built-in default" sentinel for decay/attack.)
	for _, id := range synthParamSupportedIDs {
		for i, p := range SynthParamDefs(id) {
			if p.Min >= p.Max {
				t.Errorf("SynthParamDefs(%q)[%d=%s] Min=%v >= Max=%v",
					id, i, p.Name, p.Min, p.Max)
			}
		}
	}
}

func TestSynthParamDefsUnknownReturnsNil(t *testing.T) {
	for _, id := range []string{"", "unknown", "kicker", "808", "Snare" /* case-sensitive */} {
		if got := SynthParamDefs(id); got != nil {
			t.Errorf("SynthParamDefs(%q)=%v, want nil", id, got)
		}
	}
}

func TestHasSynthParamsMatchesDefs(t *testing.T) {
	for _, id := range synthParamSupportedIDs {
		if !HasSynthParams(id) {
			t.Errorf("HasSynthParams(%q)=false, want true", id)
		}
	}
	for _, id := range []string{"", "unknown", "Kick" /* wrong case */} {
		if HasSynthParams(id) {
			t.Errorf("HasSynthParams(%q)=true, want false", id)
		}
	}
}

// TestSynthParamDefsPitchUnit guards against accidentally dropping the
// semitone unit on the pitch param — it's the only param with a Unit
// label and the FX panel relies on it for axis rendering.
func TestSynthParamDefsPitchUnit(t *testing.T) {
	defs := SynthParamDefs("kick")
	if len(defs) == 0 || defs[0].Name != "pitch" {
		t.Fatalf("expected first param to be pitch, got %#v", defs)
	}
	if defs[0].Unit != "st" {
		t.Errorf("pitch.Unit=%q, want %q", defs[0].Unit, "st")
	}
}
