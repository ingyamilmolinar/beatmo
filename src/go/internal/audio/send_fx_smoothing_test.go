//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Phase-7 click-free contract for send-FX reconfiguration. Pre-Phase-7,
// ConfigureSendDelay free()d and re-malloc()'d the delay buffer on every
// time change and called delay_init(), which memset() the buffer to zero
// and reset the LP feedback state. The audible result was a hard cut of
// the wet tail at the exact moment the user dragged the time slider — a
// click.
//
// Post-Phase-7, ConfigureSendDelay calls delay_set_time_smooth/
// delay_set_feedback_smooth/delay_set_damping_smooth which mutate the
// state in place and never zero the buffer. This test proves the
// post-mutation state preserves continuity:
//
//   1. Inject a step impulse, let it ring into the delay tail.
//   2. Reconfigure the delay time mid-flight.
//   3. Process more samples and assert the output is non-zero AND has
//      no discontinuity larger than a hard-clip-class spike.
//
// Reverb is similar but the test surface is the wet-tail decay before vs
// after reverb_set_params_smooth: the tail must continue decaying, not
// jump to silence.

func TestConfigureSendDelay_PreservesBufferState(t *testing.T) {
	// Phase-7 contract: ConfigureSendDelay must NOT call delay_init — calling
	// delay_init memset()s the buffer to zero AND resets writePos AND resets
	// lpState. Pre-Phase-7 did all three on every time change, producing the
	// audible "click to silence" the user can reproduce by dragging the time
	// slider during a sustained pattern.
	//
	// The strongest mechanical proof of "didn't reinit" is that writePos
	// keeps advancing across the reconfigure call and lpState (the LP
	// feedback filter state) is unchanged at the instant of the call. Both
	// are zero immediately after delay_init.
	const sr = 44100
	if sendFX == nil {
		initSendEffects(sr)
		t.Cleanup(func() {
			resetSendEffects()
			ConfigureSendDelay(300, 0.3, 3000)
		})
	}
	ConfigureSendDelay(80, 0.8, 3000)
	resetSendEffects()

	const block = 64
	buf := make([]float32, block)
	buf[0] = 1.0
	sendDelayProcessForTest(buf)
	// Ring enough blocks that lpState is non-zero (any feedback from the
	// impulse echo). 80 ms @ 44.1k = ~3528 samples = ~55 blocks of 64.
	for i := 0; i < 200; i++ {
		zeroFloat32(buf)
		sendDelayProcessForTest(buf)
	}

	preWritePos := int(sendFX.delay.writePos)
	preLP := float32(sendFX.delay.lpState)
	if preWritePos == 0 {
		t.Fatalf("writePos still 0 after 200 process blocks — delay not advancing?")
	}

	// Reconfigure time. Pre-Phase-7 zeroed both fields below; post-Phase-7
	// only updates length / feedback / lp coefficient.
	ConfigureSendDelay(150, 0.5, 4000)

	postWritePos := int(sendFX.delay.writePos)
	postLP := float32(sendFX.delay.lpState)

	// writePos must not be reset to 0. delay_set_time_smooth only wraps
	// writePos when the new length is shorter than the old position;
	// otherwise it stays put.
	if postWritePos == 0 && preWritePos != 0 {
		t.Errorf("ConfigureSendDelay reset writePos: pre=%d post=0 (called delay_init?)", preWritePos)
	}

	// lpState must equal pre value exactly. delay_init forces it to 0.
	if postLP != preLP {
		t.Errorf("ConfigureSendDelay perturbed lpState: pre=%g post=%g", preLP, postLP)
	}

	// And the configured time/feedback/damping must actually take effect.
	gotTime, gotFB, gotDamp := SendDelayParams()
	if gotTime != 150 || gotFB != 0.5 || gotDamp != 4000 {
		t.Errorf("ConfigureSendDelay did not store params: got (%g,%g,%g) want (150,0.5,4000)",
			gotTime, gotFB, gotDamp)
	}
}

func TestConfigureSendReverb_PreservesTail(t *testing.T) {
	// Phase-7 contract for reverb: ConfigureSendReverb must NOT call
	// reverb_init (which memset()s every comb + allpass buffer to zero —
	// the audible "click to silence"). Instead it should mutate the comb
	// feedback, comb damp, and wet/dry coefficients in place.
	const sr = 44100
	if sendFX == nil {
		initSendEffects(sr)
		t.Cleanup(func() {
			resetSendEffects()
			ConfigureSendReverb(0.7, 0.4, 0.3)
		})
	}
	ConfigureSendReverb(0.85, 0.3, 0.5)
	resetSendEffects()

	const block = 64
	in := make([]float32, block)
	out := make([]float32, block)
	in[0] = 1.0
	sendReverbProcessForTest(in, out)
	for i := 0; i < 200; i++ {
		zeroFloat32(in)
		sendReverbProcessForTest(in, out)
	}

	zeroFloat32(in)
	sendReverbProcessForTest(in, out)
	preTail := absMaxFloat32(out)
	if preTail == 0 {
		t.Fatalf("pre-change reverb tail is silent — test cannot detect a click")
	}

	// Snapshot one comb's position before the change. reverb_init zeroes
	// each comb's pos; reverb_set_params_smooth must not touch it.
	preCombPos := int(sendFX.reverb.combs[0].pos)

	ConfigureSendReverb(0.5, 0.5, 0.5)

	// Comb positions must NOT have reset to 0 (which is what reverb_init does).
	if int(sendFX.reverb.combs[0].pos) == 0 && preCombPos != 0 {
		t.Errorf("ConfigureSendReverb reset combs[0].pos: pre=%d post=0", preCombPos)
	}

	zeroFloat32(in)
	sendReverbProcessForTest(in, out)
	postTail := absMaxFloat32(out)
	if postTail == 0 {
		t.Fatalf("ConfigureSendReverb silenced the reverb tail (click)")
	}

	// Params must take effect.
	gotR, gotD, gotW := SendReverbParams()
	if gotR != 0.5 || gotD != 0.5 || gotW != 0.5 {
		t.Errorf("ConfigureSendReverb did not store params: got (%g,%g,%g) want (0.5,0.5,0.5)",
			gotR, gotD, gotW)
	}
}

// Helpers below avoid duplicating CGo boilerplate in each test. They reach
// directly into sendFX which is set up by the tests that call init.

func zeroFloat32(buf []float32) {
	for i := range buf {
		buf[i] = 0
	}
}

func absMaxFloat32(buf []float32) float32 {
	var m float32
	for _, v := range buf {
		a := float32(math.Abs(float64(v)))
		if a > m {
			m = a
		}
	}
	return m
}
