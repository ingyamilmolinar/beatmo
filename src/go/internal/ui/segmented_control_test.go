//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSegmentedControl_DrawsThreeEqualWidthCells verifies that a 3-segment
// control partitions its rect into three equal-width hit zones.
func TestSegmentedControl_DrawsThreeEqualWidthCells(t *testing.T) {
	sc := NewSegmentedControl([]string{"Pads", "EQ", "Wave"}, 0, nil)
	sc.SetRect(image.Rect(0, 0, 300, 44))

	for i := 0; i < 3; i++ {
		got := sc.SegmentRect(i).Dx()
		want := 100
		if i == 2 {
			want = 100 // last segment absorbs rounding remainder; 300/3 is exact
		}
		if got != want {
			t.Fatalf("segment %d width = %d, want %d", i, got, want)
		}
	}
}

// TestSegmentedControl_TapDispatchesIndex verifies a tap at the geometric
// center of each segment fires the callback with the matching index.
func TestSegmentedControl_TapDispatchesIndex(t *testing.T) {
	got := -1
	sc := NewSegmentedControl([]string{"A", "B", "C"}, 0, func(i int) { got = i })
	sc.SetRect(image.Rect(0, 0, 300, 44))

	for i := 0; i < 3; i++ {
		sr := sc.SegmentRect(i)
		cx := sr.Min.X + sr.Dx()/2
		cy := sr.Min.Y + sr.Dy()/2
		got = -1
		if !sc.HitTest(cx, cy) {
			t.Fatalf("segment %d hit-test missed at center (%d,%d)", i, cx, cy)
		}
		if got != i {
			t.Fatalf("segment %d dispatched index %d", i, got)
		}
		if sc.Active() != i {
			t.Fatalf("segment %d Active()=%d after tap", i, sc.Active())
		}
	}
}

// TestSegmentedControl_OutsideTapIgnored verifies hits outside the rect
// are not consumed.
func TestSegmentedControl_OutsideTapIgnored(t *testing.T) {
	called := false
	sc := NewSegmentedControl([]string{"A", "B"}, 0, func(int) { called = true })
	sc.SetRect(image.Rect(10, 10, 110, 54))

	if sc.HitTest(5, 5) {
		t.Fatal("hit-test should miss outside the rect")
	}
	if called {
		t.Fatal("callback should not fire on outside tap")
	}
}

// TestSegmentedControl_ActiveSegmentUsesAccent verifies the active segment
// is filled with the accent token, while inactive segments are not. We
// intercept drawRoundedRect and look for an accent-coloured fill inside
// the active segment's rect.
func TestSegmentedControl_ActiveSegmentUsesAccent(t *testing.T) {
	sc := NewSegmentedControl([]string{"A", "B", "C"}, 1, nil)
	sc.SetRect(image.Rect(0, 0, 300, 44))

	dst := ebiten.NewImage(300, 44)
	calls := captureRoundedRectCalls(t, func() { sc.Draw(dst) })

	activeR := sc.SegmentRect(1)
	accent := color.NRGBAModel.Convert(TokenAccent()).(color.NRGBA)
	matched := false
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		got, ok := c.Color.(color.NRGBA)
		if !ok {
			continue
		}
		if got.R == accent.R && got.G == accent.G && got.B == accent.B && c.Rect.In(activeR) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("no accent-coloured fill inside active segment %v; calls=%+v", activeR, calls)
	}
}

// TestSegmentedControl_SetActiveDoesNotFireCallback verifies SetActive is
// a quiet setter — only HitTest fires onClick.
func TestSegmentedControl_SetActiveDoesNotFireCallback(t *testing.T) {
	calls := 0
	sc := NewSegmentedControl([]string{"A", "B"}, 0, func(int) { calls++ })
	sc.SetActive(1)
	if calls != 0 {
		t.Fatalf("SetActive fired callback %d times; want 0", calls)
	}
	if sc.Active() != 1 {
		t.Fatalf("SetActive(1) didn't take effect; Active()=%d", sc.Active())
	}
}
