//go:build test

package ui

import "testing"

// The channel pill must always agree with what the panel content is
// actually editing. On the Synth/Sampler tabs a "main" (Master) channel
// resolves to the first instrument row (synthTabActiveInstrument fallback,
// load-bearing across the suite) — so the pill must show THAT row's name,
// never "Master". Regression: mobile Synth/Sampler screenshots showed a
// gold "Master" pill above a dnb-kick banner (A4 in the 2026-07-04
// critique).
func TestChannelPillFollowsSynthTabResolution(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("expected a default row")
	}
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = "kick"

	z := dv.eqPanelZone
	z.SetActiveChannel("main")
	z.SetActiveTab(TabSynth)
	z.refreshChannelPillLabel()

	if got := z.stickyBar.ChannelBtn().Text; got != "Kick" {
		t.Fatalf("Synth tab under Master channel: pill = %q, want resolved row name %q", got, "Kick")
	}
}

// Outside Synth/Sampler the Master label stays; and an instrument channel
// set programmatically (raw id) must display the row NAME — the dropdown
// path already showed names, the SetActiveChannel path leaked raw ids
// ("dnb-kick" in the desktop Sampler screenshot).
func TestChannelPillShowsRowNameNotRawID(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = "kick"

	z := dv.eqPanelZone
	z.SetActiveTab(TabEQ)

	z.SetActiveChannel("main")
	z.refreshChannelPillLabel()
	if got := z.stickyBar.ChannelBtn().Text; got != "Master" {
		t.Fatalf("EQ tab on Master: pill = %q, want %q", got, "Master")
	}

	z.SetActiveChannel("kick")
	z.refreshChannelPillLabel()
	if got := z.stickyBar.ChannelBtn().Text; got != "Kick" {
		t.Fatalf("EQ tab on kick: pill = %q, want row name %q", got, "Kick")
	}
}
