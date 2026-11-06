package audio

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSetInstrumentParamRejectsUnknownNames: a name the engine does not know —
// retired (post_tone_on, gone in Phase C) or beyond a family's slot capacity
// (gen2_kick_h2) — is refused, never stored as a silent dead key that a later
// export would write out (Phase C task C6).
func TestSetInstrumentParamRejectsUnknownNames(t *testing.T) {
	const inst = roster.Kick
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	SetInstrumentParam(inst, "post_tone_on", 0)
	SetInstrumentParam(inst, "gen2_kick_h2", 1)
	got := GetInstrumentParams(inst)
	for _, k := range []string{"post_tone_on", "gen2_kick_h2"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s was stored", k)
		}
	}
	SetInstrumentParam(inst, "gen1_kick_h2", 0.4)
	if GetInstrumentParams(inst)["gen1_kick_h2"] != 0.4 {
		t.Error("a valid knob was refused")
	}
}

func TestValidateRecipeParams(t *testing.T) {
	for _, c := range []struct {
		p    map[string]float64
		want string // substring of the error; "" = valid
	}{
		{map[string]float64{"post_enabled": 1}, "post_enabled"},
		{map[string]float64{"gen2_source": 5}, "gen2_source"},
		{map[string]float64{"gen1_kick_h2": math.NaN()}, "gen1_kick_h2"},
		{map[string]float64{"gen1_kick_h2": 0.4, "gen3_source": 9}, ""},
	} {
		err := ValidateRecipeParams(c.p)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%v rejected: %v", c.p, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%v: err = %v, want one naming %s", c.p, err, c.want)
		}
	}
}
