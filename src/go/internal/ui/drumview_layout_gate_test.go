//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// A frame where nothing they lay out from changed skips them; a change still reaches the next frame.
func TestIdleFramesSkipForcedPanelLayouts(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	if len(g.drum.Rows) == 0 {
		g.drum.AddRow()
	}
	expandEQPanelForTest(t, g)
	z, tz := g.drum.eqPanelZone, g.drum.timelineZone
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	z.layouts, tz.layouts = 0, 0
	const idle = 32
	for i := 0; i < idle; i++ {
		_ = g.Update()
	}
	if z.layouts == 0 || z.layouts > idle/eqPanelRelayoutEvery+1 {
		t.Fatalf("audio panel laid out %d times in %d idle frames, want 1..%d (the safety re-layout, no more)", z.layouts, idle, idle/eqPanelRelayoutEvery+1)
	}
	if tz.layouts > 0 {
		t.Fatalf("timeline laid out %d times in %d idle frames: none of its inputs moved", tz.layouts, idle)
	}

	// A rename of the selected row reaches the channel pill on the next frame.
	row := g.drum.Rows[0]
	g.drum.setEQActiveChannel(channel.Of(row.Instrument))
	_ = g.Update()
	row.Name = "Renamed Row"
	_ = g.Update()
	if got := z.stickyBar.ChannelBtn().Text; got != "Renamed Row" {
		t.Fatalf("channel pill reads %q after the rename, want the new name", got)
	}

	// A tab switch lays the panel out on the next frame.
	before := z.layouts
	z.SetActiveTab(TabSpectrum)
	_ = g.Update()
	if z.layouts == before {
		t.Fatal("a tab switch did not lay the panel out")
	}

	// So does a bridge call (any export can change panel state).
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}
	before = z.layouts
	jsExportSeq.Add(1)
	_ = g.Update()
	if z.layouts == before {
		t.Fatal("a bridge call did not lay the panel out")
	}
}
