//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTimelineLenButtonsHittableOnMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	restore := noInputForTest()
	defer restore()

	z, _ := newTestTimelineZone()
	zoneRect := image.Rect(100, 50, 660, 400)
	tree := registerTimelineZone(z, zoneRect)

	// Create len+/- buttons positioned outside the zone rect (like recalcButtons does).
	decClicked := 0
	incClicked := 0
	decBtn := &Button{OnClick: func() { decClicked++ }, Repeat: true}
	incBtn := &Button{OnClick: func() { incClicked++ }, Repeat: true}

	// Position buttons 4px to the right of zone rect (mimics the real layout bug).
	btnX := zoneRect.Max.X + 4
	decBtn.SetRect(image.Rect(btnX, 60, btnX+32, 80))
	incBtn.SetRect(image.Rect(btnX, 85, btnX+32, 105))

	z.SetButtons(nil, decBtn, incBtn)
	z.RefreshButtonHitAreas()

	// Layout + push to hit index.
	tree.Update()

	idx := tree.HitIndexRef()

	// Verify len-dec is hittable.
	decCX := (decBtn.Rect().Min.X + decBtn.Rect().Max.X) / 2
	decCY := (decBtn.Rect().Min.Y + decBtn.Rect().Max.Y) / 2
	hits := idx.At(decCX, decCY)
	foundDec := false
	for _, h := range hits {
		if h.Tag == "timeline-len-dec" {
			foundDec = true
		}
	}
	if !foundDec {
		t.Errorf("expected timeline-len-dec to be hittable at (%d,%d) on mobile, got %d hits: %v",
			decCX, decCY, len(hits), indexedHitTags(hits))
	}

	// Verify len-inc is hittable.
	incCX := (incBtn.Rect().Min.X + incBtn.Rect().Max.X) / 2
	incCY := (incBtn.Rect().Min.Y + incBtn.Rect().Max.Y) / 2
	hits = idx.At(incCX, incCY)
	foundInc := false
	for _, h := range hits {
		if h.Tag == "timeline-len-inc" {
			foundInc = true
		}
	}
	if !foundInc {
		t.Errorf("expected timeline-len-inc to be hittable at (%d,%d) on mobile, got %d hits: %v",
			incCX, incCY, len(hits), indexedHitTags(hits))
	}
}

func TestTimelineLenButtonsHittableOnDesktop(t *testing.T) {
	forceSmallScreenForTest = false

	restore := noInputForTest()
	defer restore()

	z, _ := newTestTimelineZone()
	zoneRect := image.Rect(100, 50, 660, 400)
	tree := registerTimelineZone(z, zoneRect)

	decBtn := &Button{OnClick: func() {}, Repeat: true}
	incBtn := &Button{OnClick: func() {}, Repeat: true}

	btnX := zoneRect.Max.X + 4
	decBtn.SetRect(image.Rect(btnX, 60, btnX+32, 80))
	incBtn.SetRect(image.Rect(btnX, 85, btnX+32, 105))

	z.SetButtons(nil, decBtn, incBtn)
	z.RefreshButtonHitAreas()
	tree.Update()

	idx := tree.HitIndexRef()

	decCX := (decBtn.Rect().Min.X + decBtn.Rect().Max.X) / 2
	decCY := (decBtn.Rect().Min.Y + decBtn.Rect().Max.Y) / 2
	hits := idx.At(decCX, decCY)
	foundDec := false
	for _, h := range hits {
		if h.Tag == "timeline-len-dec" {
			foundDec = true
		}
	}
	if !foundDec {
		t.Errorf("expected timeline-len-dec to be hittable on desktop at (%d,%d), got %d hits",
			decCX, decCY, len(hits))
	}
}

func TestTimelineLenButtonPressFiresCallbackMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	z, _ := newTestTimelineZone()
	zoneRect := image.Rect(100, 50, 660, 400)
	tree := registerTimelineZone(z, zoneRect)

	incClicked := 0
	incBtn := &Button{OnClick: func() { incClicked++ }, Repeat: true}

	btnX := zoneRect.Max.X + 4
	incBtn.SetRect(image.Rect(btnX, 85, btnX+32, 105))

	z.SetButtons(nil, nil, incBtn)
	z.RefreshButtonHitAreas()

	// Layout frame.
	tree.Update()

	// Press on the button center.
	mx = (incBtn.Rect().Min.X + incBtn.Rect().Max.X) / 2
	my = (incBtn.Rect().Min.Y + incBtn.Rect().Max.Y) / 2
	pressed = true
	tree.Update()

	if incClicked == 0 {
		t.Error("expected inc button OnClick to fire on mobile press")
	}

	// Release.
	pressed = false
	tree.Update()
}

// indexedHitTags extracts tags from indexed hit areas for diagnostic output.
func indexedHitTags(hits []indexedHitArea) []string {
	tags := make([]string, len(hits))
	for i, h := range hits {
		tags[i] = h.Tag
	}
	return tags
}
