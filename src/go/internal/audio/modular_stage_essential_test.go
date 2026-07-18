package audio

import (
	"fmt"
	"testing"
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
		recipe string
		param  string
		want   bool
	}{
		// Pure-oscillator voices: the OSC stage IS the whole sound → essential.
		{"osc sole source (FM bass) is essential", "synth-modular-bass-fm", "osc_enabled", true},
		{"osc sole source (bare modular) is essential", "synth-modular", "osc_enabled", true},

		// OSC layered over a gen-bank slot: disabling OSC leaves the layer → OK.
		{"osc layered over gen-slot (cello) is disableable", "synth-modular-cello", "osc_enabled", false},

		// OSC already off (sound is the gen-bank / bespoke voice) → OK to toggle.
		{"osc off, gen-bank voice (organ) is disableable", "synth-modular-organ-church", "osc_enabled", false},
		{"osc off, bespoke voice (snare) is disableable", "drum-snare", "osc_enabled", false},

		// Every PROCESSOR stage on the sole-osc FM bass is always bypassable:
		// its off-state is identity, never silence.
		{"fm on FM bass is disableable", "synth-modular-bass-fm", "fm_enabled", false},
		{"env on FM bass is disableable", "synth-modular-bass-fm", "env_enabled", false},
		{"filter on FM bass is disableable", "synth-modular-bass-fm", "filter_enabled", false},
		{"filtenv on FM bass is disableable", "synth-modular-bass-fm", "filtenv_enabled", false},
		{"lfo on FM bass is disableable", "synth-modular-bass-fm", "lfo_enabled", false},
		{"burst on FM bass is disableable", "synth-modular-bass-fm", "burst_enabled", false},
		{"pitchenv on FM bass is disableable", "synth-modular-bass-fm", "pitchenv_enabled", false},
		{"formant on FM bass is disableable", "synth-modular-bass-fm", "formant_enabled", false},
		{"drive on FM bass is disableable", "synth-modular-bass-fm", "drive_enabled", false},
		{"post on FM bass is disableable", "synth-modular-bass-fm", "post_enabled", false},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inst := fmt.Sprintf("ess-probe-%d", i)
			BindInstrumentToRecipe(inst, c.recipe)
			t.Cleanup(func() { ResetInstrumentParams(inst) })
			if got := StageDisableSilences(inst, c.param); got != c.want {
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
	BindInstrumentToRecipe(inst, "synth-modular-bass-fm")
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
	BindInstrumentToRecipe(inst, "synth-modular-bass-fm")
	t.Cleanup(func() { ResetInstrumentParams(inst) })
	if StageDisableSilences(inst, "") {
		t.Error("empty enable param must not be essential")
	}
	if StageDisableSilences(inst, "nonsense_enabled") {
		t.Error("unknown enable param must not be essential")
	}
}
