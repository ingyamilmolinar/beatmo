package ui

import (
	"image"
	"testing"
)

// controlHeaderHeight mirrors stickyBarHeight: 32 desktop, 40 mobile.
func TestControlHeaderHeight(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	r2 := SetDensityForTest(DensityComfortable)
	defer r2()
	if got := controlHeaderHeight(); got != audioControlHeaderH {
		t.Fatalf("desktop control header height = %d, want %d", got, audioControlHeaderH)
	}
}

// newFreezePill builds an icon-only pause pill that flips to a play icon /
// colAccent when the freeze callback reports frozen, and back to the pause icon
// / colTextSecondary. Text stays empty throughout (drawPillTabAt renders the
// icon when Text == "").
func TestNewFreezePillTogglesVisual(t *testing.T) {
	frozen := false
	b := newFreezePill(func() bool { frozen = !frozen; return frozen })
	if b.Icon != string(IconPause) || b.Text != "" {
		t.Fatalf("initial freeze Icon=%q Text=%q, want pause icon / empty text", b.Icon, b.Text)
	}
	b.OnClick()
	if b.Icon != string(IconPlay) || b.Text != "" {
		t.Fatalf("after first toggle Icon=%q Text=%q, want play icon / empty text", b.Icon, b.Text)
	}
	b.OnClick()
	if b.Icon != string(IconPause) || b.Text != "" {
		t.Fatalf("after second toggle Icon=%q Text=%q, want pause icon / empty text", b.Icon, b.Text)
	}
}

// With no components built, the active controls are nil on every tab, the
// header height is 0, and bodyRect equals contentRect (zero behavior change).
func TestPhase0NoControlHeader(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.Layout(image.Rect(0, 400, 600, 600))
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		if tab != TabWave && tab != TabMeters && tab != TabSpectrum {
			if z.controlHeaderH() != 0 {
				t.Fatalf("tab %v: controlHeaderH = %d, want 0", tab, z.controlHeaderH())
			}
			if z.bodyRect() != z.contentRect() {
				t.Fatalf("tab %v: bodyRect %v != contentRect %v", tab, z.bodyRect(), z.contentRect())
			}
		}
	}
}

// On TabWave, the dispatcher publishes the wave AUTO and freeze hit areas in the
// control header (below the switcher row, above the body), and NOT via the
// sticky bar.
func TestWaveControlsOwnFreeze(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.SetActiveTab(TabWave)
	z.Layout(image.Rect(0, 400, 600, 600))

	if z.controlHeaderH() != audioControlHeaderH {
		t.Fatalf("wave control header = %d, want %d", z.controlHeaderH(), audioControlHeaderH)
	}
	if z.bodyRect().Min.Y != z.contentRect().Min.Y+audioControlHeaderH {
		t.Fatalf("body top %d, want %d", z.bodyRect().Min.Y, z.contentRect().Min.Y+audioControlHeaderH)
	}

	var autoHit, freezeHit *HitArea
	for i := range z.HitAreas() {
		if z.HitAreas()[i].Tag == "wave-freeze-btn" {
			freezeHit = &z.HitAreas()[i]
		}
		if z.HitAreas()[i].Tag == "wave-auto-btn" {
			autoHit = &z.HitAreas()[i]
		}
	}
	if autoHit == nil {
		t.Fatal("wave-auto-btn hit area not published on TabWave")
	}
	if freezeHit == nil {
		t.Fatal("wave-freeze-btn hit area not published on TabWave")
	}
	if autoHit.Rect.Min.Y < z.headerRect().Min.Y || autoHit.Rect.Max.Y > z.headerRect().Max.Y {
		t.Fatalf("auto rect %v not inside header %v", autoHit.Rect, z.headerRect())
	}

	// Clicking the wave AUTO pill toggles the zone's auto-gain.
	z.waveAutoGain = true
	z.waveControls.autoBtn.OnClick()
	if z.waveAutoGain {
		t.Fatal("clicking wave AUTO did not toggle waveAutoGain off")
	}
}

func TestLevelsControlsButtonsAndWiring(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.levelsLatches = NewMultiLevelsLatch()
	z.SetActiveTab(TabMeters)
	z.Layout(image.Rect(0, 400, 600, 600))

	tags := map[string]bool{}
	for _, h := range z.HitAreas() {
		tags[h.Tag] = true
	}
	for _, want := range []string{"levels-freeze-btn", "levels-clear-clips-btn", "levels-k20-btn"} {
		if !tags[want] {
			t.Fatalf("levels control %q hit area missing", want)
		}
	}

	// K-20 toggles via the component.
	if z.levelsControls.K20View() {
		t.Fatal("K20 should start false")
	}
	z.levelsControls.k20Btn.OnClick()
	if !z.levelsControls.K20View() {
		t.Fatal("K20 toggle did not flip")
	}

	// Clear-Clips clears the latches (proxy for the wired callback firing).
	z.levelsLatches.Get("main").Update(3, -1, -1)
	if !z.levelsLatches.Get("main").Latched() {
		t.Fatal("precondition: latch should be latched after a clip update")
	}
	z.levelsControls.clearClipsBtn.OnClick()
	if z.levelsLatches.Get("main").Latched() {
		t.Fatal("Clear-Clips did not clear the latch")
	}
}

func TestSpectrumControlsButtonsAndWiring(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.SetActiveTab(TabSpectrum)
	z.Layout(image.Rect(0, 400, 600, 600))

	tags := map[string]bool{}
	for _, h := range z.HitAreas() {
		tags[h.Tag] = true
	}
	for _, want := range []string{
		"spectrum-freeze-btn", "spectrum-freqscale-btn",
		"spectrum-slope-btn", "spectrum-pre-btn", "spectrum-reset-hold-btn",
	} {
		if !tags[want] {
			t.Fatalf("spectrum control %q hit area missing", want)
		}
	}

	if z.spectrumControls.SlopeDBPerOct() != 0 {
		t.Fatal("slope should start at 0")
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 3 {
		t.Fatalf("slope after 1 click = %v, want 3", z.spectrumControls.SlopeDBPerOct())
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 4.5 {
		t.Fatalf("slope after 2 clicks = %v, want 4.5", z.spectrumControls.SlopeDBPerOct())
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 0 {
		t.Fatalf("slope after 3 clicks = %v, want 0 (wrap)", z.spectrumControls.SlopeDBPerOct())
	}

	if !z.spectrumControls.FreqScaleLog() {
		t.Fatal("freq scale should default to log")
	}
	z.spectrumControls.freqScaleBtn.OnClick()
	if z.spectrumControls.FreqScaleLog() {
		t.Fatal("freq scale did not toggle to lin")
	}

	if z.spectrumControls.PreOverlay() {
		t.Fatal("pre overlay should start false")
	}
	z.spectrumControls.preBtn.OnClick()
	if !z.spectrumControls.PreOverlay() {
		t.Fatal("pre overlay did not toggle")
	}

	z.spectrumControls.resetHoldBtn.OnClick() // must not panic
}

// On another tab, the wave AUTO hit area is not published.
func TestWaveControlsInactiveNoHitAreas(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.SetActiveTab(TabSpectrum)
	z.Layout(image.Rect(0, 400, 600, 600))
	for _, h := range z.HitAreas() {
		if h.Tag == "wave-auto-btn" {
			t.Fatal("wave-auto-btn must not publish when Spectrum is active")
		}
	}
}

// The main tab-switcher row publishes ONLY channel/tabs/legend/expander hit
// areas — never any per-tab control or the close button — on every tab.
func TestMainTabBarOnlyHasTabsChannelLegendExpander(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	banned := map[string]bool{
		"eq-freeze-btn": true, "eq-freqscale-btn": true, "eq-slope-btn": true,
		"eq-pre-btn": true, "eq-reset-hold-btn": true, "eq-clear-clips-btn": true,
		"eq-k20-btn": true, "eq-close-btn": true,
	}
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		for _, h := range z.stickyBar.HitAreas() {
			if banned[h.Tag] {
				t.Fatalf("tab %v: sticky bar still publishes banned hit %q", tab, h.Tag)
			}
		}
	}
}

// On mobile, the desktop-only pills are absent but the freeze button is still
// present on Spectrum/Levels (no mobile freeze regression). The Wave tab no
// longer has a freeze pill (it has the AUTO pill instead).
func TestMobileControlVisibilityPreserved(t *testing.T) {
	setupMobileTest(t, true)
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Fatal("setupMobileTest did not force mobile profile")
	}

	freezeTag := map[PanelTab]string{
		TabSpectrum: "spectrum-freeze-btn",
		TabMeters:   "levels-freeze-btn",
	}
	desktopOnly := []string{
		"spectrum-slope-btn", "spectrum-pre-btn", "spectrum-reset-hold-btn",
		"spectrum-freqscale-btn", "levels-clear-clips-btn", "levels-k20-btn",
	}

	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	for _, tab := range []PanelTab{TabSpectrum, TabMeters} {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 360, 600))
		tags := map[string]bool{}
		for _, h := range z.HitAreas() {
			tags[h.Tag] = true
		}
		if !tags[freezeTag[tab]] {
			t.Fatalf("mobile tab %v: freeze %q missing", tab, freezeTag[tab])
		}
		for _, banned := range desktopOnly {
			if tags[banned] {
				t.Fatalf("mobile tab %v: desktop-only pill %q should be hidden on mobile", tab, banned)
			}
		}
	}
}

// The X close button is gone everywhere.
func TestCloseButtonGone(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{})
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		for _, h := range z.HitAreas() {
			if h.Tag == "eq-close-btn" {
				t.Fatalf("tab %v: eq-close-btn hit area still present", tab)
			}
		}
	}
}
