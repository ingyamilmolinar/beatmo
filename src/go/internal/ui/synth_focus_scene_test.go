package ui

import "testing"

// TestSynthFocusSceneSelectsIntendedKnob — the crop_synth_focus_* scenes drive
// synthTabFocusKnobSetup, which walks the selected stage's knobs to select the
// one whose ParamDef.Name matches the requested param so the focus graph shows
// that knob's property-native domain. If the name isn't present in the stage,
// the selection silently falls back to the section's main knob (still renders,
// wrong domain). TestSceneCropSubjectsResolveToVisibleRect only checks the
// Subject *bounds*, so it would pass a silent fallback. This pins the actual
// selected knob for each focus scene (mirrors TestSynthChipSceneSelectsIntendedStage).
func TestSynthFocusSceneSelectsIntendedKnob(t *testing.T) {
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
		{"crop_synth_full_note", "amp_decay"},
		{"crop_synth_focus_fm_decay", "fm_op2_decay"},
		{"crop_synth_focus_fm_penv", "fm_pitch_env_amount"},
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
			idx := g.drum.synthSelectedKnobIdx(inst, sec)
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
