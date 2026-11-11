//go:build test

package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the contract that the input HitIndex can NEVER drift from
// what a zone renders.
//
// Root cause of the original bug ("mute/solo toggles a different row than
// the one rendered under the mouse after scrolling"): the tree republishes
// a zone's hit areas on an edge-trigger — zone.NeedsLayout() || rect change
// (drumview_tree.go Phase 1) — but needLayout is a CONSUMABLE flag cleared
// by zone.Layout(). DrumView.Draw calls zone.Layout out-of-band every frame
// (the lazy-layout pass and updateRowRects → rowRackZone.Layout), so any
// scroll that lands after Phase 1 (zone wheel adapter fires in Phase 3,
// momentum in Phase 2, legacy touch/step-drag after tree.Update) gets its
// republish signal eaten by Draw: the rendered buttons move, the HitIndex
// keeps the pre-scroll rect snapshots, and the next click dispatches to the
// row that USED to be at that position.

// scrollableTestGame builds a game with enough drum rows that the row rack
// can scroll by at least 2 rows, and settles layout.
func scrollableTestGame(t *testing.T) (*Game, *ebiten.Image) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.Update()

	rack := g.drum.rowRackZone
	if rack == nil {
		t.Fatal("row rack zone is nil")
	}
	for i := 0; len(g.drum.Rows) < rack.VisibleRows()+3 && i < 32; i++ {
		g.drum.AddRow()
		g.Update()
	}
	if len(g.drum.Rows) < rack.VisibleRows()+3 {
		t.Skipf("could not grow rows beyond visible window (rows=%d visible=%d)",
			len(g.drum.Rows), rack.VisibleRows())
	}
	g.Update()
	g.Update()
	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)
	g.Update()
	return g, screen
}

// dispatchPress mimics the tree dispatcher: query the HitIndex at (x, y)
// and press the first handler that does not ignore the event.
func dispatchPress(t *testing.T, g *Game, x, y int) string {
	t.Helper()
	hits := g.drum.tree.HitIndexRef().At(x, y)
	for _, h := range hits {
		if h.Handler == nil {
			continue
		}
		if res := h.Handler.OnPress(x, y); res != InputIgnored {
			// Release immediately so no capture leaks into later tests.
			h.Handler.OnRelease(x, y)
			return h.Tag
		}
	}
	return ""
}

// TestRowRackMuteTargetsRenderedRowAfterScroll reproduces the user-visible
// bug: scroll the rack by one row AFTER the tree's publish gate ran (as the
// wheel adapter, momentum, and the legacy touch/step-drag paths all do),
// let Draw consume the needLayout flag, then click the mute button of a row
// at its RENDERED position. The model row that toggles must be the rendered
// row — not the row that occupied that position before the scroll.
func TestRowRackMuteTargetsRenderedRowAfterScroll(t *testing.T) {
	g, screen := scrollableTestGame(t)
	rack := g.drum.rowRackZone

	// Emulate any post-Phase-1 scroll: offset changes and needLayout is set
	// after the tree already ran its Layout/publish phase this frame.
	rack.SetRowOffset(rack.RowOffset() + 1)

	// Draw repositions the rendered controls (lazy layout + updateRowRects
	// both run zone.Layout) — consuming the needLayout republish signal.
	g.Draw(screen)
	// The next Update MUST leave the HitIndex coherent with the new layout.
	g.Update()

	// Target a row that is rendered on screen right now.
	row := rack.RowOffset() + 1
	if row >= len(rack.entries) || row >= len(g.drum.Rows) {
		t.Fatalf("target row %d out of range (entries=%d rows=%d)", row, len(rack.entries), len(g.drum.Rows))
	}
	btnRect := rack.entries[row].muteBtn.Rect()
	if btnRect.Empty() {
		t.Fatalf("mute button for rendered row %d has empty rect", row)
	}
	cx := btnRect.Min.X + btnRect.Dx()/2
	cy := btnRect.Min.Y + btnRect.Dy()/2

	tag := dispatchPress(t, g, cx, cy)
	t.Logf("pressed (%d,%d) inside rendered mute rect %v of row %d → dispatched to %q", cx, cy, btnRect, row, tag)

	for i, r := range g.drum.Rows {
		if i == row && !r.Muted {
			t.Errorf("rendered row %d under the cursor did NOT mute — input/render drift (stale HitIndex)", row)
		}
		if i != row && r.Muted {
			t.Errorf("row %d muted instead of rendered row %d — input dispatched to the pre-scroll occupant of that position", i, row)
		}
	}
}

// hitAreaEqual compares the published copy of a hit area against the zone's
// current one. Handler identity is compared only when checkHandler is true:
// it matters for zones whose scroll can remap the SAME screen rect to a
// DIFFERENT row's control (the rack), but is meaningless for zones that
// construct fresh adapter values on every HitAreas() call.
func hitAreaEqual(a HitArea, b HitArea, checkHandler bool) bool {
	if a.Rect != b.Rect || a.ZIndex != b.ZIndex || a.Tag != b.Tag ||
		a.Touch != b.Touch || a.ClipRect != b.ClipRect {
		return false
	}
	if !checkHandler {
		return true
	}
	if (a.Handler == nil) != (b.Handler == nil) {
		return false
	}
	if a.Handler != nil {
		ta, tb := reflect.TypeOf(a.Handler), reflect.TypeOf(b.Handler)
		if ta != tb {
			return false
		}
		if ta.Comparable() && a.Handler != b.Handler {
			return false
		}
	}
	return true
}

// handlersCallStable reports whether two consecutive HitAreas() calls on the
// zone return identical handler values — i.e. the zone caches its hit-area
// slice rather than rebuilding adapters per call. Only such zones can have
// their published handler identity meaningfully asserted.
func handlersCallStable(z Zone) bool {
	a, b := z.HitAreas(), z.HitAreas()
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i].Handler == nil) != (b[i].Handler == nil) {
			return false
		}
		if a[i].Handler == nil {
			continue
		}
		ta := reflect.TypeOf(a[i].Handler)
		if ta != reflect.TypeOf(b[i].Handler) || !ta.Comparable() || a[i].Handler != b[i].Handler {
			return false
		}
	}
	return true
}

// assertHitIndexMatchesZones asserts the structural invariant: for every
// registered zone, the hit areas owned by that zone in the tree's HitIndex
// must be exactly the zone's current HitAreas() (and empty when the zone is
// hidden). Any divergence means input dispatch no longer matches rendering.
func assertHitIndexMatchesZones(t *testing.T, g *Game, when string) {
	t.Helper()
	tree := g.drum.tree
	for _, e := range tree.zones {
		id := e.zone.ID()
		var published []HitArea
		for _, a := range tree.hitIndex.areas {
			if a.ownerID == id && !a.isPortal {
				published = append(published, a.HitArea)
			}
		}
		visible := e.visible == nil || e.visible()
		if !visible {
			if len(published) != 0 {
				t.Errorf("[%s] zone %q is hidden but still has %d published hit areas", when, id, len(published))
			}
			continue
		}
		cur := e.zone.HitAreas()
		if len(cur) != len(published) {
			t.Errorf("[%s] zone %q: HitIndex has %d areas, zone has %d — publish gate missed a layout",
				when, id, len(published), len(cur))
			continue
		}
		checkHandler := handlersCallStable(e.zone)
		for i := range cur {
			if !hitAreaEqual(published[i], cur[i], checkHandler) {
				t.Errorf("[%s] zone %q: hit area %d drifted: published tag=%q rect=%v, zone tag=%q rect=%v",
					when, id, i, published[i].Tag, published[i].Rect, cur[i].Tag, cur[i].Rect)
			}
		}
	}
}

// TestHitIndexNeverDriftsFromZoneHitAreas drives frames with scroll
// mutations interleaved at the points real input lands (between the tree's
// publish phase and Draw) and asserts the HitIndex ↔ zone parity invariant
// after every settled frame.
func TestHitIndexNeverDriftsFromZoneHitAreas(t *testing.T) {
	g, screen := scrollableTestGame(t)
	rack := g.drum.rowRackZone

	assertHitIndexMatchesZones(t, g, "settled")

	// Scroll after the publish phase, then Draw (consumes needLayout), then
	// Update — the exact sequence that strands the index.
	rack.SetRowOffset(rack.RowOffset() + 1)
	g.Draw(screen)
	g.Update()
	assertHitIndexMatchesZones(t, g, "scroll+draw+update")

	// Again with two Draws (playback draws every frame).
	rack.SetRowOffset(rack.RowOffset() + 1)
	g.Draw(screen)
	g.Draw(screen)
	g.Update()
	assertHitIndexMatchesZones(t, g, "scroll+2×draw+update")

	// Scroll back and settle.
	rack.SetRowOffset(0)
	g.Draw(screen)
	g.Update()
	g.Draw(screen)
	assertHitIndexMatchesZones(t, g, "scroll-home+update+draw")
}

// TestZoneLayoutRoutesThroughTreeDiscipline forbids direct zone.Layout()
// calls outside the tree. Out-of-band Layout consumes the zone's needLayout
// flag — the tree's only republish signal — without updating the HitIndex,
// which is exactly how the stale-input bug was born. All forced layouts
// must go through DrumViewTree.LayoutZoneNow / EnsureLayouts so layout and
// publish can never be separated.
func TestZoneLayoutRoutesThroughTreeDiscipline(t *testing.T) {
	// file → allowed substrings for lines that may call <something>Zone.Layout(.
	allowed := map[string][]string{
		"drumview_tree.go": {""}, // the chokepoint itself — any use allowed
		// grid_tree.go is the GridTree's own chokepoint (sibling tree owning
		// the top grid pane). Its layoutPass/LayoutZoneNow call zone.Layout
		// and immediately publish via hitIndex.Update — the same
		// layout-and-publish-together discipline as drumview_tree.go.
		"grid_tree.go": {""}, // sibling chokepoint — any use allowed
		// ChainPanelZone is a nested composite inside EQPanelZone (not
		// registered with the tree); its parent lays it out and owns its
		// hit areas.
		"eq_panel_zone.go": {"z.chainZone.Layout("},
	}
	re := regexp.MustCompile(`(?:\b[A-Za-z_]\w*[Zz]one|\bzone)\.Layout\(`)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for ln, line := range strings.Split(string(data), "\n") {
			if !re.MatchString(line) {
				continue
			}
			ok := false
			for _, allow := range allowed[name] {
				if allow == "" || strings.Contains(line, allow) {
					ok = true
					break
				}
			}
			if !ok {
				t.Errorf("%s:%d: direct zone Layout call bypasses the tree's hit-area publish:\n\t%s\n"+
					"Use dv.tree.LayoutZoneNow(id) (forced) or rely on Invalidate() + the tree's layout pass.",
					name, ln+1, strings.TrimSpace(line))
			}
		}
	}
}
