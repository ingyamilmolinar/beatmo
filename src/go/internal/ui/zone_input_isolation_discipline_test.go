//go:build test

package ui

import (
	"fmt"
	"image"
	"testing"
)

// zoneInputIsolationCases enumerates every Zone that renders an opaque
// visual surface and therefore MUST register a full-bounds catch-all
// hit area at its nominal z. Tag is the canonical "<zoneID>-capture"
// label callers should use; ZIndex matches the value the zone is
// registered with in DrumViewTree (see drumview_tree.go z-index
// table). To add a zone here, also add a `NewInputCaptureHitArea`
// call to its `rebuildHitAreas` covering its visible Rect.
//
// Excluded by design:
//   - Transport zone (z=100): the action-bar chrome is segmented into
//     buttons that fully tile the bar, no whitespace to leak.
//   - Row rack zone (z=120): segmented rows — every row has its own
//     hit areas, no whitespace.
//   - Timeline zone (z=110): renders the bar + steps area but is
//     INTENTIONALLY transparent to z so the grid drag adapter
//     (z=110) and the per-row scroll forwarder pick up cursor input
//     for the drum grid. Phase 3's fix is to keep the scrub
//     bar's hit area strictly bounded; the rest of the zone
//     stays click-through.
//   - Overlay portals (z≥300): handled by the portal layer.
var zoneInputIsolationCases = []struct {
	zoneID     string
	captureTag string
	zNominal   int
}{
	{"eq-panel", "eq-panel-capture", 130},
	{"chain-panel", "chain-panel-capture", 140},
}

// TestZoneInputIsolationDiscipline pins the contract: every zone in
// `zoneInputIsolationCases` registers a catch-all `HitArea` covering
// its full Rect at the expected nominal z. A failure here means a
// future refactor accidentally removed the catch-all, re-opening the
// input-leak the user reported on the mobile Synth tab. The fix is
// always: add `NewInputCaptureHitArea(zone.Rect(), z, tag)` to the
// zone's `rebuildHitAreas`.
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

// TestZoneInvisibleEqualsNoInput pins the second leg of the
// visibility contract: when a zone's host-supplied visibility
// predicate says it's hidden, `HitAreas()` MUST return an empty
// slice (or one containing no entries). Pre-fix the EQ panel zone
// returned its catch-all + per-control areas even when
// `MobileEQMode()` was false, swallowing input destined for the
// row rack beneath. The tree-level fix in `drumview_tree.go` is
// the primary enforcement; this assertion catches anyone who
// re-introduces the leak at the zone level.
//
// The test forces the canonical "hidden" state for each zone in
// the case table (mobile + viewMode=Rows), then asserts
// `HitAreas()` returns nothing. Add a new zone to the case table
// when it joins the tree.
func TestZoneInvisibleEqualsNoInput(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)
	g.drum.setViewMode(viewModeRows)
	g.drum.recalcButtons()
	// Drive a few frames so the tree's Layout phase runs and any
	// transition-triggered re-publish settles.
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

// TestZoneInvisibleNotInHitIndex is the tree-level mirror: even if a
// zone's HitAreas() were stale, the tree's HitIndex must not contain
// entries owned by an invisible zone. Verifies the tree gate is
// load-bearing independent of per-zone self-policing.
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

	// Probe a point ANYWHERE on screen. The hits returned must not
	// belong to the audio-panel zones (eq-panel / chain-panel), since
	// those are hidden by viewMode=Rows.
	probePoints := []image.Point{
		{X: 50, Y: 50},
		{X: 180, Y: 400},
		{X: 350, Y: 700},
	}
	forbiddenTags := []string{"eq-panel-capture", "chain-panel-capture", "synth-knob-0", "synth-knob-1"}
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

// TestTabIsolation_OnlyActiveTabPublishes enforces leg (i) of the
// tab-isolation contract: the shared EQPanelZone hosts every audio tab's
// content (Wave/Spectrum/Levels/EQ/Chain/Synth/Sampler), so on any given
// view mode its HitAreas() must contain ONLY tags valid for the active tab.
// A tag exclusively owned by a different tab (synth-*, sampler-*,
// save-as-dialog-*, scope-*/chain-panel-capture, or eq-panel-capture on
// Pads) means that tab's controls are still live and can swallow input —
// exactly the class of bug the user reported. Walks all 8 view modes so a
// future tab can't silently re-open the leak the way Sampler did.
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

// assertNoTransientOverlayAfterSwitch is the single invariant a tab/view
// transition must establish: NO transient interaction state from the
// departing tab survives. It centralises every check so the contract is
// defined once and reused by the transition-matrix repro tests and the
// opener-coverage discipline test below.
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

// openChannelDropdownForTest opens the audio-panel channel dropdown the way
// the sticky-bar pill does: flip the zone's open flag and build the portal.
func openChannelDropdownForTest(dv *DrumView) {
	if dv.eqPanelZone == nil {
		return
	}
	dv.eqPanelZone.channelOpen = true
	dv.eqPanelZone.buildChannelDropdown()
}

// TestTabIsolation_SwitchResetsTransientState enforces leg (ii): every
// setViewMode tears down ALL transient per-tab state. The original bug was
// the Sampler Save-As dialog surviving the switch and eating keyboard/IME
// input behind a hidden panel; the broader bug (this strengthened version)
// is that switching *into Pads* skipped CloseAllPopups entirely, so any open
// portal — including a full-screen modal — leaked and blocked all input. The
// single enforcement point is resetTransientTabState at the setViewMode
// chokepoint.
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
			dv.sampler.loadFromInstrument("kick", []float32{0.1, -0.2, 0.3, -0.4}, 48000, samplerSourceSynth)
			// Stack every transient overlay a real Sampler session can hold:
			// the Save-As dialog, a full-screen modal portal, and the channel
			// dropdown. All must be gone after the switch.
			dv.openSamplerSaveAsDialog()
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

// TestResetTransientTabState_TearsDownEveryOpener pins the chokepoint itself,
// independent of setViewMode: resetTransientTabState must close EVERY kind of
// transient overlay a tab can open. Any NEW opener added to the UI must be
// torn down here — this table is the enforcement list. A leaked overlay after
// the chokepoint runs is exactly the Sampler->Pads "input dead" class of bug.
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
			dv.sampler.loadFromInstrument("kick", []float32{0.1, -0.2, 0.3}, 48000, samplerSourceSynth)
			dv.openSamplerSaveAsDialog()
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

// zonesByID returns the discipline-test-visible audio-panel zones,
// keyed by their canonical ID. The tree itself doesn't expose a
// public "list all zones" accessor — that's deliberate (the tree's
// dispatch loop is the only legitimate consumer). For this
// discipline test we reach into the DrumView fields directly,
// mirroring the existing render_pipeline_discipline_test.go pattern.
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

// assertZoneHasCaptureHitArea is the per-zone discipline check.
// Verifies (1) HitAreas() returns at least one entry with the
// expected capture tag, (2) the capture rect equals the zone's
// visible Rect (no shrink-or-grow), (3) the capture's ZIndex
// matches the zone's nominal z.
func assertZoneHasCaptureHitArea(t *testing.T, z Zone, c struct {
	zoneID     string
	captureTag string
	zNominal   int
}) {
	t.Helper()
	rect := rectOf(z)
	if rect.Empty() {
		// Zero-bounds zone (mobile gate, not laid out yet, etc.) —
		// skip; HitAreas() correctly returns no entries.
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

// rectOf returns the zone's visible Rect via type assertion. The
// `Zone` interface doesn't expose Rect() because some zones don't
// have a single rect; the audio-panel zones do. If a new zone is
// added to `zoneInputIsolationCases`, extend this switch.
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
