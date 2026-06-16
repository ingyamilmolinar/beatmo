//go:build test

package ui

import "testing"

func TestMarquee_FitsIsStatic(t *testing.T) {
	// Text narrower than the area never scrolls, regardless of frame.
	for _, f := range []int{0, 1, 50, 999, 100000} {
		if off := notifMarqueeOffsetX(f, 40, 100, 8); off != 0 {
			t.Fatalf("fitting text must not scroll at frame %d, got %d", f, off)
		}
	}
}

func TestMarquee_StartsAtZero(t *testing.T) {
	if off := notifMarqueeOffsetX(0, 200, 100, 8); off != 0 {
		t.Fatalf("frame 0 should hold at start (offset 0), got %d", off)
	}
}

func TestMarquee_OverflowScrollsLeftBounded(t *testing.T) {
	textW, areaW, gap := 200, 100, 8
	travel := textW - areaW + gap // 108
	sawNeg := false
	minOff := 0
	for f := 0; f < 5000; f++ {
		off := notifMarqueeOffsetX(f, textW, areaW, gap)
		if off > 0 {
			t.Fatalf("offset must be <= 0, got %d at frame %d", off, f)
		}
		if off < -travel {
			t.Fatalf("scrolled past tail: %d < -%d at frame %d", off, travel, f)
		}
		if off < 0 {
			sawNeg = true
		}
		if off < minOff {
			minOff = off
		}
	}
	if !sawNeg {
		t.Fatal("expected leftward scroll for overflowing text")
	}
	if minOff != -travel {
		t.Fatalf("marquee should reach full travel -%d, deepest was %d", travel, minOff)
	}
}

func TestMarquee_Loops(t *testing.T) {
	// The offset must return to 0 (start hold) periodically — i.e. it loops.
	textW, areaW, gap := 300, 90, 8
	first := -1
	second := -1
	zeros := 0
	for f := 1; f < 6000; f++ {
		if notifMarqueeOffsetX(f, textW, areaW, gap) == 0 {
			zeros++
			if first < 0 {
				first = f
			} else if second < 0 && f > first+5 {
				second = f
				break
			}
		}
	}
	if first < 0 || second < 0 {
		t.Fatalf("expected the marquee to return to start at least twice (loop): first=%d second=%d", first, second)
	}
}
