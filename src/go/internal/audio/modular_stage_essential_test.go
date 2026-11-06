package audio

import (
	"fmt"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// modular_stage_essential_test.go pins the contract behind the Synth-tab
// "grey out the disable pill for a stage the sound can't exist without" fix:
// StageDisableSilences reports whether disabling a pipeline stage would leave
// the instrument with NO sound source (it is the sole generator). Only a
// generator gate (osc/kick) can silence, and only when nothing else is audible;
// every processor/shaper stage (filter, env, lfo, drive, …) is always safely
// bypassable.

func TestStageDisableSilences_Matrix(t *testing.T) {
	cases := []struct {
		name   string
		recipe instrument.ID
		param  string
		want   bool
	}{
		// Pure-oscillator voices: the OSC stage IS the whole sound → essential.
		{"osc sole source (FM bass) is essential", roster.BassFm, "osc_enabled", true},
		{"osc sole source (bare modular) is essential", roster.Modular, "osc_enabled", true},

		// OSC layered over a gen-bank slot: disabling OSC leaves the layer → OK.
		{"osc layered over gen-slot (cello) is disableable", roster.Cello, "osc_enabled", false},

		// OSC already off (sound is the gen-bank / bespoke voice) → OK to toggle.
		{"osc off, gen-bank voice (organ) is disableable", roster.OrganChurch, "osc_enabled", false},
		{"osc off, bespoke voice (snare) is disableable", roster.Snare, "osc_enabled", false},

		// Every PROCESSOR stage on the sole-osc FM bass is always bypassable:
		// its off-state is identity, never silence.
		{"fm on FM bass is disableable", roster.BassFm, "fm_enabled", false},
		{"env on FM bass is disableable", roster.BassFm, "env_enabled", false},
		{"filter on FM bass is disableable", roster.BassFm, "filter_enabled", false},
		{"filtenv on FM bass is disableable", roster.BassFm, "filtenv_enabled", false},
		{"lfo on FM bass is disableable", roster.BassFm, "lfo_enabled", false},
		{"burst on FM bass is disableable", roster.BassFm, "burst_enabled", false},
		{"pitchenv on FM bass is disableable", roster.BassFm, "pitchenv_enabled", false},
		{"formant on FM bass is disableable", roster.BassFm, "formant_enabled", false},
		{"drive on FM bass is disableable", roster.BassFm, "drive_enabled", false},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inst := fmt.Sprintf("ess-probe-%d", i)
			_ = InstallInstrumentDocForTest(instrument.ID(inst), c.recipe)
			t.Cleanup(func() { RemoveInstrumentDocForTest(instrument.ID(inst)) })
			if got := StageDisableSilences(instrument.ID(inst), c.param); got != c.want {
				t.Errorf("StageDisableSilences(%q via %s, %q) = %v, want %v",
					inst, c.recipe, c.param, got, c.want)
			}
		})
	}
}

// TestStageDisableSilences_OscOffIsInteractive — once the sole OSC generator is
// already disabled, its pill must become interactive again so the user can turn
// the sound back on. "Essential" describes an ENABLED sole source, never an
// already-off one.
func TestStageDisableSilences_OscOffIsInteractive(t *testing.T) {
	const inst = "ess-osc-off"
	_ = InstallInstrumentDocForTest(inst, roster.BassFm)
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	if !StageDisableSilences(inst, "osc_enabled") {
		t.Fatal("precondition: FM bass OSC must be essential while enabled")
	}
	SetInstrumentParam(inst, "osc_enabled", 0)
	if StageDisableSilences(inst, "osc_enabled") {
		t.Error("OSC pill must be interactive once OSC is already disabled (so it can be re-enabled)")
	}
}

// TestStageDisableSilences_Robustness — empty / unknown args never report
// essential (no pill should ever be locked by accident).
func TestStageDisableSilences_Robustness(t *testing.T) {
	if StageDisableSilences("", "osc_enabled") {
		t.Error("empty instrument id must not be essential")
	}
	const inst = "ess-robust"
	_ = InstallInstrumentDocForTest(inst, roster.BassFm)
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	if StageDisableSilences(inst, "") {
		t.Error("empty enable param must not be essential")
	}
	if StageDisableSilences(inst, "nonsense_enabled") {
		t.Error("unknown enable param must not be essential")
	}
}
