package scope

import "testing"

func TestActiveSpan(t *testing.T) {
	// Helper: build a buffer of length n with a transient (value 1.0) at
	// indices [from,to).
	build := func(n, from, to int) []float64 {
		s := make([]float64, n)
		for i := from; i < to && i < n; i++ {
			s[i] = 1.0
		}
		return s
	}

	t.Run("empty returns zero range", func(t *testing.T) {
		if s, e := ActiveSpan(nil, 0.05, 0.2); s != 0 || e != 0 {
			t.Errorf("ActiveSpan(nil)=%d,%d want 0,0", s, e)
		}
	})

	t.Run("silent returns full range", func(t *testing.T) {
		buf := make([]float64, 100)
		if s, e := ActiveSpan(buf, 0.05, 0.2); s != 0 || e != 100 {
			t.Errorf("silent ActiveSpan=%d,%d want 0,100", s, e)
		}
	})

	t.Run("centered transient is tightly framed", func(t *testing.T) {
		// 1000-sample buffer, transient at [500,510). No padding.
		buf := build(1000, 500, 510)
		s, e := ActiveSpan(buf, 0.5, 0.0)
		if s != 500 || e != 510 {
			t.Errorf("ActiveSpan=%d,%d want 500,510", s, e)
		}
		// Framed span must be far smaller than the full buffer.
		if e-s >= len(buf)/2 {
			t.Errorf("framed span %d not tight vs buffer %d", e-s, len(buf))
		}
	})

	t.Run("padding expands the span and clamps to bounds", func(t *testing.T) {
		buf := build(1000, 500, 510) // span 10
		s, e := ActiveSpan(buf, 0.5, 1.0) // pad = 1.0 * 10 = 10 each side
		if s != 490 || e != 520 {
			t.Errorf("padded ActiveSpan=%d,%d want 490,520", s, e)
		}
		// Transient near the front: padding must clamp to 0.
		front := build(1000, 0, 5)
		s2, _ := ActiveSpan(front, 0.5, 1.0)
		if s2 != 0 {
			t.Errorf("front-clamped start=%d want 0", s2)
		}
		// Transient near the back: padding must clamp to len.
		back := build(1000, 995, 1000)
		_, e2 := ActiveSpan(back, 0.5, 1.0)
		if e2 != 1000 {
			t.Errorf("back-clamped end=%d want 1000", e2)
		}
	})

	t.Run("threshold gates which samples count", func(t *testing.T) {
		// Peak 1.0 at index 50; a tiny 0.01 blip at index 10 must be ignored
		// at a 0.05 threshold (0.05*1.0 = 0.05 > 0.01).
		buf := make([]float64, 100)
		buf[10] = 0.01
		buf[50] = 1.0
		s, e := ActiveSpan(buf, 0.05, 0.0)
		if s != 50 || e != 51 {
			t.Errorf("thresholded ActiveSpan=%d,%d want 50,51", s, e)
		}
	})
}
