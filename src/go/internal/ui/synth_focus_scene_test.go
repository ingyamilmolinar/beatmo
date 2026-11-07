package ui

import "testing"

// The focus scenes select their knob by ParamDef.Name and silently fall back to the main knob on a miss, which the
// bounds test would pass; this pins the selected knob.
func TestSynthFocusSceneSelectsIntendedControl(t *testing.T) {
	cases := []struct{ scene, knob string }{
		{"crop_synth_focus_filter", "filter_cutoff"},
		{"crop_synth_focus_env", "amp_attack"},
		{"crop_synth_focus_post", "drive"},
		{"crop_synth_focus_fm", "fm_op2_depth"},
		{"crop_synth_focus_osc", "osc_type"},
		{"crop_synth_focus_pitch", "osc_octave"},
		{"crop_synth_focus_motion", "lfo_rate"},
		{"crop_synth_focus_burst", "burst1_amp"},
		{"crop_synth_focus_burst_hit", "burst2_amp"},
		{"crop_synth_focus_drive", "drive"},
		{"crop_synth_focus_fm_decay", "gen1_fm_dec2"},
		{"crop_synth_focus_fm_penv", "gen1_fm_pe_amt"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.scene, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			if err := RunScene(g, c.scene); err != nil {
				t.Fatalf("RunScene(%q): %v", c.scene, err)
			}
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}
			inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
			sec := g.drum.synthSelectedSection()
			if sec == nil {
				t.Fatalf("no selected section for %s", c.scene)
			}
			idx := g.drum.synthSelectedControlIdx(inst, sec)
			if idx < 0 || idx >= len(g.drum.instEditorBindings) {
				t.Fatalf("bad selected knob idx %d", idx)
			}
			got := g.drum.instEditorBindings[idx].def.Name
			if got != c.knob {
				t.Errorf("scene %s: selected knob = %q, want %q (FELL BACK to section main knob)", c.scene, got, c.knob)
			}
		})
	}
}
