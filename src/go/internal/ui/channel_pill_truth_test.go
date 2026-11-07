//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The pill names what the content edits: on Synth/Sampler Master resolves to the first row, so the pill shows that
// row's name.
func TestChannelPillFollowsSynthTabResolution(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("expected a default row")
	}
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = roster.Kick

	z := dv.eqPanelZone
	z.SetActiveChannel(channel.Master)
	z.SetActiveTab(TabSynth)
	z.refreshChannelPillLabel()

	if got := z.stickyBar.ChannelBtn().Text; got != "Kick" {
		t.Fatalf("Synth tab under Master channel: pill = %q, want resolved row name %q", got, "Kick")
	}
}

// Elsewhere Master stays; a channel set programmatically shows the row name, never the raw id.
func TestChannelPillShowsRowNameNotRawID(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = roster.Kick

	z := dv.eqPanelZone
	z.SetActiveTab(TabEQ)

	z.SetActiveChannel(channel.Master)
	z.refreshChannelPillLabel()
	if got := z.stickyBar.ChannelBtn().Text; got != "Master" {
		t.Fatalf("EQ tab on Master: pill = %q, want %q", got, "Master")
	}

	z.SetActiveChannel(channel.Of(roster.Kick))
	z.refreshChannelPillLabel()
	if got := z.stickyBar.ChannelBtn().Text; got != "Kick" {
		t.Fatalf("EQ tab on kick: pill = %q, want row name %q", got, "Kick")
	}
}
