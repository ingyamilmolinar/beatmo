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

// The HitIndex must never drift from what a zone renders. needLayout is the tree's only republish signal and a
// consumable flag: a Layout outside the tree eats it, the buttons move and the index keeps the old rects, so a
// click hits the row that was there before the scroll.

// Enough rows to scroll by at least 2.
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

// The tree dispatcher's rule: the first handler that does not ignore the press.
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

// Scrolled after the tree's publish ran, as the wheel, momentum and touch paths do, with Draw then eating
// needLayout: the mute click must still toggle the row rendered under it.
func TestRowRackMuteTargetsRenderedRowAfterScroll(t *testing.T) {
	g, screen := scrollableTestGame(t)
	rack := g.drum.rowRackZone

	// Any scroll that lands after the tree's publish phase.
	rack.SetRowOffset(rack.RowOffset() + 1)

	// Draw lays the zone out and consumes the republish signal.
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

// Handler identity only matters where a scroll can remap the same rect to another row's control (the rack);
// other zones build fresh adapters per HitAreas() call.
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

// Only a zone that caches its slice can have its published handler identity asserted.
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

// Each zone's entries in the HitIndex are exactly its current HitAreas(), and none while it is hidden.
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

// Scroll mutations land where real input does, between the publish phase and Draw.
func TestHitIndexNeverDriftsFromZoneHitAreas(t *testing.T) {
	g, screen := scrollableTestGame(t)
	rack := g.drum.rowRackZone

	assertHitIndexMatchesZones(t, g, "settled")

	// The exact sequence that strands the index.
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

// A Layout outside the tree consumes needLayout without republishing; forced layouts go through
// LayoutZoneNow / EnsureLayouts, so layout and publish never separate.
func TestZoneLayoutRoutesThroughTreeDiscipline(t *testing.T) {
	// file → allowed substrings for lines that may call <something>Zone.Layout(.
	allowed := map[string][]string{
		"drumview_tree.go": {""}, // the chokepoint itself — any use allowed
		// The GridTree's own chokepoint, which publishes right after it lays out.
		"grid_tree.go": {""}, // sibling chokepoint — any use allowed
		// Nested inside EQPanelZone, which lays it out and owns its hit areas.
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
