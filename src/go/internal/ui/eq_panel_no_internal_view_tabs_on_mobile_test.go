//go:build test

package ui

import (
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestEQPanel_NoRightSideTabsOnMobile (Theme 1) verifies that on mobile
// the EQ panel suppresses its right-side internal view-tab strip — the
// bottom-bar segmented switcher owns that role. The left-side
// Master/HP/LP/Freeze chips remain.
func TestEQPanel_NoRightSideTabsOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum
	dv.setViewMode(viewModeEQ)
	advanceFrames(g, 2)

	z := dv.eqPanelZone
	if z == nil {
		t.Fatal("eqPanelZone nil")
	}

	for i := 0; i < 5; i++ {
		btn := z.stickyBar.TabBtn(i)
		if btn != nil && !btn.Rect().Empty() {
			t.Errorf("mobile: tabButton[%d] rect should be empty, got %v", i, btn.Rect())
		}
	}
	// Left strip remains.
	if z.stickyBar.ChannelBtn() == nil || z.stickyBar.ChannelBtn().Rect().Empty() {
		t.Error("mobile: eqChannelBtn (Master) should still be laid out")
	}
	if z.hpfBtn == nil || z.hpfBtn.Rect().Empty() {
		t.Error("mobile: hpfBtn should still be laid out on EQ tab")
	}
	// Hit areas: no eq-tab-* tags should appear.
	for _, ha := range z.hitAreas {
		if strings.HasPrefix(ha.Tag, "eq-tab-") {
			t.Errorf("mobile: hit area %q should not be registered", ha.Tag)
		}
	}
}

// TestEQPanel_RightSideTabsPresentOnDesktop guards desktop unchanged.
func TestEQPanel_RightSideTabsPresentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	advanceFrames(g, 2)
	dv := g.drum
	z := dv.eqPanelZone
	if z == nil {
		t.Fatal("eqPanelZone nil")
	}
	nonEmpty := 0
	for i := 0; i < 5; i++ {
		btn := z.stickyBar.TabBtn(i)
		if btn != nil && !btn.Rect().Empty() {
			nonEmpty++
		}
	}
	if nonEmpty != 5 {
		t.Errorf("desktop: expected all 5 tab buttons laid out, got %d", nonEmpty)
	}
}
