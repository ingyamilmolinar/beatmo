//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every opaque zone with its "<id>-capture" tag and nominal z; a new one also needs a NewInputCaptureHitArea
// over its Rect. Excluded: transport and rack, whose controls tile them; the timeline, transparent so the grid
// drag and row scroll get input; portals, which handle their own.
var zoneInputIsolationCases = []struct {
	zoneID     string
	captureTag string
	zNominal   int
}{
	{"eq-panel", "eq-panel-capture", 130},
	{"chain-panel", "chain-panel-capture", 140},
}

// A missing catch-all lets chrome whitespace fall through to a lower zone; the fix is always
// NewInputCaptureHitArea(zone.Rect(), z, tag).
func TestZoneInputIsolationDiscipline(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.recalcButtons()

	zones := zonesByID(g.drum)
	for _, c := range zoneInputIsolationCases {
		z, ok := zones[c.zoneID]
		if !ok {
			t.Errorf("zone %q not found in DrumView — discipline-test case is stale; either restore the zone or remove the case", c.zoneID)
			continue
		}
		assertZoneHasCaptureHitArea(t, z, c)
	}
}

// A hidden zone publishes no hit areas, or it swallows input meant for the rack beneath. The tree enforces this
// first; this catches a zone-level leak. Add new zones to the case table.
func TestZoneInvisibleEqualsNoInput(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)
	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	// So the tree's Layout phase and any re-publish settle.
	g.Update()
	g.Update()

	if !Profile().IsMobile() {
		t.Fatalf("test scaffold: forceSmallScreen did not flip Profile().IsMobile()")
	}
	if g.drum.MobileEQMode() {
		t.Fatalf("test scaffold: MobileEQMode should be false in viewMode=Rows")
	}

	zones := zonesByID(g.drum)
	for _, c := range zoneInputIsolationCases {
		z, ok := zones[c.zoneID]
		if !ok {
			continue // staleness is reported by the discipline test above
		}
		areas := z.HitAreas()
		if len(areas) != 0 {
			tags := make([]string, 0, len(areas))
			for _, h := range areas {
				tags = append(tags, h.Tag)
			}
			t.Errorf("zone %q is hidden (mobile + viewMode=Rows) but HitAreas() returned %d entries: %v. "+
				"Hidden zones must publish zero hit areas — either add an `IsHiddenForInput` predicate via the zone's callbacks, "+
				"or fix the tree's visibility gate in drumview_tree.go's Update Layout phase.",
				c.zoneID, len(areas), tags)
		}
	}
}

// Even with stale zone HitAreas(), the tree's index holds nothing from an invisible zone.
func TestZoneInvisibleNotInHitIndex(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)
	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	g.Update()
	g.Update()

	// viewMode=Rows hides the audio-panel zones.
	probePoints := []image.Point{
		{X: 50, Y: 50},
		{X: 180, Y: 400},
		{X: 350, Y: 700},
	}
	forbiddenTags := []string{"eq-panel-capture", "chain-panel-capture", "synth-control-0", "synth-control-1"}
	for _, pt := range probePoints {
		hits := g.drum.tree.HitIndexRef().At(pt.X, pt.Y)
		for _, h := range hits {
			for _, bad := range forbiddenTags {
				if h.Tag == bad {
					t.Errorf("HitIndex.At(%d,%d) returned hidden-zone hit %q (z=%d) — tree's visibility gate is broken",
						pt.X, pt.Y, h.Tag, h.ZIndex)
				}
			}
		}
	}
}

// EQPanelZone hosts every audio tab, so in each view mode its HitAreas() carry only the active tab's tags: another
// tab's tag means its controls are still live and can swallow input. Every view mode is walked.
func TestTabIsolation_OnlyActiveTabPublishes(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	for _, c := range allViewModeCases {
		dv.setViewMode(c.mode)
		advanceFrames(g, 2)
		if dv.eqPanelZone == nil {
			continue
		}
		for _, ha := range dv.eqPanelZone.HitAreas() {
			if tagForbiddenInMode(ha.Tag, c.name) {
				t.Errorf("view mode %s: EQPanelZone publishes hidden-tab hit %q (rect=%v z=%d) — only the active tab's hit areas may be published. "+
					"A non-active tab's control is live and can capture input.",
					c.name, ha.Tag, ha.Rect, ha.ZIndex)
			}
		}
	}
}

// No transient state from the departing tab survives a switch; defined once for every caller.
func assertNoTransientOverlayAfterSwitch(t *testing.T, dv *DrumView, ctx string) {
	t.Helper()
	if dv.tree != nil && dv.portal().IsOpen() {
		t.Fatalf("%s: a portal overlay (top=%q) survived — it can swallow input on the new view", ctx, dv.portal().TopID())
	}
	if dv.tree != nil && dv.portal().HasModal() {
		t.Fatalf("%s: a MODAL portal survived — it blocks ALL input on the new view", ctx)
	}
	if dv.saveAsDialog != nil {
		t.Fatalf("%s: the Save-As dialog survived — it keeps eating keyboard/IME input behind the new view", ctx)
	}
	if dv.tree != nil && dv.tree.Capturing() {
		t.Fatalf("%s: pointer capture %q survived — it keeps routing drags to a hidden handler", ctx, dv.tree.CapturedTag())
	}
	if dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen() {
		t.Fatalf("%s: the audio-panel channel dropdown survived the switch", ctx)
	}
}

// As the sticky-bar pill does.
func openChannelDropdownForTest(dv *DrumView) {
	if dv.eqPanelZone == nil {
		return
	}
	dv.eqPanelZone.channelOpen = true
	dv.eqPanelZone.buildChannelDropdown()
}

// Every setViewMode tears down all transient tab state through resetTransientTabState, Pads included: a
// surviving portal or modal blocks all input behind a hidden panel.
func TestTabIsolation_SwitchResetsTransientState(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	for _, dest := range []struct {
		name string
		mode viewMode
	}{
		{"Pads", viewModeRows},
		{"Synth", viewModeSynth},
		{"EQ", viewModeEQ},
	} {
		t.Run("Sampler_to_"+dest.name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(360, 700)
			advanceFrames(g, 2)
			dv := g.drum

			dv.setViewMode(viewModeSampler)
			advanceFrames(g, 2)
			dv.sampler.loadFromInstrument(roster.Kick, []float32{0.1, -0.2, 0.3, -0.4}, 48000, samplerSourceSynth)
			// Everything a real Sampler session can hold open.
			dv.samplerSaveAs()
			dv.openNamingPortal()
			openChannelDropdownForTest(dv)
			if dv.saveAsDialog == nil {
				t.Fatal("precondition: Save-As dialog did not open")
			}
			if !dv.portal().HasModal() {
				t.Fatal("precondition: modal naming portal did not open")
			}

			dv.setViewMode(dest.mode)
			advanceFrames(g, 2)
			assertNoTransientOverlayAfterSwitch(t, dv, "Sampler->"+dest.name)
		})
	}
}

// Every kind of transient overlay a tab can open; a new opener must be torn down here.
func TestResetTransientTabState_TearsDownEveryOpener(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	cases := []struct {
		name   string
		setup  viewMode // tab to be on before opening (sticky bar / sampler buffer)
		open   func(dv *DrumView)
		isOpen func(dv *DrumView) bool
	}{
		{"overflow-menu", viewModeRows, func(dv *DrumView) { dv.openOverflowMenuPortal() }, func(dv *DrumView) bool { return dv.IsOverflowMenuOpen() }},
		{"context-menu", viewModeRows, func(dv *DrumView) { dv.openContextMenuPortal() }, func(dv *DrumView) bool { return dv.IsContextMenuOpen() }},
		{"naming-modal", viewModeRows, func(dv *DrumView) { dv.openNamingPortal() }, func(dv *DrumView) bool { return dv.IsNamingOpen() }},
		{"channel-dropdown", viewModeEQ, openChannelDropdownForTest, func(dv *DrumView) bool { return dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen() }},
		{"save-as-dialog", viewModeSampler, func(dv *DrumView) {
			dv.sampler.loadFromInstrument(roster.Kick, []float32{0.1, -0.2, 0.3}, 48000, samplerSourceSynth)
			dv.samplerSaveAs()
		}, func(dv *DrumView) bool { return dv.saveAsDialog != nil }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(360, 700)
			advanceFrames(g, 2)
			dv := g.drum

			dv.setViewMode(c.setup)
			advanceFrames(g, 2)

			c.open(dv)
			if !c.isOpen(dv) {
				t.Fatalf("precondition: opener %q did not open its overlay", c.name)
			}

			dv.resetTransientTabState()

			if c.isOpen(dv) {
				t.Fatalf("opener %q survived resetTransientTabState — add its teardown to resetTransientTabState (drumview_close_popups.go)", c.name)
			}
			assertNoTransientOverlayAfterSwitch(t, dv, "after resetTransientTabState/"+c.name)
		})
	}
}

// The tree deliberately exposes no zone list, so this reads the DrumView fields.
func zonesByID(dv *DrumView) map[string]Zone {
	out := map[string]Zone{}
	if dv.eqPanelZone != nil {
		out[dv.eqPanelZone.ID()] = dv.eqPanelZone
	}
	if dv.eqPanelZone != nil && dv.eqPanelZone.chainZone != nil {
		out[dv.eqPanelZone.chainZone.ID()] = dv.eqPanelZone.chainZone
	}
	return out
}

// The capture tag is present, its rect equals the zone's, and its z is nominal.
func assertZoneHasCaptureHitArea(t *testing.T, z Zone, c struct {
	zoneID     string
	captureTag string
	zNominal   int
}) {
	t.Helper()
	rect := rectOf(z)
	if rect.Empty() {
		// A zone with zero bounds (not laid out yet) correctly has none.
		return
	}
	var capture *HitArea
	for i, h := range z.HitAreas() {
		if h.Tag == c.captureTag {
			capture = &z.HitAreas()[i]
			break
		}
	}
	if capture == nil {
		t.Errorf("zone %q: HitAreas() missing %q catch-all. Add `NewInputCaptureHitArea(%q.Rect(), %d, %q)` to its rebuildHitAreas.",
			c.zoneID, c.captureTag, c.zoneID, c.zNominal, c.captureTag)
		return
	}
	if capture.Rect != rect {
		t.Errorf("zone %q: capture Rect=%v != zone.Rect()=%v", c.zoneID, capture.Rect, rect)
	}
	if capture.ZIndex != c.zNominal {
		t.Errorf("zone %q: capture ZIndex=%d, want zone's nominal z=%d", c.zoneID, capture.ZIndex, c.zNominal)
	}
}

// Zone has no Rect(): not every zone has one rect. Extend this for a new case.
func rectOf(z Zone) image.Rectangle {
	switch v := z.(type) {
	case *EQPanelZone:
		return v.PanelRect()
	case *ChainPanelZone:
		return v.Rect()
	default:
		panic(fmt.Sprintf("rectOf: unknown zone type %T; extend the switch in zone_input_isolation_discipline_test.go", z))
	}
}
