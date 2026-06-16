package ui

import "testing"

func TestDiscreteKnobDrawsDetentTicks(t *testing.T) {
	k := newDiscreteKnob([]string{"a", "b", "c", "d", "e"}) // 5 detents; helper in knob_discrete_test.go
	var calls int
	restore := swapDetentTickDrawerForTest(func(_ float64) { calls++ })
	defer restore()
	img := newTrackedImage("test.knob", 100, 100)
	defer releaseImage(img)
	k.Draw(img)
	if calls != 5 {
		t.Fatalf("detent ticks drawn=%d want 5", calls)
	}
}
