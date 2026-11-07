package ui

import (
	"image"
	"testing"
)

func newDiscreteKnob(enum []string) *Knob {
	k := NewKnob(0)
	k.SetRect(image.Rect(0, 0, 100, 100))
	k.Scale = KnobScale{Min: 0, Max: float64(len(enum) - 1), Enum: enum}
	k.Discrete = true
	return k
}

func TestDiscreteKnobDetentCount(t *testing.T) {
	k := NewKnob(0)
	k.Scale = KnobScale{Min: -2, Max: 2, Step: 1} // 5 detents
	k.Discrete = true
	if got := k.detentCount(); got != 5 {
		t.Fatalf("detentCount=%d want 5", got)
	}
	k2 := newDiscreteKnob([]string{"a", "b", "c"})
	if got := k2.detentCount(); got != 3 {
		t.Fatalf("enum detentCount=%d want 3", got)
	}
}

func nearAnyDetent(v float64, n int) bool {
	for i := 0; i < n; i++ {
		d := float64(i) / float64(n-1)
		if v-d < 1e-9 && d-v < 1e-9 {
			return true
		}
	}
	return false
}
