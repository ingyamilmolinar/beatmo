package audio

import "testing"

// TestPostStageHasNoGatesOrderOrEnable: the post ops are always on and each is a
// no-op at its identity, so the per-op gates, the op-order switch and
// post_enabled are gone; the one thing post_enabled really did — skip the ±1.5
// output clamp — is now the explicit out_clamp knob (Phase C task C4).
func TestPostStageHasNoGatesOrderOrEnable(t *testing.T) {
	idx := ModularParamSchemaIndexMap()
	for _, gone := range []string{"post_enabled", "post_order", "post_pitch_on", "post_decay_on",
		"post_body_on", "post_brightness_on", "post_tone_on", "post_drive_on"} {
		if _, ok := idx[gone]; ok {
			t.Errorf("%s still in the param block", gone)
		}
	}
	if _, ok := idx["out_clamp"]; !ok {
		t.Fatal("out_clamp missing from the param block")
	}
	ident := modularFieldInits()
	if ident["out_clamp"] != 1.5 {
		t.Errorf("out_clamp identity = %v, want 1.5 (the clamp every non-post instrument had)", ident["out_clamp"])
	}
	if ident["post_decay_rate"] <= 0 {
		t.Errorf("post_decay_rate identity = %v: Fade would be a dead knob on every instrument that does not set a rate", ident["post_decay_rate"])
	}
}
