//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A grid-pane-only toast is invisible in every mobile view but Pads, exactly when the user stares at a
// disabled Play. Iterating bottomNavModeList covers a future view automatically.
func TestToastVisibleInEveryMobileViewMode(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844) // portrait mobile
	screen := image.Rect(0, 0, 390, 844)

	for _, vm := range bottomNavModeList {
		g.drum.setViewMode(vm)

		g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, 100)
		g.drum.logsToast.updateTask(42)

		host := g.toastHostRect()
		if host.Empty() {
			t.Fatalf("view mode %v: toast host rect is empty on mobile", vm)
		}
		r := g.drum.logsToast.rectIn(host)
		if r.Empty() {
			t.Fatalf("view mode %v: toast rect empty despite an active task", vm)
		}
		if !r.In(screen) {
			t.Errorf("view mode %v: toast rect %v is outside the screen %v", vm, r, screen)
		}

		// Exactly one of the two toast layers is visible: Pads uses the grid pane, every other mode the drum pane.
		wantGrid := vm == viewModeRows
		gotGrid := g.drum.gridPaneVisibleForToast()
		if gotGrid != wantGrid {
			t.Errorf("view mode %v: gridPaneVisibleForToast=%v want %v", vm, gotGrid, wantGrid)
		}

		g.drum.logsToast.endTask()
	}
}

// The fallback paints only when the grid pane is not the active mobile surface, and never on desktop.
func TestDrumToastLayerVisibilityMatchesHostRect(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	var toastLayer *DrumToastLayer
	for _, l := range g.drum.audioTree.LayersForTest() {
		if dt, ok := l.(*DrumToastLayer); ok {
			toastLayer = dt
		}
	}
	if toastLayer == nil {
		t.Fatal("drumview-toast layer not registered on audioTree")
	}

	g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, 100)
	t.Cleanup(func() { g.drum.logsToast.endTask() })

	g.drum.setViewMode(viewModeRows)
	if toastLayer.Visible() {
		t.Error("drumview-toast visible on Pads (grid pane owns the toast there)")
	}

	g.drum.setViewMode(viewModeEQ)
	if !toastLayer.Visible() {
		t.Error("drumview-toast NOT visible on EQ view mode with an active task")
	}

	g.drum.logsToast.endTask()
	if toastLayer.Visible() {
		t.Error("drumview-toast visible with no active task")
	}
}

// A passive toast must not swallow a tap meant for the row rack or EQ panel beneath.
func TestToastLayerRegistersNoHitArea(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	g.drum.setViewMode(viewModeEQ)
	g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, 100)
	t.Cleanup(func() { g.drum.logsToast.endTask() })

	found := false
	for _, l := range g.drum.audioTree.LayersForTest() {
		if _, ok := l.(*DrumToastLayer); ok {
			found = true
			if _, isZone := l.(zoneAsLayer); isZone {
				t.Fatal("drumview-toast is a Zone, not a passive Layer — it would publish HitAreas")
			}
		}
	}
	if !found {
		t.Fatal("drumview-toast layer not found in audioTree")
	}
}
