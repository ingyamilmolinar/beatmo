//go:build test

package ui

import (
	"image"
	"strings"
	"testing"
)

// --- Task B1: AudioStickyBar in isolation ---

// newTestAudioStickyBar constructs a sticky bar with no-op callbacks and lays
// it out into the supplied rect. Returns the bar plus a count of invocations
// for each callback channel so tests can assert routing.
func newTestAudioStickyBar(t *testing.T, rect image.Rectangle) (*AudioStickyBar, *stickyBarCallbackLog) {
	t.Helper()
	log := &stickyBarCallbackLog{}
	bar := NewAudioStickyBar(
		130,
		func() { log.channel++ },
		func(tab PanelTab) { log.tabs = append(log.tabs, tab) },
	)
	bar.Layout(rect)
	return bar, log
}

type stickyBarCallbackLog struct {
	channel int
	tabs    []PanelTab
}

// TestStickyBarLayoutFitsWithinRect verifies every visible button stays inside
// the bar's rect after Layout.
func TestStickyBarLayoutFitsWithinRect(t *testing.T) {
	assertDefaultParityState(t)

	bar, _ := newTestAudioStickyBar(t, image.Rect(0, 0, 800, stickyBarH))

	check := func(name string, btn *Button) {
		t.Helper()
		if btn == nil {
			return
		}
		r := btn.Rect()
		if r.Empty() {
			return // some buttons may be intentionally hidden (e.g. tabs on mobile)
		}
		if r.Max.X > 800 {
			t.Errorf("%s rect Max.X=%d exceeds bar width 800", name, r.Max.X)
		}
		if r.Min.X < 0 {
			t.Errorf("%s rect Min.X=%d before bar Min.X=0", name, r.Min.X)
		}
		if r.Min.Y < 0 {
			t.Errorf("%s rect Min.Y=%d before bar Min.Y=0", name, r.Min.Y)
		}
		if r.Max.Y > stickyBarH {
			t.Errorf("%s rect Max.Y=%d exceeds bar height %d", name, r.Max.Y, stickyBarH)
		}
	}

	check("channel", bar.ChannelBtn())
	for i := 0; i < 5; i++ {
		check("tab"+string(rune('0'+i)), bar.TabBtn(i))
	}
	check("legend", bar.LegendBtn())
	check("expander", bar.ExpanderBtn())
}

// TestStickyBarHitAreasUseParentZIndex verifies all hit areas register at the
// parent's z-index + 1.
func TestStickyBarHitAreasUseParentZIndex(t *testing.T) {
	assertDefaultParityState(t)

	bar, _ := newTestAudioStickyBar(t, image.Rect(0, 0, 800, stickyBarH))
	areas := bar.HitAreas()
	if len(areas) == 0 {
		t.Fatal("sticky bar should have hit areas after layout")
	}
	for _, a := range areas {
		if a.ZIndex != 130+1 {
			t.Errorf("hit area %q has ZIndex=%d, want %d", a.Tag, a.ZIndex, 131)
		}
	}
}

// TestStickyBarTabButtonsAreCount6 verifies TabBtn(0..5) are non-nil
// (EQ, Wave, Spectrum, Levels, Chain, Synth) and TabBtn(6) is nil.
func TestStickyBarTabButtonsMatchAllPanelTabs(t *testing.T) {
	assertDefaultParityState(t)

	bar, _ := newTestAudioStickyBar(t, image.Rect(0, 0, 800, stickyBarH))
	n := len(AllPanelTabs())
	for i := 0; i < n; i++ {
		if bar.TabBtn(i) == nil {
			t.Errorf("TabBtn(%d) should be non-nil (%d tabs)", i, n)
		}
	}
	if bar.TabBtn(n) != nil {
		t.Errorf("TabBtn(%d) should be nil (only %d tabs)", n, n)
	}
	if bar.TabBtn(-1) != nil {
		t.Error("TabBtn(-1) should be nil")
	}
}

// TestStickyBarChannelClickInvokesCallback verifies clicking the channel
// button invokes the onChannel callback.
func TestStickyBarChannelClickInvokesCallback(t *testing.T) {
	assertDefaultParityState(t)

	bar, log := newTestAudioStickyBar(t, image.Rect(0, 0, 800, stickyBarH))
	if bar.ChannelBtn() == nil || bar.ChannelBtn().OnClick == nil {
		t.Fatal("channel button should have OnClick set")
	}
	bar.ChannelBtn().OnClick()
	if log.channel != 1 {
		t.Errorf("expected channel callback invoked once, got %d", log.channel)
	}
}

// NOTE: the freeze toggle, the close button, and the freq-scale (log/lin) chip
// were removed from the sticky bar in the slim-bar phase. Freeze + freq-scale
// moved into the per-tab control components (audio_tab_controls.go) — covered
// by TestWaveControls* / TestLevelsControls* / TestSpectrumControls*. The X
// close button was deleted entirely (TestCloseButtonGone in
// audio_tab_controls_test.go pins its absence).

// TestStickyBarRectMatchesLastLayout exercises (*AudioStickyBar).Rect(),
// the public bounds accessor used by the screenshot harness (subject
// SubjectAudioStickyBar) to crop captures without reaching into private
// fields. Layout() must persist the rect verbatim; Rect() must return it.
func TestStickyBarRectMatchesLastLayout(t *testing.T) {
	assertDefaultParityState(t)

	bar, _ := newTestAudioStickyBar(t, image.Rect(0, 0, 800, stickyBarH))
	if got := bar.Rect(); got != (image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{800, stickyBarH}}) {
		t.Errorf("Rect()=%v after first Layout, want (0,0)-(800,%d)", got, stickyBarH)
	}

	// A second Layout overwrites the stored rect.
	bar.Layout(image.Rect(120, 40, 920, 40+stickyBarH))
	if got := bar.Rect(); got != (image.Rectangle{Min: image.Point{120, 40}, Max: image.Point{920, 40 + stickyBarH}}) {
		t.Errorf("Rect()=%v after second Layout, want (120,40)-(920,%d)", got, 40+stickyBarH)
	}
}

// --- Task B2: integration with EQPanelZone ---

// TestEQStickyBarOwnsAllChromeHits verifies that every chrome-prefixed hit
// area (channel/tab-N) on the panel is present in the sticky bar's hit-area
// list (set membership by tag). Freeze + close are no longer bar chrome
// (slim-bar phase).
func TestEQStickyBarOwnsAllChromeHits(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	restore := noInputForTest()
	tree.Update()
	restore()

	if z.stickyBar == nil {
		t.Fatal("EQPanelZone.stickyBar should be non-nil after Layout")
	}
	stickyTags := map[string]bool{}
	for _, a := range z.stickyBar.HitAreas() {
		stickyTags[a.Tag] = true
	}

	chromePrefixes := []string{"eq-channel-btn"}
	for _, prefix := range chromePrefixes {
		matched := false
		for _, a := range z.HitAreas() {
			if a.Tag == prefix {
				matched = true
				if !stickyTags[a.Tag] {
					t.Errorf("chrome hit %q present on panel but missing from sticky bar", a.Tag)
				}
			}
		}
		if !matched {
			// The channel pill must always be present (it's the persistent
			// left-anchored selector on every tab).
			if prefix == "eq-channel-btn" {
				t.Errorf("expected chrome hit %q present on panel", prefix)
			}
		}
	}

	// Tab hits (desktop only); skip on mobile profile.
	if !Profile().IsMobile() {
		for _, a := range z.HitAreas() {
			if strings.HasPrefix(a.Tag, "eq-tab-") {
				if !stickyTags[a.Tag] {
					t.Errorf("tab hit %q present on panel but missing from sticky bar", a.Tag)
				}
			}
		}
	}
}

// NOTE: TestEQHasCloseButton was deleted in the slim-bar phase — the X close
// button and EQCallbacks.OnClose were removed entirely. TestCloseButtonGone in
// audio_tab_controls_test.go now pins the button's absence on every tab.

// TestEQHPFLPFInsideContentRectOnEQTab verifies that on TabEQ the HPF and LPF
// buttons sit inside the panel's content rect (below the sticky bar) and not
// in the chrome strip.
func TestEQHPFLPFInsideContentRectOnEQTab(t *testing.T) {
	assertDefaultParityState(t)

	z, _ := newTestEQPanelZone(nil)
	tree := registerEQZone(z, image.Rect(0, 400, 600, 580))
	restore := noInputForTest()
	tree.Update()
	restore()

	if z.tabState.ActiveTab() != TabEQ {
		t.Fatalf("expected default TabEQ, got %v", z.tabState.ActiveTab())
	}

	content := z.ContentRect()
	if z.hpfBtn == nil || z.hpfBtn.Rect().Empty() {
		t.Fatal("hpfBtn should be laid out on TabEQ")
	}
	if z.lpfBtn == nil || z.lpfBtn.Rect().Empty() {
		t.Fatal("lpfBtn should be laid out on TabEQ")
	}
	for _, c := range []struct {
		name string
		r    image.Rectangle
	}{
		{"hpf", z.hpfBtn.Rect()},
		{"lpf", z.lpfBtn.Rect()},
	} {
		if !c.r.In(content) && !c.r.Overlaps(content) {
			t.Errorf("%s rect %v not inside content rect %v", c.name, c.r, content)
		}
		// Sticky bar occupies [rect.Min.Y, rect.Min.Y+stickyBarH); HPF/LPF
		// must sit below that strip.
		if c.r.Min.Y < z.rect.Min.Y+stickyBarH {
			t.Errorf("%s Min.Y=%d should be >= stickyBar bottom %d",
				c.name, c.r.Min.Y, z.rect.Min.Y+stickyBarH)
		}
	}
}
