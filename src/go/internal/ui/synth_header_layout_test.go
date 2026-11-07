//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A generous headerH, so the density token governs the button height.
func synthHeaderForTest(t *testing.T, contentR image.Rectangle, inst instrument.ID, mobile bool) synthHeaderLayout {
	t.Helper()
	dv := &DrumView{}
	dv.layoutSynthHeader(contentR, 64, inst, mobile)
	return dv.instEditorHeader
}

// Like the Sampler: title left, actions right in Preview < Save < Save As < Reset < ⋯, ⋯ flush against the inset.
func TestSynthHeader_SamplerStyle(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := synthHeaderForTest(t, image.Rect(0, 0, 900, 200), roster.Violin, false)
	if h.rect.Empty() {
		t.Fatal("header rect empty")
	}
	if !(h.previewRect.Max.X <= h.saveAsRect.Min.X) {
		t.Fatalf("buttons not ordered Preview<SaveAs: %v %v", h.previewRect, h.saveAsRect)
	}
	if h.moreRect.Max.X != h.rect.Max.X-SpaceMD || h.saveAsRect.Max.X > h.moreRect.Min.X {
		t.Fatalf("⋯ not flush right after Save As: save-as %v more %v header %v", h.saveAsRect, h.moreRect, h.rect)
	}
	if h.moreRect.Dx() != h.moreRect.Dy() {
		t.Fatalf("⋯ is an icon keycap (square), got %v", h.moreRect)
	}
	for _, r := range []image.Rectangle{h.previewRect, h.saveAsRect, h.moreRect} {
		if r.Min.Y != h.previewRect.Min.Y || r.Dy() != h.previewRect.Dy() {
			t.Fatalf("action cluster not aligned: %v vs %v", r, h.previewRect)
		}
	}
	if got, want := audioTabHeaderName(roster.Violin), "Violin"; got != want {
		t.Fatalf("title name %q want %q", got, want)
	}
}

func TestSynthHeaderButtonHeightFromDensity(t *testing.T) {
	measure := func(d Density) int {
		restore := SetDensityForTest(d)
		defer restore()
		h := synthHeaderForTest(t, image.Rect(0, 0, 1000, 200), "kick-1", false)
		return h.saveAsRect.Dy()
	}
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		want := Profile().DensityValues().SynthHeaderButtonH
		restore()
		if got := measure(d); got != want {
			t.Errorf("density=%v: Save As button height %d, want SynthHeaderButtonH %d", d, got, want)
		}
	}
}

// Label-derived widths.
func TestSynthHeaderButtonsFitLabels(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := synthHeaderForTest(t, image.Rect(0, 0, 1000, 200), "kick-1", false)
	for _, tc := range []struct {
		name  string
		r     image.Rectangle
		label string
	}{
		{"preview", h.previewRect, "Preview"},
		{"saveAs", h.saveAsRect, "Save As"},
	} {
		if tc.r.Empty() {
			t.Errorf("%s button empty on a wide desktop header", tc.name)
			continue
		}
		if tc.r.Dx() < TextWidth(tc.label) {
			t.Errorf("%s button width %d < label %q width %d (would truncate)",
				tc.name, tc.r.Dx(), tc.label, TextWidth(tc.label))
		}
	}
}

// The actions live on the sticky bar.
func TestSynthHeaderMobileLaysOutNothing(t *testing.T) {
	h := synthHeaderForTest(t, image.Rect(0, 0, 360, 200), "kick-1", true)
	if !h.rect.Empty() || !h.saveAsRect.Empty() || !h.previewRect.Empty() {
		t.Fatalf("mobile header laid out in-panel rects: %+v", h)
	}
}
