package audio

import (
	"math"
	"testing"
)

// Phase 11 contract: SetSendDelayParam / SetSendReverbParam dispatch through
// the Phase-4 sanitize boundary (NaN/Inf rejected, out-of-range clamped) and
// then call the existing ConfigureSendDelay/ConfigureSendReverb. Tests run
// under -tags test so they reuse stub.SendDelayParams/SendReverbParams
// instead of touching real C audio state.

func TestSetSendDelayParam_RoutesAndSanitizes(t *testing.T) {
	// Native build requires sendFX to be initialized so ConfigureSendDelay
	// is not an early-return no-op. The test-tag stub always succeeds.
	ensureSendFXForTest(t)
	// Reset to defaults.
	ConfigureSendDelay(300, 0.3, 3000)

	// Normal in-range update.
	if v, ok := SetSendDelayParam("delay.time_ms", 450); !ok || v != 450 {
		t.Fatalf("delay.time_ms=450: got (%v,%v); want (450,true)", v, ok)
	}
	gotT, gotF, gotD := SendDelayParams()
	if gotT != 450 || gotF != 0.3 || gotD != 3000 {
		t.Errorf("after delay.time_ms=450: got params (%g,%g,%g); want (450,0.3,3000)", gotT, gotF, gotD)
	}

	// Clamp high → ceiling.
	if v, ok := SetSendDelayParam("delay.feedback", 99); !ok || v != 1 {
		t.Errorf("delay.feedback=99: got (%v,%v); want (1,true)", v, ok)
	}
	// Clamp low → floor.
	if v, ok := SetSendDelayParam("delay.damping_hz", -100); !ok || v != 100 {
		t.Errorf("delay.damping_hz=-100: got (%v,%v); want (100,true)", v, ok)
	}

	// Non-finite rejected (no mutation). Read state via the public getter so
	// the test works in both build paths (test stub + native CGo).
	preT, preF, preD := SendDelayParams()
	if v, ok := SetSendDelayParam("delay.time_ms", math.NaN()); ok || v != 0 {
		t.Errorf("delay.time_ms=NaN: got (%v,%v); want (0,false)", v, ok)
	}
	postT, postF, postD := SendDelayParams()
	if preT != postT || preF != postF || preD != postD {
		t.Errorf("NaN should not mutate params; pre=(%g,%g,%g) post=(%g,%g,%g)",
			preT, preF, preD, postT, postF, postD)
	}

	// Unknown param rejected.
	if v, ok := SetSendDelayParam("delay.bogus", 0.5); ok || v != 0 {
		t.Errorf("delay.bogus: got (%v,%v); want (0,false)", v, ok)
	}
}

func TestSetSendReverbParam_RoutesAndSanitizes(t *testing.T) {
	ensureSendFXForTest(t)
	ConfigureSendReverb(0.7, 0.4, 0.3)

	if v, ok := SetSendReverbParam("reverb.room", 0.85); !ok || v != 0.85 {
		t.Fatalf("reverb.room=0.85: got (%v,%v); want (0.85,true)", v, ok)
	}
	gotR, gotD, gotW := SendReverbParams()
	if gotR != 0.85 || gotD != 0.4 || gotW != 0.3 {
		t.Errorf("after reverb.room=0.85: got (%g,%g,%g); want (0.85,0.4,0.3)", gotR, gotD, gotW)
	}

	if v, ok := SetSendReverbParam("reverb.wet", 99); !ok || v != 1 {
		t.Errorf("reverb.wet=99: got (%v,%v); want (1,true)", v, ok)
	}

	if v, ok := SetSendReverbParam("reverb.damping", math.Inf(+1)); ok || v != 0 {
		t.Errorf("reverb.damping=+Inf: got (%v,%v); want (0,false)", v, ok)
	}
}

func TestSendFXParamSchemaCoverage(t *testing.T) {
	names := SendFXParamNames()
	if len(names) == 0 {
		t.Fatal("SendFXParamNames returned empty list")
	}
	for _, n := range names {
		def := SendFXParamDef(n)
		if def.Name != n {
			t.Errorf("SendFXParamDef(%q): got name=%q", n, def.Name)
		}
		if def.Min > def.Max {
			t.Errorf("def %q: Min %v > Max %v", n, def.Min, def.Max)
		}
		if def.Default < def.Min || def.Default > def.Max {
			t.Errorf("def %q: Default %v outside [%v,%v]", n, def.Default, def.Min, def.Max)
		}
	}
}
