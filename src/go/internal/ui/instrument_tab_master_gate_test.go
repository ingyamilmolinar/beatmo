//go:build test

package ui

import (
	"image"
	"testing"
)

// The Synth and Sampler tabs edit a single instrument's recipe / sample and
// are meaningless on the master bus. These tests pin the user-facing rule
// "when the audio panel's active channel is Master, the Synth and Sampler
// tabs are blocked":
//   - both desktop tab pills are Disabled (the shared hit-adapter swallows
//     clicks, and the pill renders greyed),
//   - the onTab pill-click callback refuses to switch into them,
//   - the mobile bottom-nav Synth/Sampler segments report disabled.
//
// The complementary direction (Master hidden from the channel dropdown while
// on Synth/Sampler) already lives in buildChannelDropdown, so the real UI can
// never reach "Master + Synth"; that is why no programmatic redirect is
// needed here — these guards are purely about not letting the user enter the
// tabs from a Master context.

// tabPillForTest returns the sticky-bar tab pill for a given PanelTab, or nil.
func tabPillForTest(b *AudioStickyBar, tab PanelTab) *Button {
	for i, t := range AllPanelTabs() {
		if t == tab {
			return b.TabBtn(i)
		}
	}
	return nil
}

// instrumentRowsForTest returns two rows whose instruments can be selected as
// the active channel.
func instrumentRowsForTest() []*DrumRow {
	return []*DrumRow{
		{Instrument: "kick", Name: "Kick"},
		{Instrument: "snare", Name: "Snare"},
	}
}

func TestInstrumentTabsDisabledWhenMasterSelected(t *testing.T) {
	z, _ := newTestEQPanelZone(instrumentRowsForTest())
	z.SetActiveChannel("main") // Master
	z.Layout(image.Rect(0, 400, 600, 580))

	synth := tabPillForTest(z.stickyBar, TabSynth)
	sampler := tabPillForTest(z.stickyBar, TabSampler)
	if synth == nil || sampler == nil {
		t.Fatal("expected Synth and Sampler tab pills to exist")
	}
	if !synth.Disabled {
		t.Error("Synth pill should be Disabled when Master channel is selected")
	}
	if !sampler.Disabled {
		t.Error("Sampler pill should be Disabled when Master channel is selected")
	}
}

func TestInstrumentTabsEnabledWhenInstrumentSelected(t *testing.T) {
	z, _ := newTestEQPanelZone(instrumentRowsForTest())
	z.SetActiveChannel("kick") // a concrete instrument
	z.Layout(image.Rect(0, 400, 600, 580))

	synth := tabPillForTest(z.stickyBar, TabSynth)
	sampler := tabPillForTest(z.stickyBar, TabSampler)
	if synth == nil || sampler == nil {
		t.Fatal("expected Synth and Sampler tab pills to exist")
	}
	if synth.Disabled {
		t.Error("Synth pill should be enabled when a concrete instrument is selected")
	}
	if sampler.Disabled {
		t.Error("Sampler pill should be enabled when a concrete instrument is selected")
	}
}

func TestOnTabClickRefusesInstrumentTabsUnderMaster(t *testing.T) {
	z, _ := newTestEQPanelZone(instrumentRowsForTest())
	z.SetActiveChannel("main")

	// Invoking the pill's click callback directly exercises the onTab closure
	// (bypassing the Disabled-pill hit adapter) — the closure itself must gate
	// so no code path can switch into an instrument tab under Master.
	if pill := tabPillForTest(z.stickyBar, TabSynth); pill != nil && pill.OnClick != nil {
		pill.OnClick()
	}
	if got := z.ActiveTab(); got == TabSynth {
		t.Errorf("onTab(TabSynth) under Master should be refused, got %v", got)
	}
	if pill := tabPillForTest(z.stickyBar, TabSampler); pill != nil && pill.OnClick != nil {
		pill.OnClick()
	}
	if got := z.ActiveTab(); got == TabSampler {
		t.Errorf("onTab(TabSampler) under Master should be refused, got %v", got)
	}

	// With an instrument selected the same click switches into the tab.
	z.SetActiveChannel("kick")
	if pill := tabPillForTest(z.stickyBar, TabSynth); pill != nil && pill.OnClick != nil {
		pill.OnClick()
	}
	if got := z.ActiveTab(); got != TabSynth {
		t.Errorf("onTab(TabSynth) with instrument selected should switch, got %v", got)
	}
}

// TestMobileInstrumentViewsDisabledWhenMasterSelected covers the mobile
// bottom-nav parity: the Synth/Sampler segment-disable predicates track the
// active channel the same way the desktop pills do.
func TestMobileInstrumentViewsDisabledWhenMasterSelected(t *testing.T) {
	g := newSynthTabGame(t) // Rows[0].Instrument == "snare"
	dv := g.drum

	dv.eqPanelZone.SetActiveChannel("main")
	if !dv.samplerViewDisabled() {
		t.Error("Sampler view should be disabled under Master")
	}
	if !dv.synthViewDisabled() {
		t.Error("Synth view should be disabled under Master")
	}

	dv.eqPanelZone.SetActiveChannel("snare")
	if dv.samplerViewDisabled() {
		t.Error("Sampler view should be enabled once an instrument is selected")
	}
	// Synth view additionally requires the instrument to have synth controls;
	// "snare" (bound to drum-snare) does, so it should be enabled here.
	if dv.synthViewDisabled() {
		t.Error("Synth view should be enabled for a synth-capable instrument")
	}
}
