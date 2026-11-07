package audio

import (
	"encoding/json"
	"slices"
	"testing"
)

// What data may name and what can actually be built are the same set.
func TestEveryEffectTypeIsRegistered(t *testing.T) {
	reg := EffectRegistrations()
	for _, et := range EffectTypes {
		if _, ok := reg[et]; !ok {
			t.Errorf("effect type %q has no registration", et)
		}
	}
	for et := range reg {
		if !slices.Contains(EffectTypes, et) {
			t.Errorf("registered effect %q is missing from EffectTypes", et)
		}
	}
}

func TestEffectTypeDecodesStrictly(t *testing.T) {
	var s EffectSlot
	if err := json.Unmarshal([]byte(`{"type":"chorus","enabled":true}`), &s); err != nil || s.Type != EffectChorus {
		t.Fatalf("decode = %+v, %v", s, err)
	}
	for _, bad := range []string{"Chorus", "fuzz", ""} {
		if err := json.Unmarshal([]byte(`{"type":"`+bad+`"}`), &s); err == nil {
			t.Errorf("effect type %q decoded", bad)
		}
	}
}
