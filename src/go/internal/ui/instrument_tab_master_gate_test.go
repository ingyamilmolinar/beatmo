//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Synth and Sampler edit one instrument and mean nothing on the master bus, so with Master active their desktop
// pills are disabled, onTab refuses them and the mobile segments report disabled. buildChannelDropdown hides
// Master while on those tabs, so no redirect is needed.

// tabPillForTest returns the sticky-bar tab pill for a given PanelTab, or nil.
func tabPillForTest(b *AudioStickyBar, tab PanelTab) *Button {
	for i, t := range AllPanelTabs() {
		if t == tab {
			return b.TabBtn(i)
		}
	}
	return nil
}

func instrumentRowsForTest() []*DrumRow {
	return []*DrumRow{
		{Instrument: roster.Kick, Name: "Kick"},
		{Instrument: roster.Snare, Name: "Snare"},
	}
}

func TestInstrumentTabsDisabledWhenMasterSelected(t *testing.T) {
	z, _ := newTestEQPanelZone(instrumentRowsForTest())
	z.SetActiveChannel(channel.Master) // Master
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
	z.SetActiveChannel(channel.Of(roster.Kick)) // a concrete instrument
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
	z.SetActiveChannel(channel.Master)

	// Bypassing the disabled pill's hit adapter: the onTab closure itself must refuse, so no path reaches the tab.
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
	z.SetActiveChannel(channel.Of(roster.Kick))
	if pill := tabPillForTest(z.stickyBar, TabSynth); pill != nil && pill.OnClick != nil {
		pill.OnClick()
	}
	if got := z.ActiveTab(); got != TabSynth {
		t.Errorf("onTab(TabSynth) with instrument selected should switch, got %v", got)
	}
}

func TestMobileInstrumentViewsDisabledWhenMasterSelected(t *testing.T) {
	g := newSynthTabGame(t) // Rows[0].Instrument == "snare"
	dv := g.drum

	dv.eqPanelZone.SetActiveChannel(channel.Master)
	if !dv.samplerViewDisabled() {
		t.Error("Sampler view should be disabled under Master")
	}
	if !dv.synthViewDisabled() {
		t.Error("Synth view should be disabled under Master")
	}

	dv.eqPanelZone.SetActiveChannel(channel.Of(roster.Snare))
	if dv.samplerViewDisabled() {
		t.Error("Sampler view should be enabled once an instrument is selected")
	}
	// Synth also needs synth controls, which snare has.
	if dv.synthViewDisabled() {
		t.Error("Synth view should be enabled for a synth-capable instrument")
	}
}
