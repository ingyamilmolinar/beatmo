package ui

import (
	"image"
	"testing"
)

func TestSmoothstepAndLerp(t *testing.T) {
	if smoothstep(0) != 0 || smoothstep(1) != 1 {
		t.Fatal("smoothstep endpoints must be 0 and 1")
	}
	if v := smoothstep(0.5); v <= 0.4 || v >= 0.6 {
		t.Fatalf("smoothstep(0.5) ~0.5, got %v", v)
	}
	if lerpInt(0, 10, 0.5) != 5 {
		t.Fatal("lerpInt midpoint")
	}
	a := image.Rect(0, 0, 10, 10)
	b := image.Rect(10, 10, 30, 30)
	if got := lerpRect(a, b, 0); got != a {
		t.Fatalf("lerpRect t=0 must equal a, got %v", got)
	}
	if got := lerpRect(a, b, 1); got != b {
		t.Fatalf("lerpRect t=1 must equal b, got %v", got)
	}
}

func TestArmGrow_EasesInThenOut(t *testing.T) {
	dv := &DrumView{armGrowIdx: -1}
	dv.setArmGrowSubject(armedScopeSynth, "inst", 3, 1) // arm
	if idx, active := dv.armGrowControl(armedScopeSynth, "inst"); !active || idx != 3 {
		t.Fatalf("armGrowControl want (3,true) got (%d,%v)", idx, active)
	}
	// Ease in: T climbs 0->1 over a bounded number of ticks.
	ticks := 0
	for dv.advanceArmGrow() {
		ticks++
		if ticks > 100 {
			t.Fatal("grow never settled")
		}
	}
	if dv.armGrowT < 0.999 {
		t.Fatalf("after settle armGrowT ~1, got %v", dv.armGrowT)
	}
	// Lock: target 0, subject retained until settled, then cleared.
	dv.setArmGrowSubject(armedScopeSynth, "inst", 3, 0)
	if _, active := dv.armGrowControl(armedScopeSynth, "inst"); !active {
		t.Fatal("subject must be retained through the shrink")
	}
	for dv.advanceArmGrow() {
	}
	if dv.armGrowT > 0.001 {
		t.Fatalf("after shrink armGrowT ~0, got %v", dv.armGrowT)
	}
	if _, active := dv.armGrowControl(armedScopeSynth, "inst"); active {
		t.Fatal("subject must clear once shrink settles")
	}
}

// A fresh/zero DrumView (armGrowIdx default) must not report an active subject
// for any instrument.
func TestArmGrowControl_InertWhenIdle(t *testing.T) {
	dv := &DrumView{armGrowIdx: -1}
	if _, active := dv.armGrowControl(armedScopeSynth, ""); active {
		t.Fatal("idle DrumView must not be active for empty instID")
	}
	if _, active := dv.armGrowControl(armedScopeSynth, "x"); active {
		t.Fatal("idle DrumView must not be active for any instID")
	}
}
