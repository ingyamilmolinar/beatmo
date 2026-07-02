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
// renders as a lit sunset-gold keycap — its cap face is drawn via the cached
// rounded-button primitive (drawRoundedButton, the same path drawPillTabAt
// uses for the desktop audio tabs) with the accent (#FFB30A) fill, located
// inside the active segment. Inactive segments must NOT carry an accent fill.
func TestSegmentedControl_ActiveSegmentUsesAccent(t *testing.T) {
	sc := NewSegmentedControl([]string{"A", "B", "C"}, 1, nil)
	sc.SetRect(image.Rect(0, 0, 300, 44))

	dst := ebiten.NewImage(300, 44)
	var rec drawCallRecorder
	rec.record(t, func() { sc.Draw(dst) })

	accent := color.RGBAModel.Convert(TokenAccent()).(color.RGBA)
	accentIn := func(seg image.Rectangle) bool {
		for _, c := range rec.calls {
			if c.Kind != drawCallRoundedButton {
				continue
			}
			if c.Color == accent && c.Rect.In(seg) {
				return true
			}
		}
		return false
	}

	if !accentIn(sc.SegmentRect(1)) {
		t.Fatalf("active segment %v has no accent keycap cap; calls=%+v", sc.SegmentRect(1), rec.calls)
	}
	if accentIn(sc.SegmentRect(0)) {
		t.Fatalf("inactive segment 0 should not carry an accent fill")
	}
	if accentIn(sc.SegmentRect(2)) {
		t.Fatalf("inactive segment 2 should not carry an accent fill")
	}
}

// TestSegmentedControl_DisabledSegmentNotAccented verifies a disabled segment
// never renders the lit-amber active cap even when it is the active index
// (e.g. the Synth segment greyed for WAV instruments). It also must not draw
// dark-on-amber active text.
func TestSegmentedControl_DisabledSegmentNotAccented(t *testing.T) {
	sc := NewSegmentedControl([]string{"A", "B", "C"}, 1, nil)
	sc.SetRect(image.Rect(0, 0, 300, 44))
	sc.SetSegmentDisabled(1, true)

	dst := ebiten.NewImage(300, 44)
	var rec drawCallRecorder
	rec.record(t, func() { sc.Draw(dst) })

	accent := color.RGBAModel.Convert(TokenAccent()).(color.RGBA)
	for _, c := range rec.calls {
		if c.Kind == drawCallRoundedButton && c.Color == accent && c.Rect.In(sc.SegmentRect(1)) {
			t.Fatalf("disabled active segment must not render the amber cap; got %+v", c)
		}
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
